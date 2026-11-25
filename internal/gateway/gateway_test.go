package gateway

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func testHandler(t *testing.T, upstream http.Handler) *handler {
	t.Helper()
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)
	h, err := newHandler("local-gateway-key", "private-litellm-key", log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	h.proxy.Transport = server.Client().Transport
	// Director rewrites to 127.0.0.1:3889; override the dial target for tests.
	h.proxy.Director = func(req *http.Request) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(server.URL, "http://")
		req.Host = req.URL.Host
		req.Header.Set("Authorization", "Bearer private-litellm-key")
	}
	return h
}

func do(t *testing.T, h *handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "127.0.0.1:3888"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUnauthorizedRequestNeverReachesUpstream(t *testing.T) {
	calls := 0
	h := testHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	rec := do(t, h, http.MethodPost, "/v1/chat/completions", `{"model":"nenifix-coder"}`, nil)
	if rec.Code != http.StatusUnauthorized || calls != 0 {
		t.Fatalf("status=%d upstream calls=%d", rec.Code, calls)
	}
}

func TestAllowlistAndAliasRejectBeforeUpstream(t *testing.T) {
	calls := 0
	h := testHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	cases := []struct {
		path string
		body string
		want int
	}{
		{"/admin", "", http.StatusNotFound},
		{"/v1/chat/completions", `{"model":"other-model"}`, http.StatusBadRequest},
		{"/v1/chat/completions", `{broken`, http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := do(t, h, http.MethodPost, c.path, c.body, map[string]string{"Authorization": "Bearer local-gateway-key"})
		if rec.Code != c.want {
			t.Errorf("path=%s status=%d want=%d", c.path, rec.Code, c.want)
		}
	}
	if calls != 0 {
		t.Fatalf("upstream calls=%d, want 0", calls)
	}
}

func TestAnthropicMessagesRouteRewritesAndAuthorises(t *testing.T) {
	var gotPath string
	var gotAuth string
	h := testHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"nenifix-coder"}`)
	}))
	bad := do(t, h, http.MethodPost, "/v1/messages", `{"model":"nenifix-coder","max_tokens":16,"messages":[]}`, nil)
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorised status=%d", bad.Code)
	}
	wrong := do(t, h, http.MethodPost, "/v1/messages", `{"model":"other-model","max_tokens":16,"messages":[]}`, map[string]string{"Authorization": "Bearer local-gateway-key"})
	if wrong.Code != http.StatusBadRequest {
		t.Fatalf("wrong model status=%d", wrong.Code)
	}
	rec := do(t, h, http.MethodPost, "/v1/messages", `{"model":"nenifix-coder","max_tokens":16,"messages":[]}`, map[string]string{"Authorization": "Bearer local-gateway-key"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("upstream path=%q", gotPath)
	}
	if gotAuth != "Bearer private-litellm-key" {
		t.Fatalf("upstream auth=%q", gotAuth)
	}
	if !strings.Contains(rec.Body.String(), `"type":"message"`) {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestOversizedBodyRejectedBeforeUpstream(t *testing.T) {
	calls := 0
	h := testHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	body := `{"model":"nenifix-coder","padding":"` + strings.Repeat("x", maxBodyBytes+10) + `"}`
	rec := do(t, h, http.MethodPost, "/v1/chat/completions", body, map[string]string{"Authorization": "Bearer local-gateway-key"})
	if rec.Code != http.StatusRequestEntityTooLarge || calls != 0 {
		t.Fatalf("status=%d upstream calls=%d", rec.Code, calls)
	}
}

func TestUpstreamErrorBodyIsSanitized(t *testing.T) {
	h := testHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"error":{"message":"SECRET RAW UPSTREAM TEXT"}}`)
	}))
	rec := do(t, h, http.MethodPost, "/v1/chat/completions", `{"model":"nenifix-coder"}`, map[string]string{"Authorization": "Bearer local-gateway-key"})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status=%d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "SECRET RAW UPSTREAM TEXT") {
		t.Fatal("raw upstream error leaked")
	}
}

func TestStreamingChunksPassThroughInOrder(t *testing.T) {
	h := testHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		for i := 0; i < 3; i++ {
			_, _ = fmt.Fprintf(w, "data: chunk-%d\n\n", i)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	rec := do(t, h, http.MethodPost, "/v1/chat/completions", `{"model":"nenifix-coder","stream":true}`, map[string]string{"Authorization": "Bearer local-gateway-key"})
	want := "data: chunk-0\n\ndata: chunk-1\n\ndata: chunk-2\n\ndata: [DONE]\n\n"
	if rec.Body.String() != want {
		t.Fatalf("body=%q want=%q", rec.Body.String(), want)
	}
}

func TestConcurrencyLimitReturns429(t *testing.T) {
	h := testHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	const requests = 30
	statuses := make(chan int, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := do(t, h, http.MethodPost, "/v1/chat/completions", `{"model":"nenifix-coder"}`, map[string]string{"Authorization": "Bearer local-gateway-key"})
			statuses <- rec.Code
		}()
	}
	wg.Wait()
	close(statuses)
	limited := 0
	ok := 0
	for code := range statuses {
		if code == http.StatusTooManyRequests {
			limited++
		}
		if code == http.StatusOK {
			ok++
		}
	}
	if limited == 0 {
		t.Fatalf("no 429 responses; ok=%d limited=%d", ok, limited)
	}
	if ok == 0 {
		t.Fatalf("no successful responses; ok=%d limited=%d", ok, limited)
	}
}

func TestHostValidation(t *testing.T) {
	h := testHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Host = "evil.example:3888"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMisdirectedRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}
