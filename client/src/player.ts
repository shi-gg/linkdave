import { GatewayOpcodes, type GatewayVoiceServerUpdateDispatchData, type GatewayVoiceStateUpdateDispatchData } from "discord-api-types/v10";

import type { LinkDaveClient } from "./client.js";
import type { Node } from "./node.js";
import type {
    MigrateReadyPayload,
    PlayerUpdatePayload,
    TrackInfo,
    VoiceConnectPayload,
    VoiceServerEvent
} from "./types.js";
import { EventName, PlayerState } from "./types.js";
import { unwrap } from "./utils.js";

export interface PlayOptions {
    startTime?: number;
    volume?: number;
}

export interface PlayerOptions {
    voiceChannelId?: string;
    selfMute?: boolean;
    selfDeaf?: boolean;
}

export type RawVoiceStateUpdate = Pick<GatewayVoiceStateUpdateDispatchData, "user_id" | "channel_id" | "session_id">;
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
    public static readonly CONNECT_TIMEOUT = 10_000;

    readonly #client: LinkDaveClient;
    readonly #guildId: string;
    #node: Node;

    #voiceChannelId: string | null = null;
    #selfMute: boolean;
    #selfDeaf: boolean;
    #state: PlayerState = PlayerState.Idle;
    #position = 0;
    #volume = 100;
    #currentTrack: TrackInfo | null = null;
    #voiceState: VoiceState | null = null;
    #pendingVoice: PendingVoiceState | null = null;
    #lastServerEvent: VoiceServerEvent | null = null;
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
        return this.#state === PlayerState.Playing;
    }

    get paused() {
        return this.#state === PlayerState.Paused;
    }

    get connected() {
        return this.#voiceState !== null;
    }

    connect(channelId?: string, timeoutMs = Player.CONNECT_TIMEOUT) {
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

        return new Promise<VoiceConnectPayload>((resolve, reject) => {
            const timer = setTimeout(
                () => {
                    this.#node.off(EventName.VoiceConnect, onConnect);
                    reject(new Error(`Voice connection timed out for guild "${this.#guildId}"`));
                },
                timeoutMs
            );

            const onConnect = (event: VoiceConnectPayload) => {
                if (event.guild_id !== this.#guildId) return;
                this.#node.off(EventName.VoiceConnect, onConnect);
                clearTimeout(timer);
                resolve(event);
            };

            this.#node.on(EventName.VoiceConnect, onConnect);
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
        this.#state = PlayerState.Idle;
        this.#currentTrack = null;
        this.#position = 0;
        this.#voiceState = null;
        this.#pendingVoice = null;
        this.#lastServerEvent = null;
    }

    async handleVoiceStateUpdate(data: RawVoiceStateUpdate) {
        if (!data.channel_id) {
            this.#voiceChannelId = null;
            this.#voiceState = null;
            this.#pendingVoice = null;
            this.#lastServerEvent = null;

            if (this.#node.connected) {
                await unwrap(this.#node.sendDisconnect(this.#guildId));
            }

            return;
        }

        this.#pendingVoice ??= {};
        this.#pendingVoice.channelId = data.channel_id;
        this.#pendingVoice.sessionId = data.session_id;

        // If we already have a server event from a previous connection to the same
        // channel and no new VOICE_SERVER_UPDATE has been received yet, re-use it.
        if (!this.#pendingVoice.serverEvent && this.#lastServerEvent) {
            this.#pendingVoice.serverEvent = this.#lastServerEvent;
        }

        this.#tryConnectLinkDave();
    }

    // A null endpoint means that the voice server allocated has gone away and is trying to be reallocated.
    // You should attempt to disconnect from the currently connected voice server,
    // and not attempt to reconnect until a new voice server is allocated.
    async handleVoiceServerUpdate(data: RawVoiceServerUpdate) {
        if (!data.endpoint) {
            this.#voiceState = null;

            if (this.#pendingVoice) {
                delete this.#pendingVoice.serverEvent;
            }

            if (this.#node.connected) {
                await unwrap(this.#node.sendDisconnect(this.#guildId));
            }

            return;
        }

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
        this.#lastServerEvent = event;

        this.#node.sendVoiceUpdate({
            client_id: this.#client.clientId,
            guild_id: this.#guildId,
            channel_id: channelId,
            session_id: sessionId,
            event
        });
    }

    async play(url: string, options: PlayOptions = {}) {
        await this.#node.sendPlay(this.#guildId, {
            url,
            ...(options.startTime !== undefined && { start_time: options.startTime }),
            ...(options.volume !== undefined && { volume: options.volume })
        });
    }

    async pause() {
        await this.#node.sendPause(this.#guildId);
    }

    async resume() {
        await this.#node.sendResume(this.#guildId);
    }

    async stop() {
        await this.#node.sendStop(this.#guildId);
        this.#currentTrack = null;
        this.#state = PlayerState.Idle;
        this.#position = 0;
    }

    async seek(position: number) {
        await this.#node.sendSeek(this.#guildId, { position });
    }

    async setVolume(volume: number) {
        this.#volume = Math.max(0, Math.min(1_000, volume));
        await this.#node.sendVolume(this.#guildId, { volume: this.#volume });
    }

    async destroy() {
        this.disconnect();

        if (this.#node.connected) {
            await unwrap(this.#node.sendDisconnect(this.#guildId));
        }

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

    _onVoiceDisconnect() {
        this.#voiceState = null;
        this.#pendingVoice = null;
    }

    _onMigrateReady(data: MigrateReadyPayload) {
        if (!this.#migrationTarget || data.guild_id !== this.#guildId) {
            return;
        }

        const targetNode = this.#migrationTarget;
        const oldNode = this.#node;

        // Don't send disconnect to old node - we're migrating
        this.#client._updatePlayerNode(this.#guildId, oldNode, targetNode);
        this.#node = targetNode;

        if (this.#voiceState) {
            this.#node.sendVoiceUpdate({
                client_id: this.#client.clientId,
                guild_id: this.#guildId,
                channel_id: this.#voiceState.channelId,
                session_id: this.#voiceState.sessionId,
                event: this.#voiceState.event
            });
        }

        if (data.state === PlayerState.Playing && data.url) {
            const playData = {
                url: data.url,
                start_time: data.position,
                volume: data.volume
            };

            // Wait for voice connection on the new node before playing
            const onVoiceConnect = (event: VoiceConnectPayload) => {
                if (event.guild_id !== this.#guildId) return;
                this.#node.off(EventName.VoiceConnect, onVoiceConnect);
                void this.#node.sendPlay(this.#guildId, playData);
            };
            this.#node.on(EventName.VoiceConnect, onVoiceConnect);
        }

        this.#migrationTarget = null;
        if (!this.#migrationResolve) return;

        this.#migrationResolve(null);
        this.#migrationResolve = null;
    }
}