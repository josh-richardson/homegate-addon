// apps/agent/internal/tunnel/proxy.go
package tunnel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/homegate/agent/internal/protocol"
)

type RequestHeaders struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

type ResponseHeaders struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
}

type RequestProxy struct {
	target string
	client *http.Client
}

func NewRequestProxy(target string) *RequestProxy {
	return &RequestProxy{
		target: target,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// HandleStream processes a sequence of request frames for a single stream,
// makes the upstream HTTP request, and calls sendFrame for each response frame.
func (p *RequestProxy) HandleStream(streamID uint32, requestFrames []*protocol.Frame, sendFrame func(*protocol.Frame)) {
	// Parse request headers
	var reqHeaders RequestHeaders
	var bodyChunks [][]byte

	for _, f := range requestFrames {
		switch f.Type {
		case protocol.FrameRequestHeaders:
			if err := json.Unmarshal(f.Payload, &reqHeaders); err != nil {
				log.Printf("stream %d: bad request headers: %v", streamID, err)
				p.sendError(streamID, 400, "bad request headers", sendFrame)
				return
			}
		case protocol.FrameRequestBody:
			bodyChunks = append(bodyChunks, f.Payload)
		}
	}

	if reqHeaders.Method == "" {
		p.sendError(streamID, 400, "missing request headers", sendFrame)
		return
	}

	// Build HTTP request
	var body io.Reader
	if len(bodyChunks) > 0 {
		combined := bytes.Join(bodyChunks, nil)
		body = bytes.NewReader(combined)
	}

	url := p.target + reqHeaders.Path
	req, err := http.NewRequest(reqHeaders.Method, url, body)
	if err != nil {
		p.sendError(streamID, 502, fmt.Sprintf("build request: %v", err), sendFrame)
		return
	}

	for k, v := range reqHeaders.Headers {
		// Strip proxy/CDN headers — the agent is the final hop, not a proxy.
		// Forwarding these to HA from an untrusted IP causes 400 errors.
		switch strings.ToLower(k) {
		case "x-forwarded-for", "x-forwarded-proto", "x-forwarded-host",
			"x-real-ip", "cf-connecting-ip", "cf-ipcountry", "cf-ray",
			"cf-visitor", "cdn-loop", "connection", "accept-encoding":
			continue
		}
		req.Header.Set(k, v)
	}
	// Explicitly request uncompressed responses so Content-Length is accurate.
	// Go's http.Client auto-adds Accept-Encoding: gzip and decompresses,
	// which strips Content-Length and causes 0-byte responses through Cloudflare.
	req.Header.Set("Accept-Encoding", "identity")

	// Execute request
	resp, err := p.client.Do(req)
	if err != nil {
		log.Printf("stream %d: upstream error: %v", streamID, err)
		p.sendError(streamID, 502, "upstream unavailable", sendFrame)
		return
	}
	defer resp.Body.Close()

	// Send response headers
	respHeaders := ResponseHeaders{
		StatusCode: resp.StatusCode,
		Headers:    make(map[string]string),
	}
	for k, v := range resp.Header {
		respHeaders.Headers[k] = v[0]
	}
	headersJSON, _ := json.Marshal(respHeaders)
	sendFrame(&protocol.Frame{
		StreamID: streamID,
		Type:     protocol.FrameResponseHeaders,
		Payload:  headersJSON,
	})

	// Send response body in chunks
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			sendFrame(&protocol.Frame{
				StreamID: streamID,
				Type:     protocol.FrameResponseBody,
				Payload:  chunk,
			})
		}
		if err != nil {
			break
		}
	}

	// Send stream close
	sendFrame(&protocol.Frame{
		StreamID: streamID,
		Type:     protocol.FrameStreamClose,
	})
}

func (p *RequestProxy) sendError(streamID uint32, statusCode int, message string, sendFrame func(*protocol.Frame)) {
	respHeaders := ResponseHeaders{
		StatusCode: statusCode,
		Headers:    map[string]string{"Content-Type": "text/plain"},
	}
	headersJSON, _ := json.Marshal(respHeaders)
	sendFrame(&protocol.Frame{StreamID: streamID, Type: protocol.FrameResponseHeaders, Payload: headersJSON})
	sendFrame(&protocol.Frame{StreamID: streamID, Type: protocol.FrameResponseBody, Payload: []byte(message)})
	sendFrame(&protocol.Frame{StreamID: streamID, Type: protocol.FrameStreamClose})
}
