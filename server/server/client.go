package server

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/shi-gg/linkdave/server/protocol"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024 // 512KB
)

type Player struct {
	guildID    snowflake.ID
	channelID  snowflake.ID
	state      string
	currentURL string
	position   int64
	volume     int
	startedAt  time.Time
}

type Client struct {
	server     *Server
	conn       *websocket.Conn
	sendCh     chan any
	sessionID  string
	clientName string

	clientId   snowflake.ID
	identified bool

	players   map[snowflake.ID]*Player
	playersMu sync.RWMutex

	closeChan chan struct{}
	closeOnce sync.Once
}

func NewClient(server *Server, conn *websocket.Conn, clientName string) *Client {
	return &Client{
		server:     server,
		conn:       conn,
		sendCh:     make(chan any, 256),
		sessionID:  uuid.New().String(),
		clientName: clientName,
		players:    make(map[snowflake.ID]*Player),
		closeChan:  make(chan struct{}),
	}
}

func (c *Client) readPump() {
	defer func() {
		c.close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		msgType, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.server.logger.Error("websocket read error", slog.Any("error", err))
			}
			return
		}
		c.server.handleMessage(c, msgType, message)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case message, ok := <-c.sendCh:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			data, err := json.Marshal(message)
			if err != nil {
				c.server.logger.Error("failed to marshal message", slog.Any("error", err))
				continue
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				c.server.logger.Error("failed to write message", slog.Any("error", err))
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.closeChan:
			return
		}
	}
}

func (c *Client) send(msg protocol.Message) {
	select {
	case c.sendCh <- msg:
	default:
		c.server.logger.Warn("client send buffer full, dropping message")
	}
}

func (c *Client) close() {
	c.closeOnce.Do(func() {
		close(c.closeChan)
		c.conn.Close()
		c.server.unregisterClient(c)
		c.server.logger.Info("client disconnected", slog.String("session", c.sessionID))
	})
}

func (c *Client) getOrCreatePlayer(guildID snowflake.ID) *Player {
	c.playersMu.Lock()
	defer c.playersMu.Unlock()

	if player, ok := c.players[guildID]; ok {
		return player
	}

	player := &Player{
		guildID: guildID,
		state:   protocol.PlayerStateIdle,
		volume:  100,
	}
	c.players[guildID] = player
	return player
}

func (c *Client) getPlayer(guildID snowflake.ID) *Player {
	c.playersMu.RLock()
	defer c.playersMu.RUnlock()
	return c.players[guildID]
}

func (c *Client) removePlayer(guildID snowflake.ID) {
	c.playersMu.Lock()
	defer c.playersMu.Unlock()
	delete(c.players, guildID)
}
