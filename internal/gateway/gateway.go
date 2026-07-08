package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/loqui-chat/loqui-backend/internal/auth"
	"github.com/loqui-chat/loqui-backend/internal/channel"
)

const (
	Subprotocol = "loqui.v1"
	tokenPrefix = "access_token." // carries jwt Sec-WebSocket-Protocol
	readLimit   = 32 * 1024       // max inbound frame size
)

// Gateway upgrades http requests to WS and owns hub
type Gateway struct {
	hub      *Hub
	tokens   *auth.Issuer
	channels *channel.Store
	log      *slog.Logger
}

func New(tokens *auth.Issuer, channels *channel.Store, log *slog.Logger) *Gateway {
	return &Gateway{hub: NewHub(), tokens: tokens, channels: channels, log: log}
}

func (g *Gateway) Hub() *Hub { return g.hub }

// clientCommand is a frame sent by client
type clientCommand struct {
	Op        string `json:"op"` // subsribe | unsubscribe
	ChannelID string `json:"channel_id"`
}

// Handler auths via Sec-WebSocket-Protocol, then runs the connection
func (g *Gateway) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := tokenFromProtocol(r.Header.Get("Sec-WebSocket-Protocol"))
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		claims, err := g.tokens.Parse(token)
		if err != nil || claims.Type != auth.AccessToken {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		userID, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		// TODO: origin checks enabled by default
		// configure OriginPatterns for browser client, set here later
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{Subprotocol},
		})
		if err != nil {
			return // Accept already wrote response
		}
		defer func() { _ = conn.CloseNow() }()
		conn.SetReadLimit(readLimit)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := newClient(g.hub, conn, userID, cancel)
		defer g.hub.removeClient(c)

		writeDone := make(chan struct{})
		go func() {
			defer close(writeDone)
			c.writeLoop(ctx)
		}()

		c.enqueue(helloEvent(userID))
		g.readLoop(ctx, c) // blocks until client disconnects

		cancel()
		<-writeDone
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}
}

func (g *Gateway) readLoop(ctx context.Context, c *Client) {
	for {
		var cmd clientCommand
		if err := wsjson.Read(ctx, c.conn, &cmd); err != nil {
			c.cancel()
			return
		}
		g.handleCommand(ctx, c, cmd)
	}
}

func (g *Gateway) handleCommand(ctx context.Context, c *Client, cmd clientCommand) {
	switch cmd.Op {
	case "subsribe":
		id, err := strconv.ParseInt(cmd.ChannelID, 10, 64)
		if err != nil {
			c.enqueue(errorEvent("invalid channel_id"))
			return
		}
		if _, err := g.channels.GetByID(ctx, id); err != nil {
			c.enqueue(errorEvent("channel not found"))
			return
		}
		g.hub.subscribe(c, id)
		c.enqueue(ackEvent("subsribed", id))
	case "unsubscribe":
		id, err := strconv.ParseInt(cmd.ChannelID, 10, 64)
		if err != nil {
			c.enqueue(errorEvent("invalid channel_id"))
			return
		}
		g.hub.unsubscribe(c, id)
		c.enqueue(ackEvent("unsubscribed", id))
	default:
		c.enqueue(errorEvent("unknown op"))
	}
}

// tokenFromProtocol pulls jwt out of Sec-WebSocket-Protocol header
func tokenFromProtocol(header string) string {
	for _, part := range strings.Split(header, ",") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(part), tokenPrefix); ok {
			return rest
		}
	}
	return ""
}

// ==== server events ====

func helloEvent(userID int64) []byte {
	b, _ := json.Marshal(map[string]any{
		"op":                    "hello",
		"user_id":               strconv.FormatInt(userID, 10),
		"heartbeat_interval_ms": pingInterval.Milliseconds(),
	})
	return b
}

func ackEvent(op string, channelID int64) []byte {
	b, _ := json.Marshal(map[string]any{
		"op":         op,
		"channel_id": strconv.FormatInt(channelID, 10),
	})
	return b
}

func errorEvent(msg string) []byte {
	b, _ := json.Marshal(map[string]any{"op": "error", "message": msg})
	return b
}
