// Phase 0 spike harness: deterministic OpenAI-compatible fake upstream.
//
// One binary serves N independent upstreams (one per deployment slot) on
// distinct loopback ports. Each keeps its own request counter so a fallback
// claim can be proven with real numbers instead of assertion.
//
// Control endpoints:
//
//	GET  /_stats   -> {"port","count","behavior"}
//	POST /_config  -> replace behavior
//	POST /_reset   -> zero the counter
//
// Behaviour modes:
//
//	ok                  normal JSON completion
//	empty               HTTP 200 with zero-byte body
//	malformed           HTTP 200 with non-JSON body
//	reset               close the connection without a response
//	sse_ok              stream: role chunk, content chunks, [DONE]
//	sse_fail_after_role stream: role chunk, then abort
//	sse_fail_after_content stream: role+content chunk, then abort
//	sse_tool_ok         stream: streamed function tool-call deltas, [DONE]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Behavior struct {
	Status           int    `json:"status"`
	DelayMS          int    `json:"delay_ms"`
	FirstByteDelayMS int    `json:"first_byte_delay_ms"`
	Mode             string `json:"mode"`
	Body             string `json:"body"`
}

type Slot struct {
	Port     int
	mu       sync.Mutex
	count    int
	behavior Behavior
}

func (s *Slot) stats() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"port": s.Port, "count": s.count, "behavior": s.behavior}
}

func (s *Slot) bump() (int, Behavior) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	return s.count, s.behavior
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func completionBody(model string) map[string]any {
	return map[string]any{
		"id":      "chatcmpl-fake",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": "fake-upstream response",
				},
			},
		},
		"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7},
	}
}

func toolCallBody(model string) map[string]any {
	return map[string]any{
		"id":      "chatcmpl-fake-tool",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"finish_reason": "tool_calls",
				"message": map[string]any{
					"role":    "assistant",
					"content": nil,
					"tool_calls": []any{
						map[string]any{
							"id":   "call_fake_1",
							"type": "function",
							"function": map[string]any{
								"name":      "get_weather",
								"arguments": `{"city":"Accra"}`,
							},
						},
					},
				},
			},
		},
		"usage": map[string]any{"prompt_tokens": 9, "completion_tokens": 5, "total_tokens": 14},
	}
}

func handler(s *Slot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_stats":
			writeJSON(w, 200, s.stats())
			return
		case "/_reset":
			s.mu.Lock()
			s.count = 0
			s.mu.Unlock()
			writeJSON(w, 200, map[string]any{"reset": true, "port": s.Port})
			return
		case "/_config":
			var b Behavior
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				writeJSON(w, 400, map[string]any{"error": err.Error()})
				return
			}
			s.mu.Lock()
			s.behavior = b
			s.mu.Unlock()
			writeJSON(w, 200, map[string]any{"applied": b, "port": s.Port})
			return
		case "/health":
			writeJSON(w, 200, map[string]any{"status": "ok", "port": s.Port})
			return
		case "/v1/models":
			writeJSON(w, 200, map[string]any{
				"object": "list",
				"data": []any{
					map[string]any{"id": "fake-model", "object": "model", "owned_by": "fake"},
				},
			})
			return
		case "/v1/chat/completions":
			// handled below
		default:
			writeJSON(w, 404, map[string]any{"error": map[string]any{"message": "not found", "type": "invalid_request_error"}})
			return
		}

		var req struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []any  `json:"messages"`
			Tools    []any  `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		n, b := s.bump()
		log.Printf("slot=%d request#%d mode=%s stream=%v tools=%d model=%s",
			s.Port, n, b.Mode, req.Stream, len(req.Tools), req.Model)

		if b.DelayMS > 0 {
			time.Sleep(time.Duration(b.DelayMS) * time.Millisecond)
		}

		if b.Status != 0 {
			writeJSON(w, b.Status, map[string]any{
				"error": map[string]any{
					"message": fmt.Sprintf("fake upstream status %d", b.Status),
					"type":    "fake_error",
					"code":    b.Status,
				},
			})
			return
		}

		model := req.Model
		if model == "" {
			model = "fake-model"
		}

		switch b.Mode {
		case "reset":
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					if tcp, ok := conn.(*net.TCPConn); ok {
						_ = tcp.SetLinger(0)
					}
					_ = conn.Close()
					return
				}
			}
			return
		case "empty":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			return
		case "malformed":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"this is":not valid json`))
			return
		case "sse_ok", "sse_fail_after_role", "sse_fail_after_content":
			streamSSE(w, b, model)
			return
		case "sse_tool_ok":
			streamToolSSE(w, model)
			return
		}

		if b.Body != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(b.Body))
			return
		}

		if len(req.Tools) > 0 && !req.Stream {
			writeJSON(w, 200, toolCallBody(model))
			return
		}
		writeJSON(w, 200, completionBody(model))
	}
}

func streamSSE(w http.ResponseWriter, b Behavior, model string) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, map[string]any{"error": "no flusher"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)

	if b.FirstByteDelayMS > 0 {
		time.Sleep(time.Duration(b.FirstByteDelayMS) * time.Millisecond)
	}

	role := map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
	}
	writeChunk(w, role)
	fl.Flush()

	if b.Mode == "sse_fail_after_role" {
		if tcp, ok := w.(interface{ Unwrap() http.ResponseWriter }); ok {
			_ = tcp
		}
		panic(http.ErrAbortHandler)
	}

	content := map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "fake streaming content"}, "finish_reason": nil}},
	}
	writeChunk(w, content)
	fl.Flush()

	if b.Mode == "sse_fail_after_content" {
		panic(http.ErrAbortHandler)
	}

	done := map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
	}
	writeChunk(w, done)
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	fl.Flush()
}

func streamToolSSE(w http.ResponseWriter, model string) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, map[string]any{"error": "no flusher"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)

	id := "chatcmpl-fake-tool-stream"
	role := map[string]any{
		"id": id, "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{"role": "assistant", "tool_calls": []any{
				map[string]any{
					"index": 0, "id": "call_fake_stream_1", "type": "function",
					"function": map[string]any{"name": "get_weather", "arguments": ""},
				},
			}},
			"finish_reason": nil,
		}},
	}
	writeChunk(w, role)
	fl.Flush()

	args1 := map[string]any{
		"id": id, "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{"tool_calls": []any{
				map[string]any{"index": 0, "function": map[string]any{"arguments": `{"city":"`}},
			}},
			"finish_reason": nil,
		}},
	}
	writeChunk(w, args1)
	fl.Flush()

	args2 := map[string]any{
		"id": id, "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{"tool_calls": []any{
				map[string]any{"index": 0, "function": map[string]any{"arguments": `Accra"}`}},
			}},
			"finish_reason": nil,
		}},
	}
	writeChunk(w, args2)
	fl.Flush()

	done := map[string]any{
		"id": id, "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}},
	}
	writeChunk(w, done)
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	fl.Flush()
}

func writeChunk(w http.ResponseWriter, v any) {
	buf, _ := json.Marshal(v)
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(buf)
	_, _ = w.Write([]byte("\n\n"))
}

func main() {
	ports := flag.String("ports", "10001,10002,10003", "comma-separated loopback ports, one per slot")
	flag.Parse()

	vals := strings.Split(*ports, ",")
	var wg sync.WaitGroup
	for i, p := range vals {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var port int
		if _, err := fmt.Sscanf(p, "%d", &port); err != nil {
			log.Fatalf("bad port %q: %v", p, err)
		}
		slot := &Slot{Port: port, behavior: Behavior{Mode: "ok"}}
		srv := &http.Server{
			Addr:              fmt.Sprintf("127.0.0.1:%d", port),
			Handler:           handler(slot),
			ReadHeaderTimeout: 10 * time.Second,
		}
		wg.Add(1)
		go func(s *http.Server, idx int, pt int) {
			defer wg.Done()
			log.Printf("fake upstream slot %c listening on 127.0.0.1:%d", rune('A'+idx), pt)
			if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("slot %d stopped: %v", pt, err)
			}
		}(srv, i, port)
	}
	log.Printf("fake upstream ready on %s", *ports)
	wg.Wait()
}
