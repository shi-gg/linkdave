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
	"github.com/shi-gg/linkdave/server/audio/source"
	"github.com/shi-gg/linkdave/server/protocol"
	"github.com/thomas-vilte/dave-go/session"
)

type Connection struct {
	logger    *slog.Logger
	guildID   snowflake.ID
	channelID snowflake.ID
	userID    snowflake.ID

	voiceConn       voice.Conn
	targetVoiceConn voice.Conn

	source       source.Source
	onTrackEnd   func(src source.Source, reason string, err error)
	onDisconnect func()
	paused       atomic.Bool
	closed       atomic.Bool
	mutex        sync.Mutex
	setupMu      sync.Mutex

	setupCancel context.CancelFunc

	stopChan chan struct{}

	staleTimer *time.Timer
}

func NewConnection(
	ctx context.Context,
	logger *slog.Logger,
	userID, guildID, channelID snowflake.ID,
	sessionID string,
	voiceServerEvent protocol.VoiceServerEvent,
	onTrackEnd func(src source.Source, reason string, err error),
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
	c.cancelStaleTimer()

	c.mutex.Lock()
	if c.setupCancel != nil {
		c.setupCancel()
	}
	c.mutex.Unlock()

	c.setupMu.Lock()
	defer c.setupMu.Unlock()

	if c.closed.Load() {
		return fmt.Errorf("connection closed")
	}

	c.mutex.Lock()
	oldVC := c.voiceConn
	c.mutex.Unlock()

	var disconnected atomic.Bool

	var vc voice.Conn
	vc = voice.NewConn(
		c.guildID,
		c.userID,
		func(ctx context.Context, guildID snowflake.ID, channelID *snowflake.ID, selfMute, selfDeaf bool) error {
			if channelID != nil || vc == nil || disconnected.Swap(true) {
				return nil
			}

			c.mutex.Lock()
			isCurrent := c.voiceConn == vc
			isTarget := c.targetVoiceConn == vc
			c.mutex.Unlock()

			if isCurrent || isTarget {
				vc.HandleVoiceStateUpdate(gateway.EventVoiceStateUpdate{
					VoiceState: discord.VoiceState{
						GuildID:   guildID,
						ChannelID: nil,
						UserID:    c.userID,
					},
				})
			}

			return nil
		},
		func() {
			c.logger.Debug("voice connection removed from manager", slog.String("guild_id", c.guildID.String()))

			c.mutex.Lock()
			wasCurrent := c.voiceConn == vc
			replacing := wasCurrent && c.targetVoiceConn != nil
			if wasCurrent {
				c.voiceConn = nil
			}
			if c.targetVoiceConn == vc {
				c.targetVoiceConn = nil
			}
			c.mutex.Unlock()

			if !wasCurrent || replacing || c.closed.Load() {
				return
			}

			c.scheduleUnexpectedDisconnect()
		},
		voice.WithConnLogger(c.logger),
		voice.WithConnDaveSessionCreateFunc(session.New),
	)

	openCtx, openCancel := context.WithCancel(ctx)

	c.mutex.Lock()
	c.setupCancel = openCancel
	c.targetVoiceConn = vc
	c.mutex.Unlock()

	if oldVC != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		oldVC.Close(closeCtx)
		cancel()
	}

	vc.HandleVoiceStateUpdate(gateway.EventVoiceStateUpdate{
		VoiceState: discord.VoiceState{
			GuildID:   c.guildID,
			ChannelID: &channelID,
			UserID:    c.userID,
			SessionID: sessionID,
		},
	})
	vc.HandleVoiceServerUpdate(gateway.EventVoiceServerUpdate{
		Token:    event.Token,
		GuildID:  c.guildID,
		Endpoint: &event.Endpoint,
	})

	if err := vc.Open(openCtx, channelID, false, false); err != nil {
		openCancel()

		c.mutex.Lock()
		if c.targetVoiceConn == vc {
			c.targetVoiceConn = nil
		}
		c.mutex.Unlock()

		return fmt.Errorf("failed to open voice connection: %w", err)
	}

	if c.closed.Load() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		vc.Close(closeCtx)
		cancel()
		return fmt.Errorf("connection closed during setup")
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.targetVoiceConn != vc {
		return fmt.Errorf("voice connection closed during setup")
	}

	c.voiceConn = vc
	c.targetVoiceConn = nil
	c.channelID = channelID
	vc.SetOpusFrameProvider(&trackWrapper{conn: c})

	return nil
}

func (c *Connection) cancelStaleTimer() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.staleTimer == nil {
		return
	}

	c.staleTimer.Stop()
	c.staleTimer = nil
}

func (c *Connection) scheduleUnexpectedDisconnect() {
	if c.closed.Load() {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.staleTimer != nil {
		c.staleTimer.Stop()
	}

	c.staleTimer = time.AfterFunc(time.Second, c.handleUnexpectedDisconnect)
}

func (c *Connection) handleUnexpectedDisconnect() {
	c.mutex.Lock()

	if c.staleTimer == nil {
		c.mutex.Unlock()
		return
	}

	c.staleTimer = nil
	stale := c.voiceConn == nil && c.targetVoiceConn == nil

	if !stale || c.closed.Swap(true) {
		c.mutex.Unlock()
		return
	}

	c.mutex.Unlock()
	c.logger.Info("sending unexpected disconnect", slog.String("guild_id", c.guildID.String()))

	c.Stop()
	c.onDisconnect()
}

func (c *Connection) HandleVoiceUpdate(ctx context.Context, channelID snowflake.ID, sessionID string, event protocol.VoiceServerEvent) error {
	c.logger.Info("handling voice update (channel move/server change)",
		slog.String("guild_id", c.guildID.String()),
		slog.String("new_channel_id", channelID.String()),
	)
	return c.setupVoiceConn(ctx, channelID, sessionID, event)
}

func (c *Connection) Play(ctx context.Context, src source.Source) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.source != nil {
		oldSource := c.source
		c.source = nil
		oldSource.Close()

		if c.onTrackEnd != nil {
			c.onTrackEnd(oldSource, protocol.TrackEndReasonReplaced, nil)
		}
	}

	c.source = src
	c.paused.Store(false)

	select {
	case <-c.stopChan:
		c.stopChan = make(chan struct{})
	default:
	}

	c.logger.Debug("started playback",
		slog.String("guild_id", c.guildID.String()),
	)

	return nil
}

func (c *Connection) Pause() {
	c.paused.Store(true)
}

func (c *Connection) Resume() {
	c.paused.Store(false)
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
			c.onTrackEnd(oldSource, protocol.TrackEndReasonStopped, nil)
		}
	}
}

func (c *Connection) handleTrackEnd(src source.Source, err error) {
	c.mutex.Lock()
	if c.source != src {
		c.mutex.Unlock()
		return
	}
	c.source = nil
	c.mutex.Unlock()

	reason := protocol.TrackEndReasonFinished
	if err != nil && err != source.ErrEOF {
		reason = protocol.TrackEndReasonError
	}

	src.Close()

	if c.onTrackEnd != nil {
		c.onTrackEnd(src, reason, err)
	}
}

type trackWrapper struct {
	conn *Connection
}

func (w *trackWrapper) ProvideOpusFrame() ([]byte, error) {
	w.conn.mutex.Lock()
	src := w.conn.source
	w.conn.mutex.Unlock()

	if src == nil {
		return nil, nil
	}
	if w.conn.paused.Load() {
		return nil, nil
	}

	return w.conn.provideOpusFrame(src)
}

func (w *trackWrapper) Close() {
	w.conn.mutex.Lock()
	defer w.conn.mutex.Unlock()

	if w.conn.source != nil {
		w.conn.source.Close()
	}
}

func (c *Connection) provideOpusFrame(src source.Source) ([]byte, error) {
	frame, err := src.ProvideOpusFrame()
	if err != nil {
		c.handleTrackEnd(src, err)
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

	c.cancelStaleTimer()
	c.Stop()

	c.mutex.Lock()
	vc := c.voiceConn
	c.mutex.Unlock()

	if vc == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	vc.Close(ctx)
	c.logger.Debug("voice connection closed", slog.String("guild_id", c.guildID.String()))
}
