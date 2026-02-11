import type { GatewayDispatchPayload, GatewayVoiceStateUpdate } from "discord-api-types/v10";
import { GatewayDispatchEvents } from "discord-api-types/v10";
import { EventEmitter } from "node:events";

import { Node, type NodeOptions } from "./node.js";
import { Player, type PlayerOptions } from "./player.js";
import type {
    Events,
    ManagerEvents,
    MigrateReadyPayload,
    NodeDrainingPayload,
    PlayerUpdatePayload
} from "./types.js";
import {
    EventName,
    ManagerEventName
} from "./types.js";

export type SendToShardFn = (guildId: string, payload: GatewayVoiceStateUpdate) => void;

export interface LinkDaveClientOptions {
    clientId: string;
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
        this.#clientId = options.clientId;
        this.#sendToShard = options.sendToShard;

        if (options.nodes) {
            for (const nodeOpts of options.nodes) {
                this.addNode(nodeOpts);
            }
        }
    }

    addNode(options: NodeOptions): Node {
        if (this.#nodes.has(options.name)) {
            throw new Error(`Node "${options.name}" already exists`);
        }

        const node = new Node(options);
        this.#setupNodeListeners(node);
        this.#nodes.set(options.name, node);
        this.emit(ManagerEventName.NodeAdd, { node });

        return node;
    }

    removeNode(name: string): boolean {
        const node = this.#nodes.get(name);
        if (!node) return false;

        node.disconnect();
        this.#nodes.delete(name);
        this.emit(ManagerEventName.NodeRemove, { node });

        return true;
    }

    getNode(name: string): Node | undefined {
        return this.#nodes.get(name);
    }

    get nodes(): Map<string, Node> {
        return this.#nodes;
    }

    async connectAll(): Promise<void> {
        const promises = Array.from(this.#nodes.values(), (node) =>
            node.connect(this.#clientId).catch(() => {
                // Ignore connection errors during initial connect
            }));

        await Promise.all(promises);
    }

    disconnectAll(): void {
        for (const node of this.#nodes.values()) {
            node.disconnect();
        }
    }

    getBestNode(): Node | undefined {
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

    getPlayer(guildId: string, options?: Omit<PlayerOptions, "guildId">): Player {
        let player = this.#players.get(guildId);
        if (player) {
            return player;
        }

        const node = this.getBestNode();
        if (!node) {
            throw new Error("No available nodes to create player");
        }

        player = new Player(this, guildId, node, options);
        this.#players.set(guildId, player);
        node.incrementPlayerCount();

        return player;
    }

    getExistingPlayer(guildId: string): Player | undefined {
        return this.#players.get(guildId);
    }

    removePlayer(guildId: string): boolean {
        const player = this.#players.get(guildId);
        if (!player) {
            return false;
        }

        player.node.decrementPlayerCount();
        this.#players.delete(guildId);
        return true;
    }

    getPlayerNode(guildId: string): Node | undefined {
        return this.#players.get(guildId)?.node;
    }

    get players(): Map<string, Player> {
        return new Map(this.#players);
    }

    get clientId(): string {
        return this.#clientId;
    }

    handleRaw({ t: event, d: data }: GatewayDispatchPayload): void {
        switch (event) {
            case GatewayDispatchEvents.VoiceStateUpdate: {
                // I am not sure in what cases guild_id would be null, DMs maybe?
                // https://discord.com/developers/docs/resources/voice#voice-state-object
                if (!data.guild_id) return;
                if (data.user_id !== this.#clientId) return;

                const player = this.#players.get(data.guild_id);
                player?.handleVoiceStateUpdate({
                    channel_id: data.channel_id,
                    session_id: data.session_id
                });

                break;
            }
            case GatewayDispatchEvents.VoiceServerUpdate: {
                const player = this.#players.get(data.guild_id);
                player?.handleVoiceServerUpdate(data);

                break;
            }
        }
    }

    _sendToShard(guildId: string, payload: GatewayVoiceStateUpdate): void {
        this.#sendToShard(guildId, payload);
    }

    #setupNodeListeners(node: Node): void {
        node.on(EventName.Ready, (data) => this.emit(EventName.Ready, data));
        node.on(EventName.PlayerUpdate, (data) => this.#handlePlayerUpdate(node, data));

        node.on(EventName.TrackStart, (data) => this.#forwardPlayerEvent(node, data.guild_id, EventName.TrackStart, data));
        node.on(EventName.TrackEnd, (data) => this.#forwardPlayerEvent(node, data.guild_id, EventName.TrackEnd, data));
        node.on(EventName.TrackError, (data) => this.#forwardPlayerEvent(node, data.guild_id, EventName.TrackError, data));
        node.on(EventName.VoiceConnect, (data) => this.#forwardPlayerEvent(node, data.guild_id, EventName.VoiceConnect, data));
        node.on(EventName.VoiceDisconnect, (data) => this.#forwardPlayerEvent(node, data.guild_id, EventName.VoiceDisconnect, data));

        node.on(EventName.Pong, () => this.emit(EventName.Pong, undefined));
        node.on(EventName.Stats, (data) => this.emit(EventName.Stats, data));

        node.on(EventName.NodeDraining, (data) => this.#handleNodeDraining(node, data));
        node.on(EventName.MigrateReady, (data) => this.#handleMigrateReady(node, data));

        node.on(EventName.Close, (data) => this.emit(EventName.Close, data));
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

    #handlePlayerUpdate(node: Node, data: PlayerUpdatePayload): void {
        const player = this.#players.get(data.guild_id);
        if (player?.node !== node) return;

        player._updateState(data);
        this.emit(EventName.PlayerUpdate, data);
    }

    #handleNodeDraining(node: Node, data: NodeDrainingPayload): void {
        this.emit(EventName.NodeDraining, data);

        for (const player of this.#players.values()) {
            if (player.node !== node) continue;

            const targetNode = this.#findMigrationTarget(node);
            if (!targetNode) {
                player.destroy();
                continue;
            }

            void player.moveNode(targetNode);
        }
    }

    #handleMigrateReady(_node: Node, data: MigrateReadyPayload): void {
        this.emit(EventName.MigrateReady, data);

        const player = this.#players.get(data.guild_id);
        if (!player) return;

        player._onMigrateReady(data);
    }

    #findMigrationTarget(excludeNode: Node): Node | undefined {
        let bestNode: Node | undefined;
        let lowestCount = Infinity;

        for (const node of this.#nodes.values()) {
            if (node === excludeNode || !node.connected || node.draining) {
                continue;
            }

            if (node.playerCount < lowestCount) {
                lowestCount = node.playerCount;
                bestNode = node;
            }
        }

        return bestNode;
    }

    _updatePlayerNode(guildId: string, oldNode: Node, newNode: Node): void {
        const player = this.#players.get(guildId);
        if (player?.node !== oldNode) return;

        oldNode.decrementPlayerCount();
        newNode.incrementPlayerCount();
    }
}