import { EventEmitter } from "node:events";

import type {
    GuildPayload,
    IdentifyPayload,
    LinkDaveEvents,
    Message,
    MigrateReadyPayload,
    NodeDrainingPayload,
    PlayerMigratePayload,
    PlayerUpdatePayload,
    PlayPayload,
    ReadyPayload,
    SeekPayload,
    StatsPayload,
    TrackEndPayload,
    TrackErrorPayload,
    TrackStartPayload,
    VoiceConnectedPayload,
    VoiceDisconnectedPayload,
    VoiceUpdatePayload,
    VolumePayload
} from "./types.js";
import {
    ClientOpCodes,
    ServerOpCodes
} from "./types.js";

export interface NodeOptions {
    name: string;
    url: string;
    autoReconnect?: boolean;
    reconnectDelay?: number;
    maxReconnectAttempts?: number;
}

export type NodeState = "disconnected" | "connecting" | "connected" | "draining";

// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging
export interface Node {
    on: <K extends keyof LinkDaveEvents>(event: K, listener: (data: LinkDaveEvents[K]) => void) => this;
    once: <K extends keyof LinkDaveEvents>(event: K, listener: (data: LinkDaveEvents[K]) => void) => this;
    off: <K extends keyof LinkDaveEvents>(event: K, listener: (data: LinkDaveEvents[K]) => void) => this;
    emit: <K extends keyof LinkDaveEvents>(event: K, data: LinkDaveEvents[K]) => boolean;
}

// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging
export class Node extends EventEmitter {
    readonly name: string;
    readonly url: string;
    readonly #options: Required<Omit<NodeOptions, "name" | "url">>;
    #ws: WebSocket | null = null;
    #sessionId: string | null = null;
    #clientId: string | null = null;
    #reconnectAttempts = 0;
    #reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
    #pingInterval: ReturnType<typeof setInterval> | null = null;
    #state: NodeState = "disconnected";
    #playerCount = 0;
    #draining = false;

    constructor(options: NodeOptions) {
        super();
        this.name = options.name;
        this.url = options.url;
        this.#options = {
            autoReconnect: options.autoReconnect ?? true,
            reconnectDelay: options.reconnectDelay ?? 5_000,
            maxReconnectAttempts: options.maxReconnectAttempts ?? 10
        };
    }

    connect(clientId: string) {
        this.#clientId = clientId;
        this.#state = "connecting";

        return new Promise((resolve, reject) => {
            if (this.#ws) {
                this.#ws.close();
            }

            const url = new URL(this.url);
            // Ensure we connect to the /ws endpoint
            if (!url.pathname || url.pathname === "/") {
                url.pathname = "/ws";
            }
            url.searchParams.set("node", this.name);

            this.#ws = new WebSocket(url.toString());

            const onOpen = () => {
                this.#state = "connected";
                this.#reconnectAttempts = 0;
                this.#startPingInterval();
                this.#sendIdentify();
                resolve(null);
            };

            const onError = (event: Event) => {
                const message = "message" in event ? String((event as { message: unknown; }).message) : "unknown";
                const error = new Error(`WebSocket error: ${message}`);
                this.emit("error", error);
                reject(error);
            };

            this.#ws.addEventListener("open", onOpen, { once: true });
            this.#ws.addEventListener("error", onError, { once: true });
            this.#ws.addEventListener("message", this.#onMessage.bind(this));
            this.#ws.addEventListener("close", this.#onClose.bind(this));
        });
    }

    disconnect() {
        this.#stopPingInterval();
        this.#stopReconnect();

        if (this.#ws) {
            this.#ws.close(1_000, "Client disconnect");
            this.#ws = null;
        }

        this.#state = "disconnected";
        this.#sessionId = null;
    }

    get state() {
        return this.#state;
    }

    get sessionId() {
        return this.#sessionId;
    }

    get playerCount() {
        return this.#playerCount;
    }

    get draining() {
        return this.#draining;
    }

    get connected() {
        return this.#state === "connected";
    }

    incrementPlayerCount() {
        this.#playerCount++;
    }

    decrementPlayerCount() {
        if (this.#playerCount > 0) {
            this.#playerCount--;
        }
    }

    #onMessage(event: MessageEvent) {
        try {
            const message = JSON.parse(event.data as string) as Message;
            this.#handleMessage(message);
        } catch {
            // Invalid messages are silently ignored
        }
    }

    #handleMessage(message: Message) {
        switch (message.op) {
            case ServerOpCodes.Ready:
                this.#handleReady(message.d as ReadyPayload);
                break;
            case ServerOpCodes.PlayerUpdate:
                this.emit("playerUpdate", message.d as PlayerUpdatePayload);
                break;
            case ServerOpCodes.TrackStart:
                this.emit("trackStart", message.d as TrackStartPayload);
                break;
            case ServerOpCodes.TrackEnd:
                this.emit("trackEnd", message.d as TrackEndPayload);
                break;
            case ServerOpCodes.TrackError:
                this.emit("trackError", message.d as TrackErrorPayload);
                break;
            case ServerOpCodes.VoiceConnected:
                this.emit("voiceConnected", message.d as VoiceConnectedPayload);
                break;
            case ServerOpCodes.VoiceDisconnected:
                this.emit("voiceDisconnected", message.d as VoiceDisconnectedPayload);
                break;
            case ServerOpCodes.Pong:
                this.emit("pong", undefined);
                break;
            case ServerOpCodes.Stats:
                this.#handleStats(message.d as StatsPayload);
                break;
            case ServerOpCodes.NodeDraining:
                this.#handleNodeDraining(message.d as NodeDrainingPayload);
                break;
            case ServerOpCodes.MigrateReady:
                this.emit("migrateReady", message.d as MigrateReadyPayload);
                break;
        }
    }

    #handleReady(data: ReadyPayload) {
        this.#sessionId = data.session_id;
        this.emit("ready", data);
    }

    #handleStats(data: StatsPayload) {
        this.#playerCount = data.players;
        this.#draining = data.draining;
        this.emit("stats", data);
    }

    #handleNodeDraining(data: NodeDrainingPayload) {
        this.#draining = true;
        this.#state = "draining";
        this.emit("nodeDraining", data);
    }

    #onClose(event: CloseEvent) {
        this.#state = "disconnected";
        this.#draining = false;
        this.#stopPingInterval();

        this.emit("close", { code: event.code, reason: event.reason });

        if (this.#options.autoReconnect && !this.#draining && this.#reconnectAttempts < this.#options.maxReconnectAttempts) {
            this.#scheduleReconnect();
        }
    }

    #scheduleReconnect() {
        if (this.#reconnectTimeout || !this.#clientId) return;

        const delay = this.#options.reconnectDelay * Math.pow(2, this.#reconnectAttempts);
        this.#reconnectAttempts++;

        this.#reconnectTimeout = setTimeout(() => {
            this.#reconnectTimeout = null;
            if (!this.#clientId) return;

            void this.connect(this.#clientId).catch(() => {
                // Ignore connection errors during reconnection
                // onClose will handle scheduling the next attempt
            });
        }, delay);
    }

    #stopReconnect() {
        if (this.#reconnectTimeout) {
            clearTimeout(this.#reconnectTimeout);
            this.#reconnectTimeout = null;
        }
        this.#reconnectAttempts = 0;
    }

    #startPingInterval() {
        this.#pingInterval = setInterval(
            () => this.#send(ClientOpCodes.Ping, undefined),
            30_000
        );
    }

    #stopPingInterval() {
        if (!this.#pingInterval) return;

        clearInterval(this.#pingInterval);
        this.#pingInterval = null;

    }

    #sendIdentify() {
        if (!this.#clientId) return;

        this.#send<IdentifyPayload>(ClientOpCodes.Identify, {
            bot_id: this.#clientId
        });
    }

    sendVoiceUpdate(data: VoiceUpdatePayload) {
        this.#send(ClientOpCodes.VoiceUpdate, data);
    }

    sendPlay(data: PlayPayload) {
        this.#send(ClientOpCodes.Play, data);
    }

    sendPause(guildId: string) {
        this.#send<GuildPayload>(ClientOpCodes.Pause, { guild_id: guildId });
    }

    sendResume(guildId: string) {
        this.#send<GuildPayload>(ClientOpCodes.Resume, { guild_id: guildId });
    }

    sendStop(guildId: string) {
        this.#send<GuildPayload>(ClientOpCodes.Stop, { guild_id: guildId });
    }

    sendSeek(data: SeekPayload) {
        this.#send(ClientOpCodes.Seek, data);
    }

    sendDisconnect(guildId: string) {
        this.#send<GuildPayload>(ClientOpCodes.Disconnect, { guild_id: guildId });
    }

    sendVolume(data: VolumePayload) {
        this.#send(ClientOpCodes.Volume, data);
    }

    sendPlayerMigrate(guildId: string) {
        this.#send<PlayerMigratePayload>(ClientOpCodes.PlayerMigrate, { guild_id: guildId });
    }

    #send<T>(op: number, data: T) {
        if (!this.#ws || this.#ws.readyState !== WebSocket.OPEN) {
            throw new Error(`Node ${this.name} is not connected`);
        }

        const message: Message<T> = { op, d: data };
        this.#ws.send(JSON.stringify(message));
    }
}