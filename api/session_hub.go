package api

import (
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// SessionHub manages active WebSocket connections keyed by child ID.
// When a session expires the expiry worker calls BroadcastToChild so the
// app receives the SESSION_EXPIRED event over the open socket in addition
// to the OneSignal push notification.
type SessionHub struct {
	mu    sync.RWMutex
	conns map[string]*websocket.Conn // child_id → conn
}

func NewSessionHub() *SessionHub {
	return &SessionHub{
		conns: make(map[string]*websocket.Conn),
	}
}

// Register stores a new WebSocket connection for a child.
// If the child already has a connection open it is closed first.
func (h *SessionHub) Register(childID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if old, ok := h.conns[childID]; ok {
		old.Close()
	}
	h.conns[childID] = conn
	log.Info().Str("child_id", childID).Msg("session hub: child connected")
}

// Unregister removes a child's connection from the hub.
func (h *SessionHub) Unregister(childID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.conns, childID)
	log.Info().Str("child_id", childID).Msg("session hub: child disconnected")
}

// BroadcastToChild sends a JSON message to a specific child's WebSocket
// connection. Returns silently if the child is not connected.
func (h *SessionHub) BroadcastToChild(childID string, msg interface{}) {
	h.mu.RLock()
	conn, ok := h.conns[childID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	if err := conn.WriteJSON(msg); err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("session hub: failed to send event, removing connection")
		h.Unregister(childID)
	}
}
