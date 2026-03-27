// apps/agent/internal/tunnel/client.go
package tunnel

import (
	"bytes"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/homegate/agent/internal/protocol"
)

type ConnState int32

const (
	StateDisconnected ConnState = iota
	StateConnecting
	StateConnected
	StateReconnecting
	StateFailed
)

type Client struct {
	brokerURL string
	jwt       string
	haTarget  string
	proxy     *RequestProxy

	label     string
	state     atomic.Int32
	baseDelay time.Duration
	mu        sync.Mutex
	ws        *websocket.Conn
	done      chan struct{}
	closeOnce sync.Once
}

func NewClient(brokerURL, jwt, haTarget string) *Client {
	return &Client{
		brokerURL: brokerURL,
		jwt:       jwt,
		haTarget:  haTarget,
		proxy:     NewRequestProxy(haTarget),
		baseDelay: 1 * time.Second,
		done:      make(chan struct{}),
	}
}

func (c *Client) SetBaseDelay(d time.Duration) {
	c.baseDelay = d
}

func (c *Client) State() ConnState {
	return ConnState(c.state.Load())
}

func (c *Client) Label() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.label
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		c.mu.Lock()
		if c.ws != nil {
			c.ws.Close()
		}
		c.mu.Unlock()
	})
}

func (c *Client) Done() <-chan struct{} {
	return c.done
}

func (c *Client) Connect() error {
	attempt := 0
	for {
		select {
		case <-c.done:
			return nil
		default:
		}

		if attempt > 0 {
			c.state.Store(int32(StateReconnecting))
			delay := c.backoff(attempt)
			log.Printf("reconnecting in %v (attempt %d)", delay, attempt)
			select {
			case <-time.After(delay):
			case <-c.done:
				return nil
			}
		}

		c.state.Store(int32(StateConnecting))
		connStart := time.Now()
		err := c.connectOnce()
		if err != nil {
			log.Printf("connection error: %v", err)

			// Check if this is an auth failure (should not reconnect)
			if websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
				c.state.Store(int32(StateFailed))
				return err
			}

			// Reset backoff if the connection was up for a while (not a fast failure)
			if time.Since(connStart) > 30*time.Second {
				attempt = 1
			} else {
				attempt++
			}
			continue
		}

		// Successful connection was closed normally, reset attempt counter
		attempt = 0
	}
}

func (c *Client) connectOnce() error {
	ws, _, err := websocket.DefaultDialer.Dial(c.brokerURL, nil)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.ws = ws
	c.mu.Unlock()

	defer func() {
		ws.Close()
		c.mu.Lock()
		c.ws = nil
		c.mu.Unlock()
	}()

	// Send JWT as first message
	if err := ws.WriteMessage(websocket.TextMessage, []byte(c.jwt)); err != nil {
		return err
	}

	// Read ACK
	_, msg, err := ws.ReadMessage()
	if err != nil {
		return err
	}

	var ack struct {
		Status string `json:"status"`
		Label  string `json:"label"`
	}
	if err := json.Unmarshal(msg, &ack); err != nil {
		return err
	}

	c.mu.Lock()
	c.label = ack.Label
	c.mu.Unlock()
	c.state.Store(int32(StateConnected))

	log.Printf("connected to broker, label=%s", ack.Label)

	// Start ping keepalive to prevent Cloudflare idle timeout (100s)
	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.mu.Lock()
				if c.ws != nil {
					c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
				}
				c.mu.Unlock()
			case <-pingDone:
				return
			case <-c.done:
				return
			}
		}
	}()

	err = c.readLoop(ws)
	close(pingDone)
	return err
}

func (c *Client) readLoop(ws *websocket.Conn) error {
	activeStreams := make(map[uint32]chan *protocol.Frame)

	cleanup := func() {
		for _, ch := range activeStreams {
			close(ch)
		}
	}

	for {
		select {
		case <-c.done:
			cleanup()
			return nil
		default:
		}

		_, raw, err := ws.ReadMessage()
		if err != nil {
			cleanup()
			return err
		}

		frame, err := protocol.DecodeFrame(bytes.NewReader(raw))
		if err != nil {
			log.Printf("frame decode error: %v", err)
			continue
		}

		sid := frame.StreamID

		switch frame.Type {
		case protocol.FrameRequestHeaders:
			ch := make(chan *protocol.Frame, 16)
			activeStreams[sid] = ch
			go c.handleStreamChan(sid, ch)
			ch <- frame

		case protocol.FrameRequestBody:
			if ch, ok := activeStreams[sid]; ok {
				ch <- frame
			}

		case protocol.FrameStreamClose:
			if ch, ok := activeStreams[sid]; ok {
				close(ch)
				delete(activeStreams, sid)
			}

		default:
			if ch, ok := activeStreams[sid]; ok {
				ch <- frame
			}
		}
	}
}

const bodyCollectTimeout = 10 * time.Millisecond

func (c *Client) handleStreamChan(streamID uint32, ch chan *protocol.Frame) {
	var frames []*protocol.Frame

	// Collect frames — wait briefly for body frames after headers
	timer := time.NewTimer(bodyCollectTimeout)
	defer timer.Stop()

	for {
		select {
		case f, ok := <-ch:
			if !ok {
				goto dispatch
			}
			frames = append(frames, f)
			if f.Type == protocol.FrameRequestBody {
				timer.Reset(bodyCollectTimeout)
			}
		case <-timer.C:
			goto dispatch
		case <-c.done:
			return
		}
	}

dispatch:
	sendFrame := func(f *protocol.Frame) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.ws != nil {
			c.ws.WriteMessage(websocket.BinaryMessage, f.Encode())
		}
	}

	// Check if this is a WebSocket upgrade request
	if c.isWebSocketRequest(frames) {
		var reqHeaders RequestHeaders
		for _, f := range frames {
			if f.Type == protocol.FrameRequestHeaders {
				json.Unmarshal(f.Payload, &reqHeaders)
				break
			}
		}
		c.proxy.HandleWebSocket(streamID, reqHeaders, ch, sendFrame)
		return
	}

	c.proxy.HandleStream(streamID, frames, sendFrame)
}

func (c *Client) isWebSocketRequest(frames []*protocol.Frame) bool {
	for _, f := range frames {
		if f.Type == protocol.FrameRequestHeaders {
			var reqHeaders RequestHeaders
			if err := json.Unmarshal(f.Payload, &reqHeaders); err == nil {
				return isWebSocketUpgrade(reqHeaders.Headers)
			}
		}
	}
	return false
}

func (c *Client) backoff(attempt int) time.Duration {
	delay := c.baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > 60*time.Second {
			delay = 60 * time.Second
			break
		}
	}
	return delay
}
