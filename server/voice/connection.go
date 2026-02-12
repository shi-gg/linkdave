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

	source       audio.Source
	onTrackEnd   func(source audio.Source, reason string, err error)
	onDisconnect func()
	paused       atomic.Bool
	closed       atomic.Bool
	mutex        sync.Mutex
	setupMu      sync.Mutex

	stopChan chan struct{}
}

func NewConnection(
	ctx context.Context,
	logger *slog.Logger,
	userID, guildID, channelID snowflake.ID,
	sessionID string,
	voiceServerEvent protocol.VoiceServerEvent,
	onTrackEnd func(source audio.Source, reason string, err error),
	onDisconnect func(),
) (*Connection, error) {
	conn := &Connection{
		logger:       logger,
		guildID:      guildID,
		channelID:    channelID,
		userID:       userID,
		onTrackEnd:   onTrackEnd,
		onDisconnect: onDisconnect,
		stopChan:     make(chan struct{}),
	}

	if err := conn.setupVoiceConn(ctx, channelID, sessionID, voiceServerEvent); err != nil {
		return nil, err
	}

	return conn, nil
}

func (c *Connection) setupVoiceConn(ctx context.Context, channelID snowflake.ID, sessionID string, event protocol.VoiceServerEvent) error {
	c.setupMu.Lock()
	defer c.setupMu.Unlock()

	if c.closed.Load() {
		return fmt.Errorf("connection closed")
	}

	var currentVoiceConn voice.Conn
	currentVoiceConn = voice.NewConn(
		c.guildID,
		c.userID,
		func(ctx context.Context, guildID snowflake.ID, channelID *snowflake.ID, selfMute, selfDeaf bool) error {
			return nil
		},
		func() {
			c.logger.Debug("voice connection removed from manager")

			if !c.closed.Load() || c.onDisconnect == nil {
				return
			}

			// Only trigger onDisconnect if this is the CURRENT voiceConn
			c.mutex.Lock()
			isCurrent := (c.voiceConn == currentVoiceConn)
			c.mutex.Unlock()

			if !isCurrent {
				return
			}

			c.onDisconnect()
		},
		voice.WithConnLogger(c.logger),
	)

	// Provide voice events concurrently to avoid deadlocks/race conditions with Open
	channelIDCopy := channelID
	endpointCopy := event.Endpoint
	go func() {
		time.Sleep(50 * time.Millisecond)

		currentVoiceConn.HandleVoiceStateUpdate(gateway.EventVoiceStateUpdate{
			VoiceState: discord.VoiceState{
				GuildID:   c.guildID,
				ChannelID: &channelIDCopy,
				UserID:    c.userID,
				SessionID: sessionID,
			},
		})

		currentVoiceConn.HandleVoiceServerUpdate(gateway.EventVoiceServerUpdate{
			Token:    event.Token,
			GuildID:  c.guildID,
			Endpoint: &endpointCopy,
		})
	}()

	if err := currentVoiceConn.Open(ctx, channelID, false, false); err != nil {
		return fmt.Errorf("failed to open voice connection: %w", err)
	}

	c.mutex.Lock()
	oldConn := c.voiceConn
	c.voiceConn = currentVoiceConn
	c.channelID = channelID
	source := c.source
	c.mutex.Unlock()

	if oldConn != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			oldConn.Close(ctx)
		}()
	}

	if source != nil {
		currentVoiceConn.SetOpusFrameProvider(&trackWrapper{
			source: source,
			conn:   c,
		})
	}

	return nil
}

func (c *Connection) HandleVoiceUpdate(ctx context.Context, channelID snowflake.ID, sessionID string, event protocol.VoiceServerEvent) error {
	c.logger.Info("handling voice update (channel move/server change)",
		slog.String("guild_id", c.guildID.String()),
		slog.String("new_channel_id", channelID.String()),
	)
	return c.setupVoiceConn(ctx, channelID, sessionID, event)
}

func (c *Connection) Play(ctx context.Context, source audio.Source) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.source != nil {
		oldSource := c.source
		c.source = nil
		oldSource.Close()

		if c.onTrackEnd != nil {
			go c.onTrackEnd(oldSource, protocol.TrackEndReasonReplaced, nil)
		}
	}

	c.source = source
	c.paused.Store(false)

	select {
	case <-c.stopChan:
		c.stopChan = make(chan struct{})
	default:
	}

	c.voiceConn.SetOpusFrameProvider(&trackWrapper{
		source: source,
		conn:   c,
	})

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
	c.mutex.Lock()
	source := c.source
	c.mutex.Unlock()

	if source != nil {
		c.voiceConn.SetOpusFrameProvider(&trackWrapper{
			source: source,
			conn:   c,
		})
	}
}

func (c *Connection) Stop() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	select {
	case <-c.stopChan:
	default:
		close(c.stopChan)
	}

	if c.source != nil {
		oldSource := c.source
		c.source = nil
		oldSource.Close()
		if c.onTrackEnd != nil {
			go c.onTrackEnd(oldSource, protocol.TrackEndReasonStopped, nil)
		}
	}

	c.voiceConn.SetOpusFrameProvider(nil)
}

func (c *Connection) handleTrackEnd(source audio.Source, err error) {
	c.mutex.Lock()
	if c.source != source {
		c.mutex.Unlock()
		return
	}
	c.source = nil
	c.mutex.Unlock()

	reason := protocol.TrackEndReasonFinished
	if err != nil && err != audio.ErrEOF {
		reason = protocol.TrackEndReasonError
	}

	if c.onTrackEnd != nil {
		c.onTrackEnd(source, reason, err)
	}
}

type trackWrapper struct {
	source audio.Source
	conn   *Connection
}

func (w *trackWrapper) ProvideOpusFrame() ([]byte, error) {
	return w.conn.provideOpusFrame(w.source)
}

func (w *trackWrapper) Close() {
	w.source.Close()
}

func (c *Connection) provideOpusFrame(source audio.Source) ([]byte, error) {
	frame, err := source.ProvideOpusFrame()
	if err != nil {
		c.handleTrackEnd(source, err)
	}
	return frame, err
}

func (c *Connection) SeekTo(positionMs int64) error {
	c.mutex.Lock()
	source := c.source
	c.mutex.Unlock()

	if source == nil {
		return fmt.Errorf("no active playback")
	}

	return source.SeekTo(positionMs)
}

func (c *Connection) Position() int64 {
	c.mutex.Lock()
	source := c.source
	c.mutex.Unlock()

	if source == nil {
		return 0
	}

	return source.Position()
}

func (c *Connection) Close() {
	if c.closed.Swap(true) {
		return
	}
	c.Stop()

	// Close the disgo voice connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.voiceConn.Close(ctx)

	c.logger.Debug("voice connection closed",
		slog.String("guild_id", c.guildID.String()),
	)
}
