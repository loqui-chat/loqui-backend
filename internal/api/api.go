// Package api serves the http api
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/loqui-chat/loqui-backend/internal/auth"
	"github.com/loqui-chat/loqui-backend/internal/channel"
	"github.com/loqui-chat/loqui-backend/internal/message"
	"github.com/loqui-chat/loqui-backend/internal/user"
)

type Server struct {
	log       *slog.Logger
	pool      *pgxpool.Pool
	users     *user.Store
	channels  *channel.Store
	messages  *message.Store
	tokens    *auth.Issuer
	dummyHash string // used to keep login timing steady for unknown users
}

func NewServer(log *slog.Logger, pool *pgxpool.Pool, users *user.Store, channels *channel.Store, messages *message.Store, tokens *auth.Issuer) *Server {
	dummy, _ := auth.HashPassword("timing-equalizer")
	return &Server{log: log, pool: pool, users: users, channels: channels, messages: messages, tokens: tokens, dummyHash: dummy}
}

// Routes returns http handler for api
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /register", s.handleRegister)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.Handle("GET /me", s.requireAuth(http.HandlerFunc(s.handleMe)))

	mux.Handle("POST /channels", s.requireAuth(http.HandlerFunc(s.handleCreateChannel)))
	mux.Handle("GET /channels", s.requireAuth(http.HandlerFunc(s.handleListChannels)))
	mux.Handle("GET /channels/{id}", s.requireAuth(http.HandlerFunc(s.handleGetChannel)))

	mux.Handle("POST /channels/{id}/messages", s.requireAuth(http.HandlerFunc(s.handleCreateMessage)))
	mux.Handle("GET /channels/{id}/messages", s.requireAuth(http.HandlerFunc(s.handleListMessages)))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.pool.Ping(ctx); err != nil {
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// ==== json helpers ====

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Context-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return err
	}
	return nil
}

// ==== auth middleware ====

type ctxKey int

const userIDKey ctxKey = 0

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := s.tokens.Parse(token)
		if err != nil || claims.Type != auth.AccessToken {
			writeError(w, http.StatusUnauthorized, "invaild token")
			return
		}
		id, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invaild token")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey, id)))
	})
}

func userIDFrom(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}
