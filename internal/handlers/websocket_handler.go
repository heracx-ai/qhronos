package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
)

// WebSocket message types
const (
	wsTypeClientHook  = "client-hook"
	wsTypeTagListener = "tag-listener"
	wsTypeEvent       = "event"
	wsTypeAck         = "ack"
	wsTypeError       = "error"
)

type wsInitMessage struct {
	Type       string   `json:"type"`
	ClientName string   `json:"client_name,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Token      string   `json:"token"`
}

type wsEventMessage struct {
	Type         string      `json:"type"`
	EventID      string      `json:"event_id"`
	OccurrenceID string      `json:"occurrence_id"`
	Payload      interface{} `json:"payload"`
	Tags         []string    `json:"tags"`
}

type wsAckMessage struct {
	Type         string `json:"type"`
	EventID      string `json:"event_id"`
	OccurrenceID string `json:"occurrence_id"`
}

type wsErrorMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type roundRobinState struct {
	index int
}

type WebSocketHandler struct {
	DB *sqlx.DB
	// Add any required services (e.g., token validation, event/occurrence repo)
	// For demo, only DB is included

	// Registries
	clientHookConns  map[string]map[*websocket.Conn]struct{}
	tagListenerConns map[string]map[*websocket.Conn]struct{}
	mu               sync.RWMutex
	// Add round-robin state for each clientID
	rrState map[string]*roundRobinState
}

func NewWebSocketHandler(db *sqlx.DB) *WebSocketHandler {
	return &WebSocketHandler{
		DB:               db,
		clientHookConns:  make(map[string]map[*websocket.Conn]struct{}),
		tagListenerConns: make(map[string]map[*websocket.Conn]struct{}),
		rrState:          make(map[string]*roundRobinState),
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *WebSocketHandler) Handle(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Read initial handshake
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var init wsInitMessage
	if err := json.Unmarshal(msg, &init); err != nil {
		conn.WriteJSON(wsErrorMessage{Type: wsTypeError, Message: "invalid handshake message"})
		return
	}

	// TODO: Authenticate token (validate JWT or master token)
	if strings.TrimSpace(init.Token) == "" {
		conn.WriteJSON(wsErrorMessage{Type: wsTypeError, Message: "authentication required"})
		return
	}

	// Register connection
	subType := init.Type
	var subKey string
	if subType == wsTypeClientHook {
		if init.ClientName == "" {
			conn.WriteJSON(wsErrorMessage{Type: wsTypeError, Message: "client_name required for client-hook"})
			return
		}
		subKey = init.ClientName
		h.mu.Lock()
		if h.clientHookConns[subKey] == nil {
			h.clientHookConns[subKey] = make(map[*websocket.Conn]struct{})
		}
		h.clientHookConns[subKey][conn] = struct{}{}
		h.mu.Unlock()
	} else if subType == wsTypeTagListener {
		if len(init.Tags) == 0 {
			conn.WriteJSON(wsErrorMessage{Type: wsTypeError, Message: "tags required for tag-listener"})
			return
		}
		h.mu.Lock()
		for _, tag := range init.Tags {
			if h.tagListenerConns[tag] == nil {
				h.tagListenerConns[tag] = make(map[*websocket.Conn]struct{})
			}
			h.tagListenerConns[tag][conn] = struct{}{}
		}
		h.mu.Unlock()
	} else {
		conn.WriteJSON(wsErrorMessage{Type: wsTypeError, Message: "invalid type"})
		return
	}

	// Ack handshake
	conn.WriteJSON(map[string]interface{}{"type": "ack", "message": "subscription registered"})

	// Listen for messages (ack for client-hook, error for tag-listener)
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var base map[string]interface{}
		if err := json.Unmarshal(msg, &base); err != nil {
			conn.WriteJSON(wsErrorMessage{Type: wsTypeError, Message: "invalid message format"})
			continue
		}
		msgType, _ := base["type"].(string)
		if msgType == wsTypeAck {
			if subType != wsTypeClientHook {
				conn.WriteJSON(wsErrorMessage{Type: wsTypeError, Message: "acknowledge not supported for tag-listener"})
				continue
			}
			// TODO: Mark occurrence as acknowledged in DB using sqlx
			// Example: update occurrences set status='acknowledged' where id=?
			// (Parse event_id and occurrence_id from message)
			// Respond with success or error
			conn.WriteJSON(map[string]interface{}{"type": "ack", "message": "acknowledged"})
		} else {
			conn.WriteJSON(wsErrorMessage{Type: wsTypeError, Message: "unexpected message type after handshake"})
		}
	}

	// Cleanup on disconnect
	h.mu.Lock()
	if subType == wsTypeClientHook {
		if conns, ok := h.clientHookConns[subKey]; ok {
			delete(conns, conn)
			if len(conns) == 0 {
				delete(h.clientHookConns, subKey)
			}
		}
	} else if subType == wsTypeTagListener {
		for _, tag := range init.Tags {
			if conns, ok := h.tagListenerConns[tag]; ok {
				delete(conns, conn)
				if len(conns) == 0 {
					delete(h.tagListenerConns, tag)
				}
			}
		}
	}
	h.mu.Unlock()
}

// DispatchToClient sends the payload to one of the connected clients for the given clientID using round-robin
func (h *WebSocketHandler) DispatchToClient(clientID string, payload []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns, ok := h.clientHookConns[clientID]
	if !ok || len(conns) == 0 {
		return fmt.Errorf("no client connected for id: %s", clientID)
	}
	// Convert map to slice for round-robin
	var connList []*websocket.Conn
	for conn := range conns {
		connList = append(connList, conn)
	}
	if len(connList) == 0 {
		return fmt.Errorf("no client connected for id: %s", clientID)
	}
	// Get or create round-robin state
	rr, ok := h.rrState[clientID]
	if !ok {
		rr = &roundRobinState{index: 0}
		h.rrState[clientID] = rr
	}
	// Pick connection
	conn := connList[rr.index%len(connList)]
	err := conn.WriteMessage(websocket.TextMessage, payload)
	// Advance round-robin index
	rr.index = (rr.index + 1) % len(connList)
	return err
}

// TODO: Add methods to dispatch events to connections (to be called by event/occurrence logic)
