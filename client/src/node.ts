import { EventEmitter } from "node:events";

import type {
    Events,
    Message,
    PlayPayload,
    SeekPayload,
    VoiceUpdatePayload,
    VolumePayload
} from "./types.js";
import {
    ClientOpCodes,
    EventName,
    ServerOpCodes
} from "./types.js";

export interface NodeOptions {
    name: string;
    url: string;
    autoReconnect?: boolean;
    reconnectDelay?: number;
    maxReconnectAttempts?: number;
}

export enum NodeState {
    Disconnected = 0,
    Connecting = 1,
    Connected = 2,
    Draining = 3
}

// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging
export interface Node {
    on: <K extends keyof Events>(event: K, listener: (data: Events[K]) => void) => this;
    once: <K extends keyof Events>(event: K, listener: (data: Events[K]) => void) => this;
    off: <K extends keyof Events>(event: K, listener: (data: Events[K]) => void) => this;
    emit: <K extends keyof Events>(event: K, data: Events[K]) => boolean;
}

// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging
export class Node extends EventEmitter {
    private static readonly NODE_PING_TIMEOUT = 30_000;

    readonly name: string;
    readonly url: string;
    readonly #options: Required<Omit<NodeOptions, "name" | "url">>;

    #ws: WebSocket | null = null;
    #sessionId: string | null = null;
    #clientId: string | null = null;
    #reconnectAttempts = 0;
    #reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
    #pingInterval: ReturnType<typeof setInterval> | null = null;
    #state: NodeState = NodeState.Disconnected;
    #playerCount = 0;

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
        this.#state = NodeState.Connecting;

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
                this.#state = NodeState.Connected;
                this.#reconnectAttempts = 0;
                this.#startPingInterval();
                this.#sendIdentify();
                resolve(null);
            };

            const onError = (event: Event) => {
                const message = "message" in event ? String((event as { message: unknown; }).message) : "unknown";
                const error = new Error(`WebSocket error: ${message}`);
                this.emit(EventName.Error, error);
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

        this.#state = NodeState.Disconnected;
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
        return this.state === NodeState.Draining;
    }

    get connected() {
        return this.#state === NodeState.Connected;
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
                this.#sessionId = message.d.session_id;
                this.emit(EventName.Ready, message.d);
                break;
            case ServerOpCodes.PlayerUpdate:
                this.emit(EventName.PlayerUpdate, message.d);
                break;
            case ServerOpCodes.TrackStart:
                this.emit(EventName.TrackStart, message.d);
                break;
            case ServerOpCodes.TrackEnd:
                this.emit(EventName.TrackEnd, message.d);
                break;
            case ServerOpCodes.TrackError:
                this.emit(EventName.TrackError, message.d);
                break;
            case ServerOpCodes.VoiceConnected:
                this.emit(EventName.VoiceConnect, message.d);
                break;
            case ServerOpCodes.VoiceDisconnected:
                this.emit(EventName.VoiceDisconnect, message.d);
                break;
            case ServerOpCodes.Pong:
                this.emit(EventName.Pong, undefined);
                break;
            case ServerOpCodes.Stats:
                this.#playerCount = message.d.players;
                this.#state = message.d.draining ? NodeState.Draining : NodeState.Connected;
                this.emit(EventName.Stats, message.d);
                break;
            case ServerOpCodes.NodeDraining:
                this.#state = NodeState.Draining;
                this.emit(EventName.NodeDraining, message.d);
                break;
            case ServerOpCodes.MigrateReady:
                this.emit(EventName.MigrateReady, message.d);
                break;
        }
    }

    #onClose(event: CloseEvent) {
        this.#state = NodeState.Disconnected;
        this.#stopPingInterval();

        this.emit(EventName.Close, { code: event.code, reason: event.reason });

        if (this.#options.autoReconnect && !this.draining && this.#reconnectAttempts < this.#options.maxReconnectAttempts) {
            this.#scheduleReconnect();
        }
    }

    #scheduleReconnect() {
        if (this.#reconnectTimeout || !this.#clientId) return;

        const delay = this.#options.reconnectDelay * (2) ** (this.#reconnectAttempts);
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
            Node.NODE_PING_TIMEOUT
        );
    }

    #stopPingInterval() {
        if (!this.#pingInterval) return;

        clearInterval(this.#pingInterval);
        this.#pingInterval = null;

    }

    #sendIdentify() {
        if (!this.#clientId) return;

        this.#send(ClientOpCodes.Identify, {
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
        this.#send(ClientOpCodes.Pause, { guild_id: guildId });
    }

    sendResume(guildId: string) {
        this.#send(ClientOpCodes.Resume, { guild_id: guildId });
    }

    sendStop(guildId: string) {
        this.#send(ClientOpCodes.Stop, { guild_id: guildId });
    }

    sendSeek(data: SeekPayload) {
        this.#send(ClientOpCodes.Seek, data);
    }

    sendDisconnect(guildId: string) {
        this.#send(ClientOpCodes.Disconnect, { guild_id: guildId });
    }

    sendVolume(data: VolumePayload) {
        this.#send(ClientOpCodes.Volume, data);
    }

    sendPlayerMigrate(guildId: string) {
        this.#send(ClientOpCodes.PlayerMigrate, { guild_id: guildId });
    }

    #send(op: ClientOpCodes, data: unknown) {
        if (!this.#ws || this.#ws.readyState !== WebSocket.OPEN) {
            throw new Error(`Node ${this.name} is not connected`);
        }

        const message = { op, d: data };
        this.#ws.send(JSON.stringify(message));
    }
}