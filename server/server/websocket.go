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
	logger            *slog.Logger
	voiceManager      *voice.Manager
	clients           map[string]*Client
	clientsByClientId map[snowflake.ID][]*Client
	clientsMu         sync.RWMutex
	startTime         time.Time
	draining          bool
	drainMu           sync.RWMutex
}

func NewServer(logger *slog.Logger, voiceManager *voice.Manager) *Server {
	s := &Server{
		logger:            logger,
		voiceManager:      voiceManager,
		clients:           make(map[string]*Client),
		clientsByClientId: make(map[snowflake.ID][]*Client),
		startTime:         time.Now(),
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

func (s *Server) OnTrackEnd(clientId, guildID snowflake.ID, source audio.Source, reason string) {
	s.clientsMu.RLock()
	clients := s.clientsByClientId[clientId]
	s.clientsMu.RUnlock()

	for _, client := range clients {
		player := client.getPlayer(guildID)
		if player == nil {
			continue
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
}

func (s *Server) OnTrackException(clientId, guildID snowflake.ID, source audio.Source, err error) {
	s.clientsMu.RLock()
	clients := s.clientsByClientId[clientId]
	s.clientsMu.RUnlock()

	for _, client := range clients {
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
}

func (s *Server) OnVoiceDisconnected(clientId, guildID snowflake.ID) {
	s.clientsMu.RLock()
	clients := s.clientsByClientId[clientId]
	s.clientsMu.RUnlock()

	for _, client := range clients {
		client.removePlayer(guildID)

		client.send(protocol.Message{
			Op: protocol.OpVoiceDisconnect,
			Data: protocol.VoiceDisconnectData{
				GuildID: guildID,
				Reason:  "connection_lost",
			},
		})
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
	s.clientsByClientId[client.clientId] = append(s.clientsByClientId[client.clientId], client)
}

func (s *Server) unregisterClient(client *Client) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	delete(s.clients, client.sessionID)
	clients := s.clientsByClientId[client.clientId]
	for i, c := range clients {
		if c == client {
			s.clientsByClientId[client.clientId] = append(clients[:i], clients[i+1:]...)
			break
		}
	}
	if len(s.clientsByClientId[client.clientId]) == 0 {
		delete(s.clientsByClientId, client.clientId)
	}
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

	player.SetChannelID(update.ChannelID)

	client.send(protocol.Message{
		Op: protocol.OpVoiceConnect,
		Data: protocol.VoiceConnectData{
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
		player.SetVolume(play.Volume)
	}

	s.logger.Info("play requested",
		slog.String("guild_id", play.GuildID.String()),
		slog.String("url", play.URL),
	)

	go func() {
		source, err := s.voiceManager.Play(context.Background(), client.clientId, play.GuildID, play.URL, play.StartTime)
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

		player.SetPlayingState(play.URL, play.StartTime)

		state, position, volume := player.GetPlayerUpdateData()
		client.send(protocol.Message{
			Op: protocol.OpTrackStart,
			Data: protocol.TrackStartData{
				GuildID: play.GuildID,
				Track: protocol.TrackInfo{
					URL:      source.URL(),
					Duration: source.Duration(),
				},
			},
		})

		client.send(protocol.Message{
			Op: protocol.OpPlayerUpdate,
			Data: protocol.PlayerUpdateData{
				GuildID:  play.GuildID,
				State:    state,
				Position: position,
				Volume:   volume,
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

	player.SetPausedState(s.voiceManager.Position(client.clientId, guild.GuildID))

	state, position, volume := player.GetPlayerUpdateData()
	client.send(protocol.Message{
		Op: protocol.OpPlayerUpdate,
		Data: protocol.PlayerUpdateData{
			GuildID:  guild.GuildID,
			State:    state,
			Position: position,
			Volume:   volume,
		},
	})
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

	player.SetState(protocol.PlayerStatePlaying)
	player.SetStartedAt(time.Now())
	player.SetPosition(s.voiceManager.Position(client.clientId, guild.GuildID))

	state, position, volume := player.GetPlayerUpdateData()
	client.send(protocol.Message{
		Op: protocol.OpPlayerUpdate,
		Data: protocol.PlayerUpdateData{
			GuildID:  guild.GuildID,
			State:    state,
			Position: position,
			Volume:   volume,
		},
	})
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

	player.SetIdleState()

	state, _, volume := player.GetPlayerUpdateData()
	client.send(protocol.Message{
		Op: protocol.OpPlayerUpdate,
		Data: protocol.PlayerUpdateData{
			GuildID:  guild.GuildID,
			State:    state,
			Position: 0,
			Volume:   volume,
		},
	})
}

func (s *Server) handleSeek(client *Client, data json.RawMessage) {
	var seek protocol.SeekData
	if err := json.Unmarshal(data, &seek); err != nil {
		s.logger.Error("failed to unmarshal seek", slog.Any("error", err))
		return
	}

	player := client.getPlayer(seek.GuildID)
	if player == nil {
		return
	}

	if err := s.voiceManager.Seek(client.clientId, seek.GuildID, seek.Position); err != nil {
		s.logger.Error("failed to seek", slog.Any("error", err))
		return
	}

	player.SetPosition(s.voiceManager.Position(client.clientId, seek.GuildID))
	player.SetStartedAt(time.Now())

	state, position, volume := player.GetPlayerUpdateData()
	client.send(protocol.Message{
		Op: protocol.OpPlayerUpdate,
		Data: protocol.PlayerUpdateData{
			GuildID:  seek.GuildID,
			State:    state,
			Position: position,
			Volume:   volume,
		},
	})
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
		Op: protocol.OpVoiceDisconnect,
		Data: protocol.VoiceDisconnectData{
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

	player.SetVolume(vol.Volume)

	state, _, volume := player.GetPlayerUpdateData()
	client.send(protocol.Message{
		Op: protocol.OpPlayerUpdate,
		Data: protocol.PlayerUpdateData{
			GuildID:  vol.GuildID,
			State:    state,
			Position: s.voiceManager.Position(client.clientId, vol.GuildID),
			Volume:   volume,
		},
	})
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
		MemoryUsed:    m.Alloc,
		MemoryAlloc:   m.TotalAlloc,
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
}

func ClientKey(clientId, guildID snowflake.ID) string {
	return clientId.String() + ":" + guildID.String()
}
