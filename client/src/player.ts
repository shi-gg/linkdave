import { GatewayOpcodes, type GatewayVoiceServerUpdateDispatchData, type GatewayVoiceStateUpdateDispatchData } from "discord-api-types/v10";

import type { LinkDaveClient } from "./client.js";
import type { Node } from "./node.js";
import type {
    MigrateReadyPayload,
    PlayerState,
    PlayerUpdatePayload,
    PlayPayload,
    TrackInfo,
    VoiceServerEvent
} from "./types.js";

export interface PlayOptions {
    startTime?: number;
    volume?: number;
}

export interface PlayerOptions {
    voiceChannelId?: string;
    selfMute?: boolean;
    selfDeaf?: boolean;
}

export type RawVoiceStateUpdate = Pick<GatewayVoiceStateUpdateDispatchData, "channel_id" | "session_id">;
export type RawVoiceServerUpdate = Pick<GatewayVoiceServerUpdateDispatchData, "token" | "guild_id" | "endpoint">;

interface VoiceState {
    channelId: string;
    sessionId: string;
    event: VoiceServerEvent;
}

interface PendingVoiceState {
    channelId?: string;
    sessionId?: string;
    serverEvent?: VoiceServerEvent;
}

export class Player {
    readonly #client: LinkDaveClient;
    readonly #guildId: string;
    #node: Node;

    #voiceChannelId: string | null = null;
    #selfMute: boolean;
    #selfDeaf: boolean;
    #state: PlayerState = "idle";
    #position = 0;
    #volume = 100;
    #currentTrack: TrackInfo | null = null;
    #voiceState: VoiceState | null = null;
    #pendingVoice: PendingVoiceState | null = null;
    #migrationTarget: Node | null = null;
    #migrationResolve: ((value: unknown) => void) | null = null;

    constructor(client: LinkDaveClient, guildId: string, node: Node, options?: PlayerOptions) {
        this.#client = client;
        this.#guildId = guildId;
        this.#node = node;
        this.#voiceChannelId = options?.voiceChannelId ?? null;
        this.#selfMute = options?.selfMute ?? false;
        this.#selfDeaf = options?.selfDeaf ?? true;
    }

    get guildId() {
        return this.#guildId;
    }

    get voiceChannelId() {
        return this.#voiceChannelId;
    }

    set voiceChannelId(id: string | null) {
        this.#voiceChannelId = id;
    }

    get state() {
        return this.#state;
    }

    get position() {
        return this.#position;
    }

    get volume() {
        return this.#volume;
    }

    get currentTrack() {
        return this.#currentTrack;
    }

    get node() {
        return this.#node;
    }

    get playing() {
        return this.#state === "playing";
    }

    get paused() {
        return this.#state === "paused";
    }

    connect(channelId?: string) {
        const targetChannel = channelId ?? this.#voiceChannelId;
        if (!targetChannel) {
            throw new RangeError("No voice channel ID provided");
        }

        this.#voiceChannelId = targetChannel;

        this.#client._sendToShard(this.#guildId, {
            op: GatewayOpcodes.VoiceStateUpdate,
            d: {
                guild_id: this.#guildId,
                channel_id: targetChannel,
                self_mute: this.#selfMute,
                self_deaf: this.#selfDeaf
            }
        });
    }

    disconnect() {
        this.#client._sendToShard(this.#guildId, {
            op: GatewayOpcodes.VoiceStateUpdate,
            d: {
                guild_id: this.#guildId,
                channel_id: null,
                self_mute: false,
                self_deaf: false
            }
        });

        this.#voiceChannelId = null;
        this.#state = "idle";
        this.#currentTrack = null;
        this.#position = 0;
        this.#voiceState = null;
        this.#pendingVoice = null;
    }

    handleVoiceStateUpdate(data: RawVoiceStateUpdate) {
        if (!data.channel_id) {
            // Left voice channel
            this.#pendingVoice = null;
            return;
        }

        this.#pendingVoice ??= {};
        this.#pendingVoice.channelId = data.channel_id;
        this.#pendingVoice.sessionId = data.session_id;

        this.#tryConnectLinkDave();
    }

    // A null endpoint means that the voice server allocated has gone away and is trying to be reallocated.
    // You should attempt to disconnect from the currently connected voice server,
    // and not attempt to reconnect until a new voice server is allocated.
    handleVoiceServerUpdate(data: RawVoiceServerUpdate) {
        this.#pendingVoice ??= {};

        const endpoint = data.endpoint || this.#pendingVoice.serverEvent?.endpoint;
        if (!endpoint) throw new Error("Missing voice server endpoint"); // TODO

        this.#pendingVoice.serverEvent = {
            token: data.token,
            guild_id: data.guild_id,
            endpoint
        };

        this.#tryConnectLinkDave();
    }

    #tryConnectLinkDave() {
        const pending = this.#pendingVoice;
        if (!pending?.channelId || !pending.sessionId || !pending.serverEvent) {
            return;
        }

        // We have all the data, connect to LinkDave
        this.#connectToLinkDave(pending.channelId, pending.sessionId, pending.serverEvent);
        this.#pendingVoice = null;
    }

    #connectToLinkDave(channelId: string, sessionId: string, event: VoiceServerEvent) {
        this.#voiceChannelId = channelId;
        this.#voiceState = { channelId, sessionId, event };

        this.#node.sendVoiceUpdate({
            guild_id: this.#guildId,
            channel_id: channelId,
            session_id: sessionId,
            event
        });
    }

    play(url: string, options: PlayOptions = {}) {
        const payload: PlayPayload = {
            guild_id: this.#guildId,
            url
        };

        if (options.startTime !== undefined) {
            payload.start_time = options.startTime;
        }

        if (options.volume !== undefined) {
            payload.volume = options.volume;
        }

        this.#node.sendPlay(payload);
    }

    pause() {
        this.#node.sendPause(this.#guildId);
    }

    resume() {
        this.#node.sendResume(this.#guildId);
    }

    stop() {
        this.#node.sendStop(this.#guildId);
        this.#currentTrack = null;
        this.#state = "idle";
        this.#position = 0;
    }

    seek(position: number) {
        this.#node.sendSeek({
            guild_id: this.#guildId,
            position
        });
    }

    setVolume(volume: number) {
        this.#volume = Math.max(0, Math.min(1_000, volume));
        this.#node.sendVolume({
            guild_id: this.#guildId,
            volume: this.#volume
        });
    }

    destroy() {
        this.disconnect();
        this.#node.sendDisconnect(this.#guildId);
        this.#client.removePlayer(this.#guildId);
    }

    async moveNode(targetNode: Node) {
        if (targetNode === this.#node) return;

        if (!targetNode.connected || targetNode.draining) {
            throw new Error(`Target node "${targetNode.name}" is not available`);
        }

        this.#migrationTarget = targetNode;
        this.#node.sendPlayerMigrate(this.#guildId);

        return new Promise((resolve) => {
            this.#migrationResolve = resolve;
        });
    }

    _updateState(data: PlayerUpdatePayload) {
        this.#state = data.state;
        this.#position = data.position;
        this.#volume = data.volume;
    }

    _setTrack(track: TrackInfo | null) {
        this.#currentTrack = track;
    }

    _onMigrateReady(data: MigrateReadyPayload) {
        if (!this.#migrationTarget || data.guild_id !== this.#guildId) {
            return;
        }

        const targetNode = this.#migrationTarget;
        const oldNode = this.#node;

        // Don't send disconnect to old node - we're migrating
        this.#node = targetNode;
        this.#client._updatePlayerNode(this.#guildId, oldNode, targetNode);

        if (this.#voiceState) {
            this.#node.sendVoiceUpdate({
                guild_id: this.#guildId,
                channel_id: this.#voiceState.channelId,
                session_id: this.#voiceState.sessionId,
                event: this.#voiceState.event
            });
        }

        if (data.state === "playing" && data.url) {
            this.#node.sendPlay({
                guild_id: this.#guildId,
                url: data.url,
                start_time: data.position,
                volume: data.volume
            });
        }

        this.#migrationTarget = null;
        if (!this.#migrationResolve) return;

        this.#migrationResolve(null);
        this.#migrationResolve = null;
    }
}