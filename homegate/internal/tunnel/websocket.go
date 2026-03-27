package tunnel

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/homegate/agent/internal/protocol"
)

// HandleWebSocket opens a WebSocket to the upstream target and bridges frames
// bidirectionally between the tunnel channel and the upstream connection.
func (p *RequestProxy) HandleWebSocket(streamID uint32, reqHeaders RequestHeaders, ch <-chan *protocol.Frame, sendFrame func(*protocol.Frame)) {
	// Build upstream WebSocket URL
	wsURL := strings.Replace(p.target, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL += reqHeaders.Path

	// Forward relevant headers
	httpHeaders := http.Header{}
	for k, v := range reqHeaders.Headers {
		switch strings.ToLower(k) {
		case "x-forwarded-for", "x-forwarded-proto", "x-forwarded-host",
			"x-real-ip", "cf-connecting-ip", "cf-ipcountry", "cf-ray",
			"cf-visitor", "cdn-loop", "connection", "upgrade",
			"sec-websocket-key", "sec-websocket-version",
			"sec-websocket-extensions", "sec-websocket-protocol":
			// Skip WebSocket handshake headers (gorilla handles these)
			// and proxy/CDN headers
			continue
		}
		httpHeaders.Set(k, v)
	}

	upstreamWS, _, err := websocket.DefaultDialer.Dial(wsURL, httpHeaders)
	if err != nil {
		log.Printf("stream %d: ws upstream dial error: %v", streamID, err)
		p.sendError(streamID, 502, "websocket upstream unavailable", sendFrame)
		return
	}
	defer upstreamWS.Close()

	// Send response headers to indicate successful upgrade
	respHeaders := ResponseHeaders{
		StatusCode: 101,
		Headers:    map[string]string{},
	}
	headersJSON, _ := json.Marshal(respHeaders)
	sendFrame(&protocol.Frame{
		StreamID: streamID,
		Type:     protocol.FrameResponseHeaders,
		Payload:  headersJSON,
	})

	done := make(chan struct{})

	// Upstream HA → Broker (via sendFrame)
	go func() {
		defer close(done)
		for {
			_, msg, err := upstreamWS.ReadMessage()
			if err != nil {
				return
			}
			sendFrame(&protocol.Frame{
				StreamID: streamID,
				Type:     protocol.FrameWebSocketData,
				Payload:  msg,
			})
		}
	}()

	// Broker → Upstream HA (from channel)
	go func() {
		for f := range ch {
			switch f.Type {
			case protocol.FrameWebSocketData:
				if err := upstreamWS.WriteMessage(websocket.TextMessage, f.Payload); err != nil {
					return
				}
			case protocol.FrameStreamClose:
				upstreamWS.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
		}
		// Channel closed — close upstream
		upstreamWS.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}()

	<-done

	// Send stream close back to broker
	sendFrame(&protocol.Frame{
		StreamID: streamID,
		Type:     protocol.FrameStreamClose,
	})
}

func isWebSocketUpgrade(headers map[string]string) bool {
	for k, v := range headers {
		if strings.EqualFold(k, "upgrade") && strings.EqualFold(v, "websocket") {
			return true
		}
	}
	return false
}
