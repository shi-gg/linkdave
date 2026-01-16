package voice

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/shi-gg/linkdave/server/audio"
	"github.com/shi-gg/linkdave/server/protocol"
)

type Connection struct {
	logger    *slog.Logger
	guildID   snowflake.ID
	channelID snowflake.ID
	userID    snowflake.ID

	voiceConn voice.Conn

	source audio.Source
	paused atomic.Bool
	mu     sync.Mutex

	stopChan chan struct{}
}

func NewConnection(
	ctx context.Context,
	logger *slog.Logger,
	userID, guildID, channelID snowflake.ID,
	sessionID string,
	voiceServerEvent protocol.VoiceServerEvent,
) (*Connection, error) {
	conn := &Connection{
		logger:    logger,
		guildID:   guildID,
		channelID: channelID,
		userID:    userID,
		stopChan:  make(chan struct{}),
	}

	// Create disgo voice connection for standalone mode.
	voiceConn := voice.NewConn(
		guildID,
		userID,
		func(ctx context.Context, guildID snowflake.ID, channelID *snowflake.ID, selfMute, selfDeaf bool) error {
			return nil
		},
		func() {
			logger.Debug("voice connection removed from manager")
		},
		voice.WithConnLogger(logger),
	)

	// Provide voice events concurrently to avoid deadlocks/race conditions with Open
	endpoint := voiceServerEvent.Endpoint
	go func() {
		time.Sleep(50 * time.Millisecond)

		voiceConn.HandleVoiceStateUpdate(gateway.EventVoiceStateUpdate{
			VoiceState: discord.VoiceState{
				GuildID:   guildID,
				ChannelID: &channelID,
				UserID:    userID,
				SessionID: sessionID,
			},
		})

		voiceConn.HandleVoiceServerUpdate(gateway.EventVoiceServerUpdate{
			Token:    voiceServerEvent.Token,
			GuildID:  guildID,
			Endpoint: &endpoint,
		})
	}()

	if err := voiceConn.Open(ctx, channelID, false, false); err != nil {
		return nil, fmt.Errorf("failed to open voice connection: %w", err)
	}

	conn.voiceConn = voiceConn

	logger.Info("voice connection established",
		slog.String("guild_id", guildID.String()),
		slog.String("channel_id", channelID.String()),
		slog.String("endpoint", endpoint),
	)

	return conn, nil
}

func (c *Connection) Play(ctx context.Context, source audio.Source) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.source != nil {
		c.source.Close()
	}

	c.source = source
	c.paused.Store(false)

	select {
	case <-c.stopChan:
		c.stopChan = make(chan struct{})
	default:
	}

	c.voiceConn.SetOpusFrameProvider(source)

	c.logger.Debug("started playback",
		slog.String("guild_id", c.guildID.String()),
	)

	return nil
}

func (c *Connection) Pause() {
	c.paused.Store(true)
	c.voiceConn.SetOpusFrameProvider(nil)
}

func (c *Connection) Resume() {
	c.paused.Store(false)
	c.mu.Lock()
	source := c.source
	c.mu.Unlock()

	if source != nil {
		c.voiceConn.SetOpusFrameProvider(source)
	}
}

func (c *Connection) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.stopChan:
	default:
		close(c.stopChan)
	}

	if c.source != nil {
		c.source.Close()
		c.source = nil
	}

	c.voiceConn.SetOpusFrameProvider(nil)
}

func (c *Connection) SeekTo(positionMs int64) error {
	c.mu.Lock()
	source := c.source
	c.mu.Unlock()

	if source == nil {
		return fmt.Errorf("no active playback")
	}

	return source.SeekTo(positionMs)
}

func (c *Connection) Position() int64 {
	c.mu.Lock()
	source := c.source
	c.mu.Unlock()

	if source == nil {
		return 0
	}

	return source.Position()
}

func (c *Connection) Close() {
	c.Stop()

	// Close the disgo voice connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.voiceConn.Close(ctx)

	c.logger.Debug("voice connection closed",
		slog.String("guild_id", c.guildID.String()),
	)
}
