export enum ClientOpCodes {
    Identify = 0,
    VoiceUpdate = 1,
    Play = 2,
    Pause = 3,
    Resume = 4,
    Stop = 5,
    Seek = 6,
    Disconnect = 7,
    Ping = 8,
    Volume = 9,
    PlayerMigrate = 10
}

export enum ServerOpCodes {
    Ready = 0,
    PlayerUpdate = 1,
    TrackStart = 2,
    TrackEnd = 3,
    TrackError = 4,
    VoiceConnected = 5,
    VoiceDisconnected = 6,
    Pong = 7,
    Stats = 8,
    NodeDraining = 9,
    MigrateReady = 10
}

export type TrackEndReason = "finished" | "stopped" | "replaced" | "error" | "cleanup";

export type PlayerState = "idle" | "playing" | "paused";

export interface Message<T = unknown> {
    op: number;
    d?: T;
}

export interface IdentifyPayload {
    bot_id: string;
}

export interface VoiceServerEvent {
    token: string;
    guild_id: string;
    endpoint: string;
}

export interface VoiceUpdatePayload {
    guild_id: string;
    channel_id: string;
    session_id: string;
    event: VoiceServerEvent;
}

export interface PlayPayload {
    guild_id: string;
    url: string;
    start_time?: number;
    volume?: number;
}

export interface GuildPayload {
    guild_id: string;
}

export interface SeekPayload {
    guild_id: string;
    position: number;
}

export interface VolumePayload {
    guild_id: string;
    volume: number;
}

export interface ReadyPayload {
    session_id: string;
    resumed: boolean;
}

export interface PlayerUpdatePayload {
    guild_id: string;
    state: PlayerState;
    position: number;
    volume: number;
}

export interface TrackInfo {
    url: string;
    title?: string;
    duration?: number;
}

export interface TrackStartPayload {
    guild_id: string;
    track: TrackInfo;
}

export interface TrackEndPayload {
    guild_id: string;
    track: TrackInfo;
    reason: TrackEndReason;
}

export interface TrackErrorPayload {
    guild_id: string;
    track: TrackInfo;
    error: string;
}

export interface VoiceConnectedPayload {
    guild_id: string;
    channel_id: string;
}

export interface VoiceDisconnectedPayload {
    guild_id: string;
    reason?: string;
}

export interface StatsPayload {
    players: number;
    playing_tracks: number;
    uptime: number;
    memory_used: number;
    memory_alloc: number;
    cpu_usage: number;
    draining: boolean;
}

export interface PlayerMigratePayload {
    guild_id: string;
}

export interface NodeDrainingPayload {
    reason: string;
    deadline_ms: number;
}

export interface MigrateReadyPayload {
    guild_id: string;
    url: string;
    position: number;
    volume: number;
    state: PlayerState;
}

export interface LinkDaveEvents {
    ready: ReadyPayload;
    playerUpdate: PlayerUpdatePayload;
    trackStart: TrackStartPayload;
    trackEnd: TrackEndPayload;
    trackError: TrackErrorPayload;
    voiceConnected: VoiceConnectedPayload;
    voiceDisconnected: VoiceDisconnectedPayload;
    pong: undefined;
    stats: StatsPayload;
    nodeDraining: NodeDrainingPayload;
    migrateReady: MigrateReadyPayload;
    close: { code: number; reason: string; };
    error: Error;
}

export type LinkDaveEventName = keyof LinkDaveEvents;