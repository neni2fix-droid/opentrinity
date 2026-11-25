// Package gateway implements the single loopback OpenAI-compatible ingress.
// It authenticates, allowlists, enforces limits and passes bytes through.
// It never chooses a provider, retries, falls back or inspects prompts.
package gateway

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nenifix/opentrinity/internal/config"
)

const (
	// SPEC.md: maximum request body is 10 MiB.
	maxBodyBytes  = 10 * 1024 * 1024
	maxConcurrent = 16
)

// Run starts the ingress listener and blocks until interrupted.
func Run(paths config.Paths, logger *log.Logger) error {
	apiKey := os.Getenv("OT_GATEWAY_KEY")
	litellmKey := os.Getenv("OT_LITELLM_KEY")
	if apiKey == "" || litellmKey == "" {
		return errors.New("gateway keys are missing from the process environment")
	}
	h, err := newHandler(apiKey, litellmKey, logger)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              "127.0.0.1:" + strconv.Itoa(config.GatewayPort),
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		// SPEC.md: maximum header size is 32 KiB.
		MaxHeaderBytes:    32 * 1024,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		logger.Printf("gateway listening on %s", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

type handler struct {
	apiKey     []byte
	litellmKey string
	logger     *log.Logger
	proxy      *httputil.ReverseProxy
	readyHTTP  *http.Client
	inFlight   int64
	reqSeq     atomic.Uint64
}

func newHandler(apiKey, litellmKey string, logger *log.Logger) (*handler, error) {
	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 0, // streaming must not be cut off between chunks
		ForceAttemptHTTP2:     false,
	}
	h := &handler{
		apiKey:     []byte(apiKey),
		litellmKey: litellmKey,
		logger:     logger,
		readyHTTP:  &http.Client{Timeout: 2 * time.Second, Transport: transport},
	}
	h.proxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "127.0.0.1:" + strconv.Itoa(config.LiteLLMPort)
			req.Host = "127.0.0.1:" + strconv.Itoa(config.LiteLLMPort)
			req.Header.Set("Authorization", "Bearer "+litellmKey)
			req.Header.Del("Proxy-Authorization")
			req.Header.Del("X-Gateway-Key")
		},
		Transport:     transport,
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			sanitizeResponse(resp)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			cw, _ := w.(*commitWriter)
			if cw != nil && cw.committed {
				closeClientConnection(w)
				return
			}
			// Upstream error text is deliberately discarded.
			writeError(w, http.StatusBadGateway, "upstream request failed")
		},
	}
	return h, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := fmt.Sprintf("req-%d", h.reqSeq.Add(1))
	rec := &commitWriter{ResponseWriter: w, status: http.StatusOK}
	w = rec
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Request-ID", id)

	if !validHost(r.Host) {
		writeError(w, http.StatusMisdirectedRequest, "invalid host")
		h.log(id, r, rec.status)
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/health":
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case r.Method == http.MethodGet && r.URL.Path == "/ready":
		h.ready(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		if !h.authorized(r) {
			writeError(w, http.StatusUnauthorized, "invalid gateway key")
		} else {
			writeJSON(w, http.StatusOK, map[string]any{
				"object": "list",
				"data": []any{map[string]any{
					"id": config.Alias, "object": "model", "owned_by": "opentrinity",
				}},
			})
		}
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		h.completion(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/messages":
		h.messages(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/responses":
		h.responses(w, r)
	default:
		writeError(w, http.StatusNotFound, "unsupported route")
	}
	h.log(id, r, rec.status)
}

func (h *handler) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	if header := r.Header.Get("Authorization"); len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		provided := []byte(header[len(prefix):])
		if subtle.ConstantTimeCompare(provided, h.apiKey) == 1 {
			return true
		}
	}
	// Claude Code sends its gateway credential in X-Api-Key by default.
	if provided := []byte(r.Header.Get("X-Api-Key")); len(provided) > 0 {
		return subtle.ConstantTimeCompare(provided, h.apiKey) == 1
	}
	return false
}

func (h *handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://127.0.0.1:"+strconv.Itoa(config.LiteLLMPort)+"/health/liveliness", nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "starting"})
		return
	}
	resp, err := h.readyHTTP.Do(req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "starting"})
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "starting"})
}

func (h *handler) completion(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeError(w, http.StatusUnauthorized, "invalid gateway key")
		return
	}
	current := atomic.LoadInt64(&h.inFlight)
	if current >= maxConcurrent {
		writeError(w, http.StatusTooManyRequests, "concurrency limit reached")
		return
	}
	atomic.AddInt64(&h.inFlight, 1)
	defer atomic.AddInt64(&h.inFlight, -1)

	if r.ContentLength > maxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || probe.Model != config.Alias {
		// Unknown aliases and malformed bodies are rejected before forwarding.
		writeError(w, http.StatusBadRequest, "unsupported model or invalid request")
		return
	}

	out := r.Clone(r.Context())
	out.Body = io.NopCloser(bytes.NewReader(body))
	out.ContentLength = int64(len(body))
	out.Header = r.Header.Clone()
	h.proxy.ServeHTTP(w, out)
}

// responses accepts Codex CLI's OpenAI Responses API calls and forwards them
// to LiteLLM. The gateway key, concurrency limit and alias allowlist still
// apply before any upstream request is made.
func (h *handler) responses(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeError(w, http.StatusUnauthorized, "invalid gateway key")
		return
	}
	current := atomic.LoadInt64(&h.inFlight)
	if current >= maxConcurrent {
		writeError(w, http.StatusTooManyRequests, "concurrency limit reached")
		return
	}
	atomic.AddInt64(&h.inFlight, 1)
	defer atomic.AddInt64(&h.inFlight, -1)

	if r.ContentLength > maxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || probe.Model != config.Alias {
		writeError(w, http.StatusBadRequest, "unsupported model or invalid request")
		return
	}

	out := r.Clone(r.Context())
	out.Body = io.NopCloser(bytes.NewReader(body))
	out.ContentLength = int64(len(body))
	out.Header = r.Header.Clone()
	h.proxy.ServeHTTP(w, out)
}

// messages accepts Claude Code's Anthropic Messages API calls and forwards
// them to LiteLLM's Anthropic-compatible endpoint. The client still uses the
// same gateway key, concurrency limit and alias allowlist as chat completions.
func (h *handler) messages(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeError(w, http.StatusUnauthorized, "invalid gateway key")
		return
	}
	current := atomic.LoadInt64(&h.inFlight)
	if current >= maxConcurrent {
		writeError(w, http.StatusTooManyRequests, "concurrency limit reached")
		return
	}
	atomic.AddInt64(&h.inFlight, 1)
	defer atomic.AddInt64(&h.inFlight, -1)

	if r.ContentLength > maxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || probe.Model != config.Alias {
		writeError(w, http.StatusBadRequest, "unsupported model or invalid request")
		return
	}

	out := r.Clone(r.Context())
	out.Body = io.NopCloser(bytes.NewReader(body))
	out.ContentLength = int64(len(body))
	out.Header = r.Header.Clone()
	h.proxy.ServeHTTP(w, out)
}

func sanitizeResponse(resp *http.Response) {
	resp.Header.Del("Server")
	resp.Header.Del("Set-Cookie")
	resp.Header.Del("WWW-Authenticate")
	for k := range resp.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-litellm-") {
			resp.Header.Del(k)
		}
	}
	if resp.StatusCode < 400 {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	_ = resp.Body.Close()
	resp.StatusCode = http.StatusBadGateway
	resp.Status = "502 Bad Gateway"
	bodyText := `{"error":{"message":"upstream request failed","type":"server_error"}}`
	if resp.Request != nil && strings.HasSuffix(resp.Request.URL.Path, "/v1/messages") {
		bodyText = `{"type":"error","error":{"type":"api_error","message":"upstream request failed"}}`
	}
	resp.Body = io.NopCloser(strings.NewReader(bodyText))
	resp.Header.Set("Content-Type", "application/json")
	resp.ContentLength = int64(len(bodyText))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{"message": message, "type": "invalid_request_error"},
	})
}

func validHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil || h != "127.0.0.1" {
		return false
	}
	port, err := strconv.Atoi(requirePort(host))
	return err == nil && port == config.GatewayPort
}

func requirePort(hostport string) string {
	_, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return ""
	}
	return port
}

func closeClientConnection(w http.ResponseWriter) {
	if unwrapper, ok := w.(interface{ Unwrap() http.ResponseWriter }); ok {
		w = unwrapper.Unwrap()
	}
	if hj, ok := w.(http.Hijacker); ok {
		conn, _, err := hj.Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}
}

func (h *handler) log(id string, r *http.Request, status int) {
	h.logger.Printf("request_id=%s method=%s path=%s status=%d", id, r.Method, r.URL.Path, status)
}

// commitWriter records whether any response byte has been sent to the client.
// Once committed, no error body may be appended and no fallback may be started.
type commitWriter struct {
	http.ResponseWriter
	committed bool
	status    int
}

func (w *commitWriter) WriteHeader(code int) {
	w.committed = true
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *commitWriter) Write(p []byte) (int, error) {
	w.committed = true
	return w.ResponseWriter.Write(p)
}

func (w *commitWriter) Flush() {
	w.committed = true
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *commitWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
