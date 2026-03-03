import { EventEmitter } from "node:events";

import { RESTClient } from "./rest.js";
import type {
    ClientMessage, Events,
    PlayPayload,
    SeekPayload,
    ServerMessage,
    VoiceUpdatePayload,
    VolumePayload
} from "./types.js";
import {
    ClientOpCodes,
    EventName,
    Routes,
    ServerOpCodes
} from "./types.js";
import { unwrap } from "./utils.js";

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
    private static readonly NODE_PING_INTERVAL = 30_000;

    readonly name: string;
    readonly url: string;
    readonly rest: RESTClient;
    readonly #options: Required<Omit<NodeOptions, "name" | "url">>;

    #ws: WebSocket | null = null;
    #sessionId: string | null = null;
    #reconnectAttempts = 0;
    #reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
    #pingInterval: ReturnType<typeof setInterval> | null = null;
    #state: NodeState = NodeState.Disconnected;
    #playerCount = 0;

    constructor(options: NodeOptions) {
        super();
        this.name = options.name;
        this.url = options.url;
        this.rest = new RESTClient(options.url);
        this.#options = {
            autoReconnect: options.autoReconnect ?? true,
            reconnectDelay: options.reconnectDelay ?? 5_000,
            maxReconnectAttempts: options.maxReconnectAttempts ?? 10
        };
    }

    connect() {
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
                resolve(null);
            };

            const onError = (event: Event) => {
                const message = "message" in event ? String((event as { message: unknown; }).message) : "unknown";
                const error = new Error(`WebSocket error: ${message} (attempt ${this.#reconnectAttempts + 1}/${this.#options.maxReconnectAttempts})`);
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
        return this.#state === NodeState.Connected || this.#state === NodeState.Draining;
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
            const message = JSON.parse(event.data as string) as ServerMessage;
            this.#handleMessage(message);
        } catch {
            // Invalid messages are silently ignored
        }
    }

    #handleMessage(message: ServerMessage) {
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
            case ServerOpCodes.VoiceConnect:
                this.emit(EventName.VoiceConnect, message.d);
                break;
            case ServerOpCodes.VoiceDisconnect:
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
        if (this.#reconnectTimeout) return;

        const delay = this.#options.reconnectDelay * (2) ** (this.#reconnectAttempts);
        this.#reconnectAttempts++;

        this.#reconnectTimeout = setTimeout(() => {
            this.#reconnectTimeout = null;
            void unwrap(this.connect());
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
            Node.NODE_PING_INTERVAL
        );
    }

    #stopPingInterval() {
        if (!this.#pingInterval) return;

        clearInterval(this.#pingInterval);
        this.#pingInterval = null;
    }

    sendVoiceUpdate(data: VoiceUpdatePayload) {
        this.#send(ClientOpCodes.VoiceUpdate, data);
    }

    sendPlayerMigrate(guildId: string) {
        this.#send(ClientOpCodes.PlayerMigrate, { guild_id: guildId });
    }

    async sendPlay(guildId: string, data: PlayPayload) {
        await this.rest.post(Routes.play(this.#requireSession(), guildId), data);
    }

    async sendPause(guildId: string) {
        await this.rest.post(Routes.pause(this.#requireSession(), guildId));
    }

    async sendResume(guildId: string) {
        await this.rest.post(Routes.resume(this.#requireSession(), guildId));
    }

    async sendStop(guildId: string) {
        await this.rest.post(Routes.stop(this.#requireSession(), guildId));
    }

    async sendSeek(guildId: string, data: SeekPayload) {
        await this.rest.post(Routes.seek(this.#requireSession(), guildId), data);
    }

    async sendVolume(guildId: string, data: VolumePayload) {
        await this.rest.patch(Routes.volume(this.#requireSession(), guildId), data);
    }

    async sendDisconnect(guildId: string) {
        await this.rest.delete(Routes.disconnect(this.#requireSession(), guildId));
    }

    #requireSession(): string {
        if (!this.#sessionId) {
            throw new Error(`Node ${this.name} has no active session`);
        }
        return this.#sessionId;
    }

    #send<T extends ClientOpCodes>(op: T, data: Extract<ClientMessage, { op: T; }>["d"]) {
        if (!this.#ws || this.#ws.readyState !== WebSocket.OPEN) {
            throw new Error(`Node ${this.name} is not connected`);
        }

        const message = { op, d: data };
        this.#ws.send(JSON.stringify(message));
    }
}