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
	mutex      sync.RWMutex
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

func (p *Player) GetState() string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.state
}

func (p *Player) SetState(state string) {
	p.mutex.Lock()
	p.state = state
	p.mutex.Unlock()
}

func (p *Player) GetCurrentURL() string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.currentURL
}

func (p *Player) SetCurrentURL(url string) {
	p.mutex.Lock()
	p.currentURL = url
	p.mutex.Unlock()
}

func (p *Player) GetPosition() int64 {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.position
}

func (p *Player) SetPosition(pos int64) {
	p.mutex.Lock()
	p.position = pos
	p.mutex.Unlock()
}

func (p *Player) GetVolume() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.volume
}

func (p *Player) SetVolume(vol int) {
	p.mutex.Lock()
	p.volume = vol
	p.mutex.Unlock()
}

func (p *Player) GetStartedAt() time.Time {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.startedAt
}

func (p *Player) SetStartedAt(t time.Time) {
	p.mutex.Lock()
	p.startedAt = t
	p.mutex.Unlock()
}

func (p *Player) GetChannelID() snowflake.ID {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.channelID
}

func (p *Player) SetChannelID(id snowflake.ID) {
	p.mutex.Lock()
	p.channelID = id
	p.mutex.Unlock()
}

func (p *Player) SetPlayingState(url string, position int64) {
	p.mutex.Lock()
	p.state = protocol.PlayerStatePlaying
	p.currentURL = url
	p.position = position
	p.startedAt = time.Now()
	p.mutex.Unlock()
}

func (p *Player) SetIdleState() {
	p.mutex.Lock()
	p.state = protocol.PlayerStateIdle
	p.currentURL = ""
	p.mutex.Unlock()
}

func (p *Player) SetPausedState(position int64) {
	p.mutex.Lock()
	p.state = protocol.PlayerStatePaused
	p.position = position
	p.mutex.Unlock()
}

func (p *Player) GetPlayerUpdateData() (state string, position int64, volume int) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.state, p.position, p.volume
}

func (p *Player) GetMigrateData() (url string, position int64, volume int, state string) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	calculatedPos := time.Since(p.startedAt).Milliseconds() + p.position
	return p.currentURL, calculatedPos, p.volume, p.state
}
