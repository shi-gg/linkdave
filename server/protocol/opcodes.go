package protocol

const (
	OpIdentify      uint8 = 0
	OpVoiceUpdate   uint8 = 1
	OpPlay          uint8 = 2
	OpPause         uint8 = 3
	OpResume        uint8 = 4
	OpStop          uint8 = 5
	OpSeek          uint8 = 6
	OpDisconnect    uint8 = 7
	OpPing          uint8 = 8
	OpVolume        uint8 = 9
	OpPlayerMigrate uint8 = 10
)

const (
	OpReady           uint8 = 0
	OpPlayerUpdate    uint8 = 1
	OpTrackStart      uint8 = 2
	OpTrackEnd        uint8 = 3
	OpTrackError      uint8 = 4
	OpVoiceConnect    uint8 = 5
	OpVoiceDisconnect uint8 = 6
	OpPong            uint8 = 7
	OpStats           uint8 = 8
	OpNodeDraining    uint8 = 9
	OpMigrateReady    uint8 = 10
)

const (
	TrackEndReasonFinished = "finished"
	TrackEndReasonStopped  = "stopped"
	TrackEndReasonReplaced = "replaced"
	TrackEndReasonError    = "error"
	TrackEndReasonCleanup  = "cleanup"
)

const (
	PlayerStateIdle    = "idle"
	PlayerStatePlaying = "playing"
	PlayerStatePaused  = "paused"
)
