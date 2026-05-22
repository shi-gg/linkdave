package voice

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/disgoorg/snowflake/v2"
	"github.com/shi-gg/linkdave/server/audio/filter"
	"github.com/shi-gg/linkdave/server/audio/source"
	"github.com/shi-gg/linkdave/server/protocol"
)

func connectionKey(sessionID string, guildID snowflake.ID) string {
	return sessionID + ":" + guildID.String()
}

type EventHandler interface {
	OnTrackEnd(sessionID string, guildID snowflake.ID, src source.Source, reason string)
	OnTrackException(sessionID string, guildID snowflake.ID, src source.Source, err error)
	OnVoiceDisconnected(sessionID string, guildID snowflake.ID)
}

type Manager struct {
	logger       *slog.Logger
	connections  map[string]*Connection
	mutex        sync.RWMutex
	eventHandler EventHandler
}

func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		logger:      logger,
		connections: make(map[string]*Connection),
	}
}

func (m *Manager) SetEventHandler(handler EventHandler) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.eventHandler = handler
}

func (m *Manager) onTrackEnd(sessionID string, guildID snowflake.ID, src source.Source, reason string, err error) {
	m.mutex.RLock()
	handler := m.eventHandler
	m.mutex.RUnlock()

	if handler == nil {
		return
	}

	if reason == protocol.TrackEndReasonError {
		handler.OnTrackException(sessionID, guildID, src, err)
	}

	handler.OnTrackEnd(sessionID, guildID, src, reason)
}

func (m *Manager) onDisconnect(sessionID string, guildID snowflake.ID, conn *Connection, key string) {
	m.mutex.Lock()
	if m.connections[key] != conn {
		m.mutex.Unlock()
		return
	}
	delete(m.connections, key)
	handler := m.eventHandler
	m.mutex.Unlock()

	if handler != nil {
		handler.OnVoiceDisconnected(sessionID, guildID)
	}
}

func (m *Manager) Connect(ctx context.Context, sessionID string, userID, guildID, channelID snowflake.ID, discordSessionID string, event protocol.VoiceServerEvent) error {
	m.mutex.Lock()
	key := connectionKey(sessionID, guildID)
	existing, ok := m.connections[key]
	m.mutex.Unlock()

	if ok {
		return existing.HandleVoiceUpdate(ctx, channelID, discordSessionID, event)
	}

	var conn *Connection
	conn, err := NewConnection(ctx, m.logger, userID, guildID, channelID, discordSessionID, event, func(src source.Source, reason string, err error) {
		m.onTrackEnd(sessionID, guildID, src, reason, err)
	}, func() {
		m.onDisconnect(sessionID, guildID, conn, key)
	})

	if err != nil {
		return fmt.Errorf("failed to create voice connection: %w", err)
	}

	m.mutex.Lock()
	if existing, ok := m.connections[key]; ok {
		m.mutex.Unlock()
		conn.Close()
		return existing.HandleVoiceUpdate(ctx, channelID, discordSessionID, event)
	}
	m.connections[key] = conn
	m.mutex.Unlock()

	return nil
}

func (m *Manager) getConnection(sessionID string, guildID snowflake.ID) *Connection {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.connections[connectionKey(sessionID, guildID)]
}

func (m *Manager) Play(ctx context.Context, sessionID string, guildID snowflake.ID, url string, startTime int64, filters *filter.Filters) (source.Source, error) {
	conn := m.getConnection(sessionID, guildID)
	if conn == nil {
		return nil, fmt.Errorf("no voice connection for guild %s", guildID)
	}

	factory := source.NewDefaultFactory()
	src, err := factory.CreateFromURL(ctx, url, startTime, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to create audio source: %w", err)
	}

	if err := conn.Play(ctx, src); err != nil {
		src.Close()
		return nil, err
	}

	return src, nil
}

func (m *Manager) Pause(sessionID string, guildID snowflake.ID) error {
	conn := m.getConnection(sessionID, guildID)
	if conn == nil {
		return fmt.Errorf("no voice connection for guild %s", guildID)
	}

	conn.Pause()
	return nil
}

func (m *Manager) Resume(sessionID string, guildID snowflake.ID) error {
	conn := m.getConnection(sessionID, guildID)
	if conn == nil {
		return fmt.Errorf("no voice connection for guild %s", guildID)
	}

	conn.Resume()
	return nil
}

func (m *Manager) Stop(sessionID string, guildID snowflake.ID) error {
	conn := m.getConnection(sessionID, guildID)
	if conn == nil {
		return fmt.Errorf("no voice connection for guild %s", guildID)
	}

	conn.Stop()
	return nil
}

func (m *Manager) Seek(sessionID string, guildID snowflake.ID, position int64) error {
	conn := m.getConnection(sessionID, guildID)
	if conn == nil {
		return fmt.Errorf("no voice connection for guild %s", guildID)
	}

	return conn.SeekTo(position)
}

func (m *Manager) Position(sessionID string, guildID snowflake.ID) int64 {
	conn := m.getConnection(sessionID, guildID)
	if conn == nil {
		return 0
	}

	return conn.Position()
}

func (m *Manager) Disconnect(sessionID string, guildID snowflake.ID) error {
	key := connectionKey(sessionID, guildID)

	m.mutex.Lock()

	conn, ok := m.connections[key]
	if !ok {
		m.mutex.Unlock()
		return nil
	}

	delete(m.connections, key)
	m.mutex.Unlock()

	go func() {
		conn.Close()

		m.logger.Info("voice connection closed",
			slog.String("session", sessionID),
			slog.String("guild_id", guildID.String()),
		)
	}()

	return nil
}

func (m *Manager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for key, conn := range m.connections {
		conn.Close()
		delete(m.connections, key)
	}
}
