package protocol

const (
	OpPing          uint8 = 0
	OpVoiceUpdate   uint8 = 1
	OpPlayerMigrate uint8 = 2
)

const (
	OpPong            uint8 = 0
	OpReady           uint8 = 1
	OpVoiceConnect    uint8 = 2
	OpVoiceDisconnect uint8 = 3
	OpPlayerUpdate    uint8 = 4
	OpTrackStart      uint8 = 5
	OpTrackEnd        uint8 = 6
	OpTrackError      uint8 = 7
	OpStats           uint8 = 8
	OpNodeDraining    uint8 = 9
	OpMigrateReady    uint8 = 10
)

const (
	TrackEndReasonFinished = "finished"
	TrackEndReasonStopped  = "stopped"
	TrackEndReasonReplaced = "replaced"
	TrackEndReasonError    = "error"
)

const (
	PlayerStateIdle    = "idle"
	PlayerStatePlaying = "playing"
	PlayerStatePaused  = "paused"
)
