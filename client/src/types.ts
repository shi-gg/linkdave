import type { Node } from "./node.js";

export enum ClientOpCodes {
    VoiceUpdate = 0,
    PlayerMigrate = 1
}

export enum ServerOpCodes {
    Ready = 0,
    VoiceConnect = 1,
    VoiceDisconnect = 2,
    PlayerUpdate = 3,
    TrackStart = 4,
    TrackEnd = 5,
    TrackError = 6,
    Stats = 7,
    NodeDraining = 8,
    MigrateReady = 9
}

export enum TrackEndReason {
    Finished = "finished",
    Stopped = "stopped",
    Replaced = "replaced",
    Error = "error"
}

export enum PlayerState {
    Idle = "idle",
    Playing = "playing",
    Paused = "paused",
    Connecting = "connecting"
}

export enum Filter {
    /** Slows and lowers pitch (speed ×0.8, pitch ×0.8). */
    Vaporwave = 0,
    /** Speeds up and raises pitch (speed ×1.3, pitch ×1.3). */
    Nightcore = 1,
    /** Rotates audio around the stereo field at 0.2 Hz. */
    Rotation = 2,
    /** Oscillates volume at 4 Hz with 0.6 depth. */
    Tremolo = 3,
    /** Oscillates pitch at 4 Hz with 0.5 depth. */
    Vibrato = 4,
    /** Suppresses high frequencies (smoothing factor 20). */
    LowPass = 5
}

export interface FiltersPayload {
    enabled?: Filter[];
    pitch?: number;
    speed?: number;
}

export type ServerMessage =
    | { op: ServerOpCodes.Ready; d: ReadyPayload; }
    | { op: ServerOpCodes.PlayerUpdate; d: PlayerUpdatePayload; }
    | { op: ServerOpCodes.TrackStart; d: TrackStartPayload; }
    | { op: ServerOpCodes.TrackEnd; d: TrackEndPayload; }
    | { op: ServerOpCodes.TrackError; d: TrackErrorPayload; }
    | { op: ServerOpCodes.VoiceConnect; d: VoiceConnectPayload; }
    | { op: ServerOpCodes.VoiceDisconnect; d: VoiceDisconnectPayload; }
    | { op: ServerOpCodes.Stats; d: StatsPayload; }
    | { op: ServerOpCodes.NodeDraining; d: NodeDrainingPayload; }
    | { op: ServerOpCodes.MigrateReady; d: MigrateReadyPayload; };

export type ClientMessage =
    | { op: ClientOpCodes.VoiceUpdate; d: VoiceUpdatePayload; }
    | { op: ClientOpCodes.PlayerMigrate; d: PlayerMigratePayload; };

export interface VoiceServerEvent {
    token: string;
    guild_id: string;
    endpoint: string;
}

export interface VoiceUpdatePayload {
    client_id: string;
    guild_id: string;
    channel_id: string;
    session_id: string;
    event: VoiceServerEvent;
}

export interface PlayPayload {
    url: string;
    start_time?: number;
    requester_id?: string;
    filters?: FiltersPayload;
}

export interface GuildPayload {
    guild_id: string;
}

export interface SeekPayload {
    position: number;
}

export interface ReadyPayload {
    session_id: string;
    resumed: boolean;
}

export interface PlayerUpdatePayload {
    guild_id: string;
    state: PlayerState;
}

export interface TrackInfo {
    url: string;
    title?: string;
    duration: number;
    requester_id?: string;
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

export interface QueueErrorPayload {
    guild_id: string;
    url: string;
    error: Error;
}

export enum DisconnectReason {
    ConnectionLost = "connection_lost",
    ConnectionFailed = "connection_failed",
    Requested = "requested",
    Inactivity = "inactivity"
}

export interface VoiceConnectPayload {
    guild_id: string;
    channel_id: string;
}

export interface VoiceDisconnectPayload {
    guild_id: string;
    reason?: DisconnectReason;
}

export interface StatsPayload {
    players: number;
    playing_tracks: number;
    uptime: number;
    memory: number;
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
    state: PlayerState;
    requester_id?: string;
    filters?: FiltersPayload;
}

export interface ClosePayload {
    code: number;
    reason: string;
}

export enum EventName {
    Ready = "ready",
    PlayerUpdate = "playerUpdate",
    TrackStart = "trackStart",
    TrackEnd = "trackEnd",
    TrackError = "trackError",
    QueueError = "queueError",
    VoiceConnect = "voiceConnect",
    VoiceDisconnect = "voiceDisconnect",

    Stats = "stats",

    NodeDraining = "nodeDraining",
    MigrateReady = "migrateReady",

    Close = "close",
    Error = "error"
}

export interface Events {
    [EventName.Ready]: ReadyPayload;
    [EventName.PlayerUpdate]: PlayerUpdatePayload;
    [EventName.TrackStart]: TrackStartPayload;
    [EventName.TrackEnd]: TrackEndPayload;
    [EventName.TrackError]: TrackErrorPayload;
    [EventName.QueueError]: QueueErrorPayload;
    [EventName.VoiceConnect]: VoiceConnectPayload;
    [EventName.VoiceDisconnect]: VoiceDisconnectPayload;
    [EventName.Stats]: StatsPayload;
    [EventName.NodeDraining]: NodeDrainingPayload;
    [EventName.MigrateReady]: MigrateReadyPayload;
    [EventName.Close]: ClosePayload;
    [EventName.Error]: Error;
}

export enum ManagerEventName {
    NodeAdd = "nodeAdd",
    NodeRemove = "nodeRemove",
    NodeReconnectAttempt = "nodeReconnectAttempt"
}

export interface ManagerEvents extends Events {
    [ManagerEventName.NodeAdd]: { node: Node; };
    [ManagerEventName.NodeRemove]: { node: Node; };
    [ManagerEventName.NodeReconnectAttempt]: { node: Node; attempt: number; };
}

export interface RESTError {
    error: string;
}

export type RESTResponse<T = undefined> = T extends undefined ? undefined : T;

export const Routes = {
    play: (sessionId: string, guildId: string) => `/sessions/${sessionId}/players/${guildId}/play` as const,
    pause: (sessionId: string, guildId: string) => `/sessions/${sessionId}/players/${guildId}/pause` as const,
    resume: (sessionId: string, guildId: string) => `/sessions/${sessionId}/players/${guildId}/resume` as const,
    stop: (sessionId: string, guildId: string) => `/sessions/${sessionId}/players/${guildId}/stop` as const,
    seek: (sessionId: string, guildId: string) => `/sessions/${sessionId}/players/${guildId}/seek` as const,
    disconnect: (sessionId: string, guildId: string) => `/sessions/${sessionId}/players/${guildId}` as const
} as const;