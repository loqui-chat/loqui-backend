package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/loqui-chat/loqui-backend/internal/auth"
	"github.com/loqui-chat/loqui-backend/internal/session"
	"github.com/loqui-chat/loqui-backend/internal/user"
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{2,32}$`)

type registerRequest struct {
	Username string  `json:"username"`
	Email    *string `json:"email"`
	Password string  `json:"password"`
}

type loginRequest struct {
	Identity string `json:"identity"` // name#xxxx or email
	Password string `json:"password"`
}

type refreshRequest struct {
	Refresh string `json:"refresh_token"`
}

type userResponse struct {
	ID            string  `json:"id"` // string: snowflakes exceed js safe ints
	Username      string  `json:"username"`
	Discriminator string  `json:"discriminator"`
	Handle        string  `json:"handle"`
	Email         *string `json:"email,omitempty"`
}

type authResponse struct {
	User    userResponse `json:"user"`
	Access  string       `json:"access_token"`
	Refresh string       `json:"refresh_token"`
}

func toUserResponse(u *user.User) userResponse {
	return userResponse{
		ID:            strconv.FormatInt(u.ID, 10),
		Username:      u.Username,
		Discriminator: u.Discriminator,
		Handle:        u.Username + "#" + u.Discriminator,
		Email:         u.Email,
	}
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}

	if !usernameRe.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "username must be 2-32 chars: letters, digits, . _ -")
		return
	}
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		switch {
		case email == "":
			req.Email = nil
		case !validEmail(email):
			writeError(w, http.StatusBadRequest, "invalid email")
			return
		default:
			req.Email = &email
		}
	}
	if n := len(req.Password); n < 8 || n > 128 {
		writeError(w, http.StatusBadRequest, "password must be 8-128 characters")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.log.Error("hash password", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	u, err := s.users.Create(r.Context(), req.Username, req.Email, hash)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrEmailTaken):
			writeError(w, http.StatusConflict, "email already registered")
		case errors.Is(err, user.ErrNoDiscriminator):
			writeError(w, http.StatusConflict, "username is full, pick another")
		default:
			s.log.Error("create user", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	s.issueAndRespond(w, r, u, http.StatusCreated)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}

	u, err := s.lookup(r.Context(), req.Identity)
	if err != nil {
		_, _ = auth.VerifyPassword(req.Password, s.dummyHash) // steady timing
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	ok, err := auth.VerifyPassword(req.Password, u.PasswordHash)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	s.issueAndRespond(w, r, u, http.StatusOK)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	issued, userID, err := s.sessions.Rotate(r.Context(), req.Refresh)
	if err != nil {
		if errors.Is(err, session.ErrReuseDetected) {
			s.log.Warn("refresh token reuse detected, session revoked")
		}
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	access, err := s.tokens.Issue(userID, auth.AccessToken)
	if err != nil {
		s.log.Error("issue access", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"access_token":  access,
		"refresh_token": issued.Token,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if err := s.sessions.Revoke(r.Context(), req.Refresh); err != nil {
		s.log.Error("revoke session", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id, _ := userIDFrom(r.Context())
	u, err := s.users.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(u))
}

// lookup resolves a login identity that is either an email or name#xxxx
func (s *Server) lookup(ctx context.Context, identity string) (*user.User, error) {
	identity = strings.TrimSpace(identity)
	if strings.Contains(identity, "@") {
		return s.users.GetByEmail(ctx, identity)
	}
	name, disc, ok := strings.Cut(identity, "#")
	if !ok {
		return nil, user.ErrNotFound
	}
	return s.users.GetByIdentity(ctx, name, disc)
}

func (s *Server) issueAndRespond(w http.ResponseWriter, r *http.Request, u *user.User, status int) {
	access, err := s.tokens.Issue(u.ID, auth.AccessToken)
	if err != nil {
		s.log.Error("issue access", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	issued, err := s.sessions.Issue(r.Context(), u.ID, r.UserAgent())
	if err != nil {
		s.log.Error("issue refresh", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, status, authResponse{User: toUserResponse(u), Access: access, Refresh: issued.Token})
}

func validEmail(email string) bool {
	at := strings.IndexByte(email, '@')
	return at > 0 && at < len(email)-1 && len(email) <= 254
}
