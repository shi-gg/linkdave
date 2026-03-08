package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/gorilla/websocket"
	"github.com/shi-gg/linkdave/server/audio"
	"github.com/shi-gg/linkdave/server/protocol"
	"github.com/shi-gg/linkdave/server/voice"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type Server struct {
	logger       *slog.Logger
	voiceManager *voice.Manager
	clients      map[string]*Client
	clientsMu    sync.RWMutex
	startTime    time.Time
	draining     bool
	drainMu      sync.RWMutex
	version      string
	password     string
}

func NewServer(logger *slog.Logger, voiceManager *voice.Manager, version string, password string) *Server {
	s := &Server{
		logger:       logger,
		voiceManager: voiceManager,
		clients:      make(map[string]*Client),
		startTime:    time.Now(),
		version:      version,
		password:     password,
	}
	voiceManager.SetEventHandler(s)
	s.startTickers()
	return s
}

func (s *Server) startTickers() {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			s.sendStats()
		}
	}()
}

func (s *Server) sendStats() {
	stats := s.GetStats()
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for _, client := range s.clients {
		client.send(protocol.Message{
			Op:   protocol.OpStats,
			Data: stats,
		})
	}
}

func (s *Server) OnTrackEnd(sessionID string, guildID snowflake.ID, source audio.Source, reason string) {
	client := s.getClientBySession(sessionID)
	if client == nil {
		return
	}

	player := client.getPlayer(guildID)
	if player == nil {
		return
	}

	if reason != protocol.TrackEndReasonReplaced && reason != protocol.TrackEndReasonStopped {
		player.SetIdleState()
	}

	client.send(protocol.Message{
		Op: protocol.OpTrackEnd,
		Data: protocol.TrackEndData{
			GuildID: guildID,
			Track: protocol.TrackInfo{
				URL:      source.URL(),
				Duration: source.Duration(),
			},
			Reason: reason,
		},
	})
}

func (s *Server) OnTrackException(sessionID string, guildID snowflake.ID, source audio.Source, err error) {
	client := s.getClientBySession(sessionID)
	if client == nil {
		return
	}

	client.send(protocol.Message{
		Op: protocol.OpTrackError,
		Data: protocol.TrackErrorData{
			GuildID: guildID,
			Track: protocol.TrackInfo{
				URL:      source.URL(),
				Duration: source.Duration(),
			},
			Error: err.Error(),
		},
	})
}

func (s *Server) OnVoiceDisconnected(sessionID string, guildID snowflake.ID) {
	client := s.getClientBySession(sessionID)
	if client == nil {
		return
	}

	client.removePlayer(guildID)

	client.send(protocol.Message{
		Op: protocol.OpVoiceDisconnect,
		Data: protocol.VoiceDisconnectData{
			GuildID: guildID,
			Reason:  "connection_lost",
		},
	})
}

func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.IsDraining() {
		http.Error(w, "Node is draining", http.StatusServiceUnavailable)
		return
	}

	if s.password != "" && r.URL.Query().Get("password") != s.password {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	clientName := r.Header.Get("Client-Name")
	if clientName == "" {
		clientName = "unknown"
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("failed to upgrade websocket", slog.Any("error", err))
		return
	}

	client := NewClient(s, conn, clientName)
	s.registerClient(client)

	s.logger.Info("client connected",
		slog.String("client", clientName),
		slog.String("session", client.sessionID),
		slog.String("addr", r.RemoteAddr),
	)

	client.send(protocol.Message{
		Op: protocol.OpReady,
		Data: protocol.ReadyData{
			SessionID: client.sessionID,
			Resumed:   false,
		},
	})

	go client.readPump()
	go client.writePump()
}

func (s *Server) registerClient(client *Client) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	s.clients[client.sessionID] = client
}

func (s *Server) unregisterClient(client *Client) {
	s.clientsMu.Lock()
	delete(s.clients, client.sessionID)
	s.clientsMu.Unlock()

	// Clean up all voice connections for this client's players
	go client.destroyAllPlayers()
}

func (s *Server) getClientBySession(sessionID string) *Client {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	return s.clients[sessionID]
}

func (s *Server) handleMessage(client *Client, msgType int, data []byte) {
	if msgType != websocket.TextMessage {
		return
	}

	var msg struct {
		Op   uint8           `json:"op"`
		Data json.RawMessage `json:"d"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		s.logger.Error("failed to unmarshal message", slog.Any("error", err))
		return
	}

	switch msg.Op {
	case protocol.OpVoiceUpdate:
		s.handleVoiceUpdate(client, msg.Data)
	case protocol.OpPing:
		s.handlePing(client)
	case protocol.OpPlayerMigrate:
		s.handlePlayerMigrate(client, msg.Data)
	default:
		s.logger.Warn("unknown op code", slog.Uint64("op", uint64(msg.Op)))
	}
}

func (s *Server) handleVoiceUpdate(client *Client, data json.RawMessage) {
	var update protocol.VoiceUpdateData
	if err := json.Unmarshal(data, &update); err != nil {
		s.logger.Error("failed to unmarshal voice update", slog.Any("error", err))
		return
	}

	s.logger.Info("voice update received",
		slog.String("guild_id", update.GuildID.String()),
		slog.String("channel_id", update.ChannelID.String()),
	)

	player := client.getOrCreatePlayer(update.GuildID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := s.voiceManager.Connect(ctx, client.sessionID, update.ClientID, update.GuildID, update.ChannelID, update.SessionID, update.Event)
	if err != nil {
		s.logger.Error("failed to connect to voice", slog.Any("error", err))
		client.send(protocol.Message{
			Op: protocol.OpTrackError,
			Data: protocol.TrackErrorData{
				GuildID: update.GuildID,
				Error:   "failed to connect to voice: " + err.Error(),
			},
		})
		return
	}

	player.SetChannelID(update.ChannelID)

	client.send(protocol.Message{
		Op: protocol.OpVoiceConnect,
		Data: protocol.VoiceConnectData{
			GuildID:   update.GuildID,
			ChannelID: update.ChannelID,
		},
	})
}

func (s *Server) handlePing(client *Client) {
	client.send(protocol.Message{
		Op:   protocol.OpPong,
		Data: nil,
	})
}

func (s *Server) handlePlayerMigrate(client *Client, data json.RawMessage) {
	var migrate protocol.PlayerMigrateData
	if err := json.Unmarshal(data, &migrate); err != nil {
		s.logger.Error("failed to unmarshal player migrate", slog.Any("error", err))
		return
	}

	player := client.getPlayer(migrate.GuildID)
	if player == nil {
		s.logger.Warn("player not found for migration", slog.String("guild_id", migrate.GuildID.String()))
		return
	}

	url, position, volume, state := player.GetMigrateData()
	client.send(protocol.Message{
		Op: protocol.OpMigrateReady,
		Data: protocol.MigrateReadyData{
			GuildID:  migrate.GuildID,
			URL:      url,
			Position: position,
			Volume:   volume,
			State:    state,
		},
	})

	s.logger.Info("player migration state sent",
		slog.String("guild_id", migrate.GuildID.String()),
		slog.String("url", url),
	)

	// Remove the player so the old voice connection cleanup
	// doesn't send a misleading voiceDisconnect to the client.
	client.removePlayer(migrate.GuildID)
}

func (s *Server) GetStats() protocol.StatsData {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	s.drainMu.RLock()
	defer s.drainMu.RUnlock()

	var totalPlayers, playingTracks int
	for _, client := range s.clients {
		client.playersMu.RLock()
		totalPlayers += len(client.players)
		for _, p := range client.players {
			if p.GetState() == protocol.PlayerStatePlaying {
				playingTracks++
			}
		}
		client.playersMu.RUnlock()
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return protocol.StatsData{
		Players:       totalPlayers,
		PlayingTracks: playingTracks,
		Uptime:        time.Since(s.startTime).Milliseconds(),
		Memory:        m.Alloc,
		Draining:      s.draining,
	}
}

func (s *Server) IsDraining() bool {
	s.drainMu.RLock()
	defer s.drainMu.RUnlock()
	return s.draining
}

func (s *Server) Drain(reason string, deadlineMs int64) {
	s.drainMu.Lock()
	s.draining = true
	s.drainMu.Unlock()

	s.logger.Info("entering drain mode", slog.String("reason", reason), slog.Int64("deadline_ms", deadlineMs))

	s.clientsMu.RLock()
	for _, client := range s.clients {
		client.send(protocol.Message{
			Op: protocol.OpNodeDraining,
			Data: protocol.NodeDrainingData{
				Reason:     reason,
				DeadlineMs: deadlineMs,
			},
		})
	}
	s.clientsMu.RUnlock()
}

func (s *Server) PlayerCount() int {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	var total int
	for _, client := range s.clients {
		client.playersMu.RLock()
		total += len(client.players)
		client.playersMu.RUnlock()
	}
	return total
}
