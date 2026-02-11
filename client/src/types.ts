import type { Node } from "./node.js";

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
    | { op: ServerOpCodes.VoiceConnected; d: VoiceConnectPayload; }
    | { op: ServerOpCodes.VoiceDisconnected; d: VoiceDisconnectPayload; }
    | { op: ServerOpCodes.Pong; d?: undefined; }
    | { op: ServerOpCodes.Stats; d: StatsPayload; }
    | { op: ServerOpCodes.NodeDraining; d: NodeDrainingPayload; }
    | { op: ServerOpCodes.MigrateReady; d: MigrateReadyPayload; };

export type ClientMessage =
    | { op: ClientOpCodes.Identify; d: IdentifyPayload; }
    | { op: ClientOpCodes.VoiceUpdate; d: VoiceUpdatePayload; }
    | { op: ClientOpCodes.Play; d: PlayPayload; }
    | { op: ClientOpCodes.Pause; d: GuildPayload; }
    | { op: ClientOpCodes.Resume; d: GuildPayload; }
    | { op: ClientOpCodes.Stop; d: GuildPayload; }
    | { op: ClientOpCodes.Seek; d: SeekPayload; }
    | { op: ClientOpCodes.Disconnect; d: GuildPayload; }
    | { op: ClientOpCodes.Ping; d?: undefined; }
    | { op: ClientOpCodes.Volume; d: VolumePayload; }
    | { op: ClientOpCodes.PlayerMigrate; d: PlayerMigratePayload; };

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
    [EventName.Close]: { code: number; reason: string; };
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