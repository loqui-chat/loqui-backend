package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/loqui-chat/loqui-backend/internal/channel"
)

type createChannelRequest struct {
	Name string `json:"name"`
}

type channelResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func toChannelResponse(c *channel.Channel) channelResponse {
	return channelResponse{
		ID:        strconv.FormatInt(c.ID, 10),
		Name:      c.Name,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// validateChannelName trims and checls length (1-100 runes) and control chars
func validateChannelName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 100 {
		return "", false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return name, true
}

func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	var req createChannelRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	name, ok := validateChannelName(req.Name)
	if !ok {
		writeError(w, http.StatusBadRequest, "channel name must be 1-100 characters with no control characters")
		return
	}
	c, err := s.channels.Create(r.Context(), name)
	if err != nil {
		s.log.Error("create channel", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, toChannelResponse(c))
}

func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	list, err := s.channels.List(r.Context())
	if err != nil {
		s.log.Error("list channels", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]channelResponse, 0, len(list))
	for i := range list {
		out = append(out, toChannelResponse(&list[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel id")
		return
	}
	c, err := s.channels.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, channel.ErrNotFound) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		s.log.Error("get channel", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toChannelResponse(c))
}
