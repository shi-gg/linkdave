package voice

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/disgoorg/snowflake/v2"
	"github.com/shi-gg/linkdave/server/audio"
	"github.com/shi-gg/linkdave/server/protocol"
)

func connectionKey(clientId, guildID snowflake.ID) string {
	return fmt.Sprintf("%s:%s", clientId, guildID)
}

type EventHandler interface {
	OnTrackEnd(clientId, guildID snowflake.ID, source audio.Source, reason string)
	OnTrackException(clientId, guildID snowflake.ID, source audio.Source, err error)
	OnVoiceDisconnected(clientId, guildID snowflake.ID)
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

func (m *Manager) onTrackEnd(clientId, guildID snowflake.ID, source audio.Source, reason string, err error) {
	m.mutex.RLock()
	handler := m.eventHandler
	m.mutex.RUnlock()

	if handler == nil {
		return
	}

	if reason == protocol.TrackEndReasonError {
		handler.OnTrackException(clientId, guildID, source, err)
	}

	handler.OnTrackEnd(clientId, guildID, source, reason)
}

func (m *Manager) onDisconnect(clientId, guildID snowflake.ID, conn *Connection, key string) {
	m.mutex.Lock()
	if m.connections[key] == conn {
		delete(m.connections, key)
	}
	handler := m.eventHandler
	m.mutex.RUnlock()

	if handler != nil {
		handler.OnVoiceDisconnected(clientId, guildID)
	}
}

func (m *Manager) Connect(ctx context.Context, clientId, guildID, channelID snowflake.ID, sessionID string, event protocol.VoiceServerEvent) error {
	m.mutex.Lock()
	key := connectionKey(clientId, guildID)
	existing, ok := m.connections[key]
	m.mutex.Unlock()

	if ok {
		return existing.HandleVoiceUpdate(ctx, channelID, sessionID, event)
	}

	var conn *Connection
	conn, err := NewConnection(ctx, m.logger, clientId, guildID, channelID, sessionID, event, func(source audio.Source, reason string, err error) {
		m.onTrackEnd(clientId, guildID, source, reason, err)
	}, func() {
		m.onDisconnect(clientId, guildID, conn, key)
	})

	if err != nil {
		return fmt.Errorf("failed to create voice connection: %w", err)
	}

	m.mutex.Lock()
	if existing, ok := m.connections[key]; ok {
		m.mutex.Unlock()
		conn.Close()
		return existing.HandleVoiceUpdate(ctx, channelID, sessionID, event)
	}
	m.connections[key] = conn
	m.mutex.Unlock()

	return nil
}

func (m *Manager) getConnection(clientId, guildID snowflake.ID) *Connection {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.connections[connectionKey(clientId, guildID)]
}

func (m *Manager) Play(ctx context.Context, clientId, guildID snowflake.ID, url string, startTime int64) (audio.Source, error) {
	conn := m.getConnection(clientId, guildID)
	if conn == nil {
		return nil, fmt.Errorf("no voice connection for guild %s", guildID)
	}

	factory := audio.NewDefaultFactory()
	source, err := factory.CreateFromURL(ctx, url, startTime)
	if err != nil {
		return nil, fmt.Errorf("failed to create audio source: %w", err)
	}

	if err := conn.Play(ctx, source); err != nil {
		source.Close()
		return nil, err
	}

	return source, nil
}

func (m *Manager) Pause(clientId, guildID snowflake.ID) error {
	conn := m.getConnection(clientId, guildID)
	if conn == nil {
		return fmt.Errorf("no voice connection for guild %s", guildID)
	}

	conn.Pause()
	return nil
}

func (m *Manager) Resume(clientId, guildID snowflake.ID) error {
	conn := m.getConnection(clientId, guildID)
	if conn == nil {
		return fmt.Errorf("no voice connection for guild %s", guildID)
	}

	conn.Resume()
	return nil
}

func (m *Manager) Stop(clientId, guildID snowflake.ID) error {
	conn := m.getConnection(clientId, guildID)
	if conn == nil {
		return fmt.Errorf("no voice connection for guild %s", guildID)
	}

	conn.Stop()
	return nil
}

func (m *Manager) Seek(clientId, guildID snowflake.ID, position int64) error {
	conn := m.getConnection(clientId, guildID)
	if conn == nil {
		return fmt.Errorf("no voice connection for guild %s", guildID)
	}

	return conn.SeekTo(position)
}

func (m *Manager) Position(clientId, guildID snowflake.ID) int64 {
	conn := m.getConnection(clientId, guildID)
	if conn == nil {
		return 0
	}

	return conn.Position()
}

func (m *Manager) Disconnect(clientId, guildID snowflake.ID) error {
	key := connectionKey(clientId, guildID)

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
			slog.String("bot_id", clientId.String()),
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
