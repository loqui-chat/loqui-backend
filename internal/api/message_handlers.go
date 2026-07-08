package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/loqui-chat/loqui-backend/internal/channel"
	"github.com/loqui-chat/loqui-backend/internal/message"
)

const maxMessageLen = 2000

type createMessageRequest struct {
	Content string `json:"content"`
}

type authorResponse struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	Handle        string `json:"handle"`
}

type messageResponse struct {
	ID        string         `json:"id"`
	ChannelID string         `json:"channel_id"`
	Content   string         `json:"content"`
	Author    authorResponse `json:"author"`
	EditedAt  *string        `json:"edited_at,omitempty"`
	CreatedAt string         `json:"created_at"`
}

type messageCreateEvent struct {
	Op   string          `json:"op"`
	Data messageResponse `json:"data"`
}

func toMessageResponse(m *message.Message) messageResponse {
	resp := messageResponse{
		ID:        strconv.FormatInt(m.ID, 10),
		ChannelID: strconv.FormatInt(m.ChannelID, 10),
		Content:   m.Content,
		Author: authorResponse{
			ID:            strconv.FormatInt(m.Author.ID, 10),
			Username:      m.Author.Username,
			Discriminator: m.Author.Discriminator,
			Handle:        m.Author.Username + "#" + m.Author.Discriminator,
		},
		CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
	}
	if m.EditedAt != nil {
		edited := m.EditedAt.UTC().Format(time.RFC3339)
		resp.EditedAt = &edited
	}
	return resp
}

// validateContent trims and checks length (1-2000 runes)
// control characters are rejected except newline and tab
func validateContent(content string) (string, bool) {
	content = strings.TrimSpace(content)
	n := utf8.RuneCountInString(content)
	if n < 1 || n > maxMessageLen {
		return "", false
	}
	for _, r := range content {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", false
		}
	}
	return content, true
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel id")
		return
	}

	var req createMessageRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	content, ok := validateContent(req.Content)
	if !ok {
		writeError(w, http.StatusBadRequest, "content must be 1-2000 characters with no control characters except newline and tab")
		return
	}

	if _, err := s.channels.GetByID(r.Context(), channelID); err != nil {
		if errors.Is(err, channel.ErrNotFound) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		s.log.Error("lookup channel", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	authorID, _ := userIDFrom(r.Context())
	m, err := s.messages.Create(r.Context(), channelID, authorID, content)
	if err != nil {
		s.log.Error("create message", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := toMessageResponse(m)
	if payload, err := json.Marshal(messageCreateEvent{Op: "message_create", Data: resp}); err == nil {
		s.gw.Hub().Publish(m.ChannelID, payload)
	} else {
		s.log.Error("marshal message event", "err", err)
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid channel id")
		return
	}

	q := r.URL.Query()
	before, err := optionalID(q.Get("before"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid before cursor")
		return
	}
	after, err := optionalID(q.Get("after"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid after cursor")
		return
	}
	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		if n > 100 {
			n = 100
		}
		limit = n
	}

	if _, err := s.channels.GetByID(r.Context(), channelID); err != nil {
		if errors.Is(err, channel.ErrNotFound) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		s.log.Error("lookup channel", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	list, err := s.messages.List(r.Context(), channelID, before, after, limit)
	if err != nil {
		s.log.Error("list messages", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]messageResponse, 0, len(list))
	for i := range list {
		out = append(out, toMessageResponse(&list[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// optionalID parses a snowflake cursor. empty return 0 (unset)
func optionalID(v string) (int64, error) {
	if v == "" {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}
