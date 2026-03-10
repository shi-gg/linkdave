import type { GatewayDispatchPayload, GatewayVoiceStateUpdate } from "discord-api-types/v10";
import { GatewayDispatchEvents } from "discord-api-types/v10";
import { EventEmitter } from "node:events";

import { Node, type NodeOptions } from "./node.js";
import { Player, type PlayerOptions } from "./player.js";
import type {
    ClosePayload,
    Events,
    ManagerEvents,
    MigrateReadyPayload,
    NodeDrainingPayload,
    PlayerUpdatePayload,
    TrackEndPayload,
    TrackStartPayload,
    VoiceDisconnectPayload
} from "./types.js";
import {
    EventName,
    ManagerEventName
} from "./types.js";

const CLIENT_ID_REGEX = /^\d{15,21}$/;

export type SendToShardFn = (guildId: string, payload: GatewayVoiceStateUpdate) => void;

export interface LinkDaveClientOptions {
    token: string;
    nodes?: NodeOptions[];
    sendToShard: SendToShardFn;
}

// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging
export interface LinkDaveClient {
    on: <K extends keyof ManagerEvents>(event: K, listener: (data: ManagerEvents[K]) => void) => this;
    once: <K extends keyof ManagerEvents>(event: K, listener: (data: ManagerEvents[K]) => void) => this;
    off: <K extends keyof ManagerEvents>(event: K, listener: (data: ManagerEvents[K]) => void) => this;
    emit: <K extends keyof ManagerEvents>(event: K, data: ManagerEvents[K]) => boolean;
}

// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging
export class LinkDaveClient extends EventEmitter {
    readonly #clientId: string;
    readonly #sendToShard: SendToShardFn;
    readonly #nodes = new Map<string, Node>();
    readonly #players = new Map<string, Player>();

    constructor(options: LinkDaveClientOptions) {
        super();
        this.#sendToShard = options.sendToShard;

        this.#clientId = Buffer
            .from(options.token.split(".")[0]!, "base64")
            .toString();

        if (!CLIENT_ID_REGEX.test(this.#clientId)) {
            throw new Error("Invalid token provided: decoded client ID is not a valid snowflake.");
        }

        if (options.nodes) {
            for (const nodeOpts of options.nodes) {
                this.addNode(nodeOpts);
            }
        }
    }

    addNode(options: NodeOptions) {
        if (this.#nodes.has(options.name)) {
            throw new Error(`Node "${options.name}" already exists`);
        }

        const node = new Node(options);
        this.#setupNodeListeners(node);
        this.#nodes.set(options.name, node);
        this.emit(ManagerEventName.NodeAdd, { node });

        return node;
    }

    removeNode(name: string) {
        const node = this.#nodes.get(name);
        if (!node) return false;

        this.#nodes.delete(name);
        node.disconnect();

        this.emit(ManagerEventName.NodeRemove, { node });

        return true;
    }

    get nodes(): ReadonlyMap<string, Node> {
        return this.#nodes;
    }

    async connectAll() {
        const promises = Array.from(this.#nodes.values(), (node) => node.connect());
        await Promise.allSettled(promises);
    }

    disconnectAll() {
        for (const node of this.#nodes.values()) {
            node.disconnect();
        }
    }

    getBestNode() {
        let bestNode: Node | undefined;
        let lowestCount = Infinity;

        for (const node of this.#nodes.values()) {
            if (!node.connected || node.draining) {
                continue;
            }

            if (node.playerCount < lowestCount) {
                lowestCount = node.playerCount;
                bestNode = node;
            }
        }

        return bestNode;
    }

    getPlayer(guildId: string, options?: Omit<PlayerOptions, "guildId">) {
        let player = this.#players.get(guildId);
        if (player) return player;

        const node = this.getBestNode();
        if (!node) {
            throw new Error("No available nodes to create player");
        }

        player = new Player(this, guildId, node, options);

        this.#players.set(guildId, player);
        node.incrementPlayerCount();

        return player;
    }

    removePlayer(guildId: string) {
        const player = this.#players.get(guildId);
        if (!player) {
            return false;
        }

        if (player.connected) {
            throw new Error(`Cannot remove player for guild "${guildId}" while it is still connected`);
        }

        player.node.decrementPlayerCount();
        this.#players.delete(guildId);

        return true;
    }

    get players(): ReadonlyMap<string, Player> {
        return this.#players;
    }

    get clientId() {
        return this.#clientId;
    }

    async handleRaw({ t: event, d: data }: GatewayDispatchPayload) {
        switch (event) {
            case GatewayDispatchEvents.VoiceStateUpdate: {
                // https://discord.com/developers/docs/resources/voice#voice-state-object
                if (!data.guild_id) return;
                if (data.user_id !== this.#clientId) return;

                const player = this.#players.get(data.guild_id);
                await player?.handleVoiceStateUpdate({
                    user_id: data.user_id,
                    channel_id: data.channel_id,
                    session_id: data.session_id
                });

                break;
            }
            case GatewayDispatchEvents.VoiceServerUpdate: {
                const player = this.#players.get(data.guild_id);
                await player?.handleVoiceServerUpdate(data);

                break;
            }
        }
    }

    _sendToShard(guildId: string, payload: GatewayVoiceStateUpdate) {
        this.#sendToShard(guildId, payload);
    }

    #setupNodeListeners(node: Node) {
        node.on(EventName.Ready, (data) => this.emit(EventName.Ready, data));
        node.on(EventName.PlayerUpdate, (data) => this.#handlePlayerUpdate(node, data));

        node.on(EventName.TrackStart, (data) => this.#handleTrackStart(node, data));
        node.on(EventName.TrackEnd, (data) => this.#handleTrackEnd(node, data));
        node.on(EventName.TrackError, (data) => this.#forwardPlayerEvent(node, data.guild_id, EventName.TrackError, data));
        node.on(EventName.QueueError, (data) => this.#forwardPlayerEvent(node, data.guild_id, EventName.QueueError, data));
        node.on(EventName.VoiceConnect, (data) => this.#forwardPlayerEvent(node, data.guild_id, EventName.VoiceConnect, data));
        node.on(EventName.VoiceDisconnect, (data) => this.#handleVoiceDisconnect(node, data));

        node.on(EventName.Pong, () => this.emit(EventName.Pong, undefined));
        node.on(EventName.Stats, (data) => this.emit(EventName.Stats, data));

        node.on(EventName.NodeDraining, (data) => this.#handleNodeDraining(node, data));
        node.on(EventName.MigrateReady, (data) => this.#handleMigrateReady(node, data));

        node.on(EventName.Close, (data) => this.#handleClose(node, data));
        node.on(EventName.Error, (data) => this.emit(EventName.Error, data));
    }

    #forwardPlayerEvent<K extends keyof Events>(
        node: Node,
        guildId: string,
        event: K,
        data: Events[K]
    ) {
        const player = this.#players.get(guildId);
        if (player?.node !== node) return;

        this.emit(event, data as ManagerEvents[K]);
    }

    #handlePlayerUpdate(node: Node, data: PlayerUpdatePayload) {
        const player = this.#players.get(data.guild_id);
        if (player?.node !== node) return;

        player._updateState(data);
        this.emit(EventName.PlayerUpdate, data);
    }

    #handleTrackStart(node: Node, data: TrackStartPayload) {
        const player = this.#players.get(data.guild_id);
        if (player?.node !== node) return;

        player._onTrackStart(data);
        this.emit(EventName.TrackStart, data);
    }

    #handleTrackEnd(node: Node, data: TrackEndPayload) {
        const player = this.#players.get(data.guild_id);
        if (player?.node !== node) return;

        player._onTrackEnd(data);
        this.emit(EventName.TrackEnd, data);
    }

    #handleVoiceDisconnect(node: Node, data: VoiceDisconnectPayload) {
        const player = this.#players.get(data.guild_id);
        if (player?.node !== node) return;

        player._onVoiceDisconnect();
        this.emit(EventName.VoiceDisconnect, data);
    }

    async #handleNodeDraining(node: Node, data: NodeDrainingPayload) {
        this.emit(EventName.NodeDraining, data);

        const promises = [];
        for (const player of this.#players.values()) {
            if (player.node !== node) continue;

            const targetNode = this.getBestNode();
            if (!targetNode) {
                promises.push(player.destroy());
                continue;
            }

            promises.push(player.moveNode(targetNode));
        }
        await Promise.allSettled(promises);
    }

    #handleMigrateReady(_node: Node, data: MigrateReadyPayload) {
        this.emit(EventName.MigrateReady, data);

        const player = this.#players.get(data.guild_id);
        if (!player) return;

        player._onMigrateReady(data);
    }

    async #handleClose(node: Node, data: ClosePayload) {
        this.emit(EventName.Close, data);

        const promises = [];
        for (const player of this.#players.values()) {
            if (player.node !== node) continue;
            promises.push(player.destroy());
        }
        await Promise.allSettled(promises);
    }

    _updatePlayerNode(guildId: string, oldNode: Node, newNode: Node) {
        const player = this.#players.get(guildId);
        if (player?.node !== oldNode) return;

        oldNode.decrementPlayerCount();
        newNode.incrementPlayerCount();
    }
}