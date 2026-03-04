package protocol

import (
	"github.com/disgoorg/snowflake/v2"
)

type Message struct {
	Op   uint8 `json:"op"`
	Data any   `json:"d,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type VoiceServerEvent struct {
	Token    string `json:"token"`
	GuildID  string `json:"guild_id"`
	Endpoint string `json:"endpoint"`
}

type VoiceUpdateData struct {
	ClientID  snowflake.ID     `json:"client_id"`
	GuildID   snowflake.ID     `json:"guild_id"`
	ChannelID snowflake.ID     `json:"channel_id"`
	SessionID string           `json:"session_id"`
	Event     VoiceServerEvent `json:"event"`
}

type PlayData struct {
	GuildID   snowflake.ID `json:"guild_id"`
	URL       string       `json:"url"`
	StartTime int64        `json:"start_time,omitempty"`
	Volume    int          `json:"volume,omitempty"`
}

type GuildData struct {
	GuildID snowflake.ID `json:"guild_id"`
}

type SeekData struct {
	GuildID  snowflake.ID `json:"guild_id"`
	Position int64        `json:"position"`
}

type VolumeData struct {
	GuildID snowflake.ID `json:"guild_id"`
	Volume  int          `json:"volume"`
}

type ReadyData struct {
	SessionID string `json:"session_id"`
	Resumed   bool   `json:"resumed"`
}

type PlayerUpdateData struct {
	GuildID  snowflake.ID `json:"guild_id"`
	State    string       `json:"state"`
	Position int64        `json:"position"`
	Volume   int          `json:"volume"`
}

type TrackInfo struct {
	URL      string `json:"url"`
	Title    string `json:"title,omitempty"`
	Duration int64  `json:"duration,omitempty"`
}

type TrackStartData struct {
	GuildID snowflake.ID `json:"guild_id"`
	Track   TrackInfo    `json:"track"`
}

type TrackEndData struct {
	GuildID snowflake.ID `json:"guild_id"`
	Track   TrackInfo    `json:"track"`
	Reason  string       `json:"reason"`
}

type TrackErrorData struct {
	GuildID snowflake.ID `json:"guild_id"`
	Track   TrackInfo    `json:"track"`
	Error   string       `json:"error"`
}

type VoiceConnectData struct {
	GuildID   snowflake.ID `json:"guild_id"`
	ChannelID snowflake.ID `json:"channel_id"`
}

type VoiceDisconnectData struct {
	GuildID snowflake.ID `json:"guild_id"`
	Reason  string       `json:"reason,omitempty"`
}

type StatsData struct {
	Players       int    `json:"players"`
	PlayingTracks int    `json:"playing_tracks"`
	Uptime        int64  `json:"uptime"`
	Memory        uint64 `json:"memory"`
	Draining      bool   `json:"draining"`
}

type NodeDrainingData struct {
	Reason     string `json:"reason"`
	DeadlineMs int64  `json:"deadline_ms"`
}

type PlayerMigrateData struct {
	GuildID snowflake.ID `json:"guild_id"`
}

type MigrateReadyData struct {
	GuildID  snowflake.ID `json:"guild_id"`
	URL      string       `json:"url"`
	Position int64        `json:"position"`
	Volume   int          `json:"volume"`
	State    string       `json:"state"`
}

type StatsResponse struct {
	Version      string `json:"version"`
	Runtime      string `json:"runtime"`
	Uptime       int64  `json:"uptime_ms"`
	NumGoroutine int    `json:"num_goroutines"`
	Memory       uint64 `json:"memory"`
}

type RequestPlay struct {
	URL       string `json:"url"`
	StartTime int64  `json:"start_time,omitempty"`
	Volume    int    `json:"volume,omitempty"`
}

type RequestSeek struct {
	Position int64 `json:"position"`
}

type RequestVolume struct {
	Volume int `json:"volume"`
}
