import type { Node } from "./node.js";

export enum ClientOpCodes {
    Ping = 0,
    VoiceUpdate = 1,
    PlayerMigrate = 2
}

export enum ServerOpCodes {
    Pong = 0,
    Ready = 1,
    VoiceConnect = 2,
    VoiceDisconnect = 3,
    PlayerUpdate = 4,
    TrackStart = 5,
    TrackEnd = 6,
    TrackError = 7,
    Stats = 8,
    NodeDraining = 9,
    MigrateReady = 10
}

export enum TrackEndReason {
    Finished = "finished",
    Stopped = "stopped",
    Replaced = "replaced",
    Error = "error",
    Cleanup = "cleanup"
}

export enum PlayerState {
    Idle = "idle",
    Playing = "playing",
    Paused = "paused"
}

export type ServerMessage =
    | { op: ServerOpCodes.Ready; d: ReadyPayload; }
    | { op: ServerOpCodes.PlayerUpdate; d: PlayerUpdatePayload; }
    | { op: ServerOpCodes.TrackStart; d: TrackStartPayload; }
    | { op: ServerOpCodes.TrackEnd; d: TrackEndPayload; }
    | { op: ServerOpCodes.TrackError; d: TrackErrorPayload; }
    | { op: ServerOpCodes.VoiceConnect; d: VoiceConnectPayload; }
    | { op: ServerOpCodes.VoiceDisconnect; d: VoiceDisconnectPayload; }
    | { op: ServerOpCodes.Pong; d?: undefined; }
    | { op: ServerOpCodes.Stats; d: StatsPayload; }
    | { op: ServerOpCodes.NodeDraining; d: NodeDrainingPayload; }
    | { op: ServerOpCodes.MigrateReady; d: MigrateReadyPayload; };

export type ClientMessage =
    | { op: ClientOpCodes.VoiceUpdate; d: VoiceUpdatePayload; }
    | { op: ClientOpCodes.Ping; d?: undefined; }
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
    volume?: number;
}

export interface GuildPayload {
    guild_id: string;
}

export interface SeekPayload {
    position: number;
}

export interface VolumePayload {
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
    duration: number;
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

export interface VoiceConnectPayload {
    guild_id: string;
    channel_id: string;
}

export interface VoiceDisconnectPayload {
    guild_id: string;
    reason?: string;
}

export interface StatsPayload {
    players: number;
    playing_tracks: number;
    uptime: number;
    memory: number;
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
    VoiceConnect = "voiceConnect",
    VoiceDisconnect = "voiceDisconnect",

    Pong = "pong",
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
    [EventName.VoiceConnect]: VoiceConnectPayload;
    [EventName.VoiceDisconnect]: VoiceDisconnectPayload;
    [EventName.Pong]: undefined;
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
    volume: (sessionId: string, guildId: string) => `/sessions/${sessionId}/players/${guildId}/volume` as const,
    disconnect: (sessionId: string, guildId: string) => `/sessions/${sessionId}/players/${guildId}` as const
} as const;