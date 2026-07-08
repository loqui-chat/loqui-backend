package gateway

import (
	"context"
	"time"

	"github.com/coder/websocket"
)

const (
	sendBuffer   = 64
	pingInterval = 30 * time.Second
	writeTimeout = 10 * time.Second
)

// Client is one websocket connection and its outbound queue
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	userID int64
	send   chan []byte
	subs   map[int64]struct{} // guarded by hub.mu
	cancel context.CancelFunc
}

func newClient(hub *Hub, conn *websocket.Conn, userID int64, cancel context.CancelFunc) *Client {
	return &Client{
		hub:    hub,
		conn:   conn,
		userID: userID,
		send:   make(chan []byte, sendBuffer),
		subs:   make(map[int64]struct{}),
		cancel: cancel,
	}
}

// enqueue queues a payload for writer, disconnecting client if its
// buffer is full
func (c *Client) enqueue(payload []byte) {
	select {
	case c.send <- payload:
	default:
		c.cancel()
	}
}

// writeLoop is the signle writer for this connection: outbound frams plus
// periodic pings. everything that sends must fot through c.send
func (c *Client) writeLoop(ctx context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Write(wctx, websocket.MessageText, payload)
			cancel()
			if err != nil {
				c.cancel()
				return
			}
		case <-ticker.C:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Ping(wctx)
			cancel()
			if err != nil {
				c.cancel()
				return
			}
		}
	}
}
