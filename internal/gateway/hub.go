// Package gateway is the websocket layer: an in-memory hub that
// fans out messages to clients subscribed to a channel. writes still
// happen over REST, the hub only devlivers
package gateway

import "sync"

// Hub tracks channel subs and broadcasts to subs
type Hub struct {
	mu       sync.RWMutex
	channels map[int64]map[*Client]struct{} // channelID -> subs
}

func NewHub() *Hub {
	return &Hub{channels: make(map[int64]map[*Client]struct{})}
}

func (h *Hub) subscribe(c *Client, channelID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.channels[channelID] == nil {
		h.channels[channelID] = make(map[*Client]struct{})
	}
	h.channels[channelID][c] = struct{}{}
	c.subs[channelID] = struct{}{}
}

func (h *Hub) unsubscribe(c *Client, channelID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drop(c, channelID)
}

func (h *Hub) removeClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for channelID := range c.subs {
		h.drop(c, channelID)
	}
}

// drop remove on sub. caller must hold write lock
func (h *Hub) drop(c *Client, channelID int64) {
	if set := h.channels[channelID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.channels, channelID)
		}
	}
	delete(c.subs, channelID)
}

// Publish delviers payload to every sub of channelID
// Delivery is non-blocking. a client whoser buffer is full is
// disconnected (it can reconnect and refetch history)
func (h *Hub) Publish(channelID int64, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.channels[channelID] {
		c.enqueue(payload)
	}
}
