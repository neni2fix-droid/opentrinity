// Package control implements the read-only local status listener.
// Browser sessions, CSRF and state-changing endpoints are deferred.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nenifix/opentrinity/internal/config"
	"github.com/nenifix/opentrinity/internal/database"
)

// Run starts the control listener and blocks until interrupted.
func Run(paths config.Paths, logger *log.Logger) error {
	store, err := database.Open(paths.DB)
	if err != nil {
		return err
	}
	defer store.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		state, stateErr := config.LoadState(paths.State)
		status := map[string]any{
			"initialized": stateErr == nil,
			"ports": map[string]int{
				"gateway": config.GatewayPort,
				"litellm": config.LiteLLMPort,
				"control": config.ControlPort,
			},
			"litellm_ready": probe("http://127.0.0.1:"+strconv.Itoa(config.LiteLLMPort)+"/health/liveliness"),
		}
		if stateErr == nil {
			status["generation_id"] = state.GenerationID
			status["started_at"] = state.StartedAt
			status["pids"] = map[string]int{
				"supervisor": state.SupervisorPID,
				"gateway":    state.GatewayPID,
				"control":    state.ControlPID,
				"litellm":    state.LiteLLMPID,
			}
		}
		writeJSON(w, http.StatusOK, status)
	})
	mux.HandleFunc("/api/v1/generations", func(w http.ResponseWriter, r *http.Request) {
		generations, err := store.ListGenerations(20)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "generation query failed"})
			return
		}
		type item struct {
			ID        int64  `json:"id"`
			SHA256    string `json:"sha256"`
			CreatedAt string `json:"created_at"`
			Status    string `json:"status"`
		}
		out := make([]item, 0, len(generations))
		for _, g := range generations {
			out = append(out, item{g.ID, g.SHA256, g.CreatedAt, g.Status})
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out})
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || host != "127.0.0.1" {
			writeJSON(w, http.StatusMisdirectedRequest, map[string]any{"error": "invalid host"})
			return
		}
		mux.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:              "127.0.0.1:" + strconv.Itoa(config.ControlPort),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		logger.Printf("control listening on %s", srv.Addr)
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

func probe(url string) bool {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
