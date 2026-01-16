package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/gorilla/websocket"
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
}

func NewServer(logger *slog.Logger, voiceManager *voice.Manager) *Server {
	return &Server{
		logger:       logger,
		voiceManager: voiceManager,
		clients:      make(map[string]*Client),
		startTime:    time.Now(),
	}
}

func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
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
	s.logger.Info("client connected", slog.String("client", clientName), slog.String("addr", r.RemoteAddr))

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
	defer s.clientsMu.Unlock()
	delete(s.clients, client.sessionID)
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
	case protocol.OpIdentify:
		s.handleIdentify(client, msg.Data)
	case protocol.OpVoiceUpdate:
		s.handleVoiceUpdate(client, msg.Data)
	case protocol.OpPlay:
		s.handlePlay(client, msg.Data)
	case protocol.OpPause:
		s.handlePause(client, msg.Data)
	case protocol.OpResume:
		s.handleResume(client, msg.Data)
	case protocol.OpStop:
		s.handleStop(client, msg.Data)
	case protocol.OpSeek:
		s.handleSeek(client, msg.Data)
	case protocol.OpDisconnect:
		s.handleDisconnect(client, msg.Data)
	case protocol.OpPing:
		s.handlePing(client)
	case protocol.OpVolume:
		s.handleVolume(client, msg.Data)
	case protocol.OpPlayerMigrate:
		s.handlePlayerMigrate(client, msg.Data)
	default:
		s.logger.Warn("unknown op code", slog.Uint64("op", uint64(msg.Op)))
	}
}

func (s *Server) handleIdentify(client *Client, data json.RawMessage) {
	var identify protocol.IdentifyData
	if err := json.Unmarshal(data, &identify); err != nil {
		s.logger.Error("failed to unmarshal identify", slog.Any("error", err))
		return
	}

	client.clientId = identify.ClientId
	client.identified = true

	s.registerClient(client)

	s.logger.Info("client identified",
		slog.String("session", client.sessionID),
		slog.String("bot_id", identify.ClientId.String()),
	)

	client.send(protocol.Message{
		Op: protocol.OpReady,
		Data: protocol.ReadyData{
			SessionID: client.sessionID,
			Resumed:   false,
		},
	})
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

	err := s.voiceManager.Connect(ctx, client.clientId, update.GuildID, update.ChannelID, update.SessionID, update.Event)
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

	player.channelID = update.ChannelID

	client.send(protocol.Message{
		Op: protocol.OpVoiceConnected,
		Data: protocol.VoiceConnectedData{
			GuildID:   update.GuildID,
			ChannelID: update.ChannelID,
		},
	})
}

func (s *Server) handlePlay(client *Client, data json.RawMessage) {
	var play protocol.PlayData
	if err := json.Unmarshal(data, &play); err != nil {
		s.logger.Error("failed to unmarshal play", slog.Any("error", err))
		return
	}

	player := client.getOrCreatePlayer(play.GuildID)
	if play.Volume > 0 {
		player.volume = play.Volume
	}

	s.logger.Info("play requested",
		slog.String("guild_id", play.GuildID.String()),
		slog.String("url", play.URL),
	)

	go func() {
		err := s.voiceManager.Play(context.Background(), client.clientId, play.GuildID, play.URL, play.StartTime)
		if err != nil {
			s.logger.Error("playback failed", slog.Any("error", err))
			client.send(protocol.Message{
				Op: protocol.OpTrackError,
				Data: protocol.TrackErrorData{
					GuildID: play.GuildID,
					Track:   protocol.TrackInfo{URL: play.URL},
					Error:   err.Error(),
				},
			})
			return
		}

		player.state = protocol.PlayerStatePlaying
		player.currentURL = play.URL

		client.send(protocol.Message{
			Op: protocol.OpTrackStart,
			Data: protocol.TrackStartData{
				GuildID: play.GuildID,
				Track:   protocol.TrackInfo{URL: play.URL},
			},
		})
	}()
}

func (s *Server) handlePause(client *Client, data json.RawMessage) {
	var guild protocol.GuildData
	if err := json.Unmarshal(data, &guild); err != nil {
		s.logger.Error("failed to unmarshal pause", slog.Any("error", err))
		return
	}

	player := client.getPlayer(guild.GuildID)
	if player == nil {
		return
	}

	if err := s.voiceManager.Pause(client.clientId, guild.GuildID); err != nil {
		s.logger.Error("failed to pause", slog.Any("error", err))
		return
	}

	player.state = protocol.PlayerStatePaused
}

func (s *Server) handleResume(client *Client, data json.RawMessage) {
	var guild protocol.GuildData
	if err := json.Unmarshal(data, &guild); err != nil {
		s.logger.Error("failed to unmarshal resume", slog.Any("error", err))
		return
	}

	player := client.getPlayer(guild.GuildID)
	if player == nil {
		return
	}

	if err := s.voiceManager.Resume(client.clientId, guild.GuildID); err != nil {
		s.logger.Error("failed to resume", slog.Any("error", err))
		return
	}

	player.state = protocol.PlayerStatePlaying
}

func (s *Server) handleStop(client *Client, data json.RawMessage) {
	var guild protocol.GuildData
	if err := json.Unmarshal(data, &guild); err != nil {
		s.logger.Error("failed to unmarshal stop", slog.Any("error", err))
		return
	}

	player := client.getPlayer(guild.GuildID)
	if player == nil {
		return
	}

	if err := s.voiceManager.Stop(client.clientId, guild.GuildID); err != nil {
		s.logger.Error("failed to stop", slog.Any("error", err))
		return
	}

	player.state = protocol.PlayerStateIdle
	player.currentURL = ""
}

func (s *Server) handleSeek(client *Client, data json.RawMessage) {
	var seek protocol.SeekData
	if err := json.Unmarshal(data, &seek); err != nil {
		s.logger.Error("failed to unmarshal seek", slog.Any("error", err))
		return
	}

	if err := s.voiceManager.Seek(client.clientId, seek.GuildID, seek.Position); err != nil {
		s.logger.Error("failed to seek", slog.Any("error", err))
	}
}

func (s *Server) handleDisconnect(client *Client, data json.RawMessage) {
	var guild protocol.GuildData
	if err := json.Unmarshal(data, &guild); err != nil {
		s.logger.Error("failed to unmarshal disconnect", slog.Any("error", err))
		return
	}
	s.logger.Info("processing disconnect op", slog.String("guild_id", guild.GuildID.String()))

	if err := s.voiceManager.Disconnect(client.clientId, guild.GuildID); err != nil {
		s.logger.Error("failed to disconnect", slog.Any("error", err))
	}

	client.removePlayer(guild.GuildID)

	client.send(protocol.Message{
		Op: protocol.OpVoiceDisconnected,
		Data: protocol.VoiceDisconnectedData{
			GuildID: guild.GuildID,
			Reason:  "requested",
		},
	})
}

func (s *Server) handlePing(client *Client) {
	client.send(protocol.Message{
		Op:   protocol.OpPong,
		Data: nil,
	})
}

func (s *Server) handleVolume(client *Client, data json.RawMessage) {
	var vol protocol.VolumeData
	if err := json.Unmarshal(data, &vol); err != nil {
		s.logger.Error("failed to unmarshal volume", slog.Any("error", err))
		return
	}

	player := client.getPlayer(vol.GuildID)
	if player == nil {
		return
	}

	player.volume = vol.Volume
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
			if p.state == protocol.PlayerStatePlaying {
				playingTracks++
			}
		}
		client.playersMu.RUnlock()
	}

	var memStats struct {
		Alloc      uint64
		TotalAlloc uint64
	}

	return protocol.StatsData{
		Players:       totalPlayers,
		PlayingTracks: playingTracks,
		Uptime:        time.Since(s.startTime).Milliseconds(),
		MemoryUsed:    memStats.Alloc,
		MemoryAlloc:   memStats.TotalAlloc,
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

	client.send(protocol.Message{
		Op: protocol.OpMigrateReady,
		Data: protocol.MigrateReadyData{
			GuildID:  migrate.GuildID,
			URL:      player.currentURL,
			Position: time.Since(player.startedAt).Milliseconds() + player.position,
			Volume:   player.volume,
			State:    player.state,
		},
	})

	s.logger.Info("player migration state sent",
		slog.String("guild_id", migrate.GuildID.String()),
		slog.String("url", player.currentURL),
	)
}

func ClientKey(clientId, guildID snowflake.ID) string {
	return clientId.String() + ":" + guildID.String()
}
