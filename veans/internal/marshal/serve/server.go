// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Package serve is Marshal's HTTP face: the board's webhook receiver, the
// JSON API the Workman frontend panels read, and the timer that runs the
// checks. It never writes to the repository.
package serve

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/veans/internal/client"
	"code.vikunja.io/veans/internal/marshal/engine"
	"code.vikunja.io/veans/internal/marshal/notify"
)

// Server serves the API and runs the watcher.
type Server struct {
	Engine *engine.Engine
	// Base is the integration branch branches are diffed against.
	Base   string
	Logger *log.Logger

	mu       sync.RWMutex
	lastTick *engine.TickResult
	authMu   sync.Mutex
	authed   map[string]authEntry
}

type authEntry struct {
	user    *client.User
	expires time.Time
}

// New wires a server around an engine.
func New(e *engine.Engine, base string, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &Server{Engine: e, Base: base, Logger: logger, authed: map[string]authEntry{}}
}

// Handler returns the routed handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		last := s.lastTick
		s.mu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "last_tick": last})
	})
	mux.HandleFunc("POST /webhooks/workman", s.handleWebhook)
	mux.Handle("GET /api/tasks/{id}/references", s.auth(s.handleTaskReferences))
	mux.Handle("GET /api/references", s.auth(s.handleReferences))
	mux.Handle("GET /api/health", s.auth(s.handleHealth))
	mux.Handle("GET /api/chokepoints", s.auth(s.handleChokepoints))
	mux.Handle("GET /api/workers", s.auth(s.handleWorkers))
	mux.Handle("GET /api/open", s.auth(s.handleOpen))
	mux.Handle("GET /api/reconcile", s.auth(s.handleReconcile))
	mux.Handle("GET /api/ledger", s.auth(s.handleLedger))
	return s.cors(mux)
}

// Run serves until ctx is done, ticking the watcher on the configured poll.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.Engine.Cfg.Serve.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		s.Logger.Printf("marshal: listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	go s.watch(ctx)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) watch(ctx context.Context) {
	s.tick(ctx)
	ticker := time.NewTicker(s.Engine.Cfg.Serve.Poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Server) tick(ctx context.Context) {
	res := s.Engine.Tick(ctx, s.Base)
	s.mu.Lock()
	s.lastTick = res
	s.mu.Unlock()
	if len(res.Errors) > 0 {
		s.Logger.Printf("marshal: tick errors: %s", strings.Join(res.Errors, "; "))
	} else {
		s.Logger.Printf("marshal: tick rev=%s broken=%d pastes=%d stale=%d strays=%d health_ok=%t notified=%d", short(res.Rev), res.Broken, res.Pastes, res.Stale, res.Strays, res.HealthOK, res.Notified)
	}
}

func short(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}

// handleWebhook verifies the board's HMAC and relays the event.
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	secret := s.Engine.Cfg.Serve.WebhookSecret
	if secret == "" {
		http.Error(w, "webhook secret not configured", http.StatusServiceUnavailable)
		return
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	got := strings.TrimSpace(r.Header.Get("X-Vikunja-Signature"))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(got)), []byte(want)) != 1 {
		s.Logger.Printf("marshal: webhook: bad signature from %s", r.RemoteAddr)
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	var d notify.Delivery
	if err := json.Unmarshal(body, &d); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	s.Engine.HandleDelivery(r.Context(), d)
	w.WriteHeader(http.StatusNoContent)
}

// auth accepts a Workman bearer token and validates it against the board,
// so the frontend panels reuse the user's own session.
func (s *Server) auth(next func(http.ResponseWriter, *http.Request, *client.User)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
		if tok == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			return
		}
		u, err := s.lookup(r.Context(), tok)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token not accepted by the board"})
			return
		}
		next(w, r, u)
	})
}

func (s *Server) lookup(ctx context.Context, tok string) (*client.User, error) {
	sum := sha256.Sum256([]byte(tok))
	key := hex.EncodeToString(sum[:])
	s.authMu.Lock()
	if e, ok := s.authed[key]; ok && time.Now().Before(e.expires) {
		s.authMu.Unlock()
		return e.user, nil
	}
	s.authMu.Unlock()
	c := client.New(s.Engine.Board.Cfg.Server, tok)
	u, err := c.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	s.authMu.Lock()
	s.authed[key] = authEntry{user: u, expires: time.Now().Add(time.Minute)}
	s.authMu.Unlock()
	return u, nil
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := s.Engine.Cfg.Serve.AllowOrigins
	if len(allowed) == 0 {
		allowed = []string{strings.TrimRight(s.Engine.Board.Cfg.Server, "/")}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		for _, a := range allowed {
			if a == "*" || strings.EqualFold(a, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				break
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleTaskReferences(w http.ResponseWriter, r *http.Request, _ *client.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad task id"})
		return
	}
	t, err := s.Engine.Board.Client.GetTask(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	res, err := s.Engine.ResolveTask(r.Context(), t)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleReferences(w http.ResponseWriter, r *http.Request, _ *client.User) {
	snap, err := s.Engine.Board.Snapshot(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	rep, err := s.Engine.References(r.Context(), snap)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request, _ *client.User) {
	snap, err := s.Engine.Board.Snapshot(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.Engine.Health(snap))
}

func (s *Server) handleChokepoints(w http.ResponseWriter, r *http.Request, _ *client.User) {
	snap, err := s.Engine.Board.Snapshot(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	rep, err := s.Engine.Chokepoints(snap)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleWorkers(w http.ResponseWriter, r *http.Request, _ *client.User) {
	res, err := s.Engine.Workers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request, _ *client.User) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}
	snap, err := s.Engine.Board.Snapshot(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	holders, err := s.Engine.OpenOnPath(snap, path)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "holders": holders})
}

func (s *Server) handleReconcile(w http.ResponseWriter, r *http.Request, _ *client.User) {
	snap, err := s.Engine.Board.Snapshot(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	checks, err := s.Engine.Reconcile(r.Context(), snap, s.Base, r.URL.Query().Get("branch"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"base": s.Base, "branches": checks})
}

func (s *Server) handleLedger(w http.ResponseWriter, r *http.Request, _ *client.User) {
	entries, err := s.Engine.Ledger.Read()
	if err != nil {
		writeError(w, err)
		return
	}
	limit := 200
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v < 5000 {
		limit = v
	}
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	//nolint:errchkjson // responses are plain structs and maps; a write error has no recipient left to tell
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	msg := err.Error()
	if strings.Contains(msg, "NOT_FOUND") || strings.Contains(msg, "not found") {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": fmt.Sprintf("%v", msg)})
}
