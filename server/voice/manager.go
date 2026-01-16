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

type Manager struct {
	logger      *slog.Logger
	connections map[string]*Connection
	mu          sync.RWMutex
}

func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		logger:      logger,
		connections: make(map[string]*Connection),
	}
}

func (m *Manager) Connect(ctx context.Context, clientId, guildID, channelID snowflake.ID, sessionID string, event protocol.VoiceServerEvent) error {
	key := connectionKey(clientId, guildID)

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.connections[key]; ok {
		existing.Close()
		delete(m.connections, key)
	}

	conn, err := NewConnection(ctx, m.logger, clientId, guildID, channelID, sessionID, event)
	if err != nil {
		return fmt.Errorf("failed to create voice connection: %w", err)
	}

	m.connections[key] = conn
	m.logger.Info("voice connection established",
		slog.String("bot_id", clientId.String()),
		slog.String("guild_id", guildID.String()),
		slog.String("channel_id", channelID.String()),
	)

	return nil
}

func (m *Manager) getConnection(clientId, guildID snowflake.ID) *Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connections[connectionKey(clientId, guildID)]
}

func (m *Manager) Play(ctx context.Context, clientId, guildID snowflake.ID, url string, startTime int64) error {
	conn := m.getConnection(clientId, guildID)
	if conn == nil {
		return fmt.Errorf("no voice connection for guild %s", guildID)
	}

	factory := audio.NewDefaultFactory()
	source, err := factory.CreateFromURL(ctx, url, startTime)
	if err != nil {
		return fmt.Errorf("failed to create audio source: %w", err)
	}

	return conn.Play(ctx, source)
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

func (m *Manager) Disconnect(clientId, guildID snowflake.ID) error {
	key := connectionKey(clientId, guildID)

	m.mu.Lock()

	conn, ok := m.connections[key]
	if !ok {
		m.mu.Unlock()
		return nil
	}

	delete(m.connections, key)
	m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, conn := range m.connections {
		conn.Close()
		delete(m.connections, key)
	}
}
