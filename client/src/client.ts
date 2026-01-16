import { EventEmitter } from "node:events";

import { Node, type NodeOptions } from "./node.js";
import { Player, type PlayerOptions } from "./player.js";
import type {
    LinkDaveEvents,
    MigrateReadyPayload,
    NodeDrainingPayload,
    PlayerUpdatePayload
} from "./types.js";


export interface GatewayPayload {
    op: number;
    d: unknown;
}

export type SendToShardFn = (guildId: string, payload: GatewayPayload) => void;

export interface LinkDaveClientOptions {
    clientId: string;
    nodes?: NodeOptions[];
    sendToShard: SendToShardFn;
}

export interface LinkDaveManagerEvents extends LinkDaveEvents {
    nodeAdded: { node: Node; };
    nodeRemoved: { node: Node; };
    nodeReconnecting: { node: Node; attempt: number; };
}

// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging
export interface LinkDaveClient {
    on: <K extends keyof LinkDaveManagerEvents>(event: K, listener: (data: LinkDaveManagerEvents[K]) => void) => this;
    once: <K extends keyof LinkDaveManagerEvents>(event: K, listener: (data: LinkDaveManagerEvents[K]) => void) => this;
    off: <K extends keyof LinkDaveManagerEvents>(event: K, listener: (data: LinkDaveManagerEvents[K]) => void) => this;
    emit: <K extends keyof LinkDaveManagerEvents>(event: K, data: LinkDaveManagerEvents[K]) => boolean;
}

// eslint-disable-next-line @typescript-eslint/no-unsafe-declaration-merging
export class LinkDaveClient extends EventEmitter {
    readonly #clientId: string;
    readonly #sendToShard: SendToShardFn;
    readonly #nodes = new Map<string, Node>();
    readonly #players = new Map<string, Player>();
    readonly #playerNodes = new Map<string, Node>();

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
        this.emit("nodeAdded", { node });

        return node;
    }

    removeNode(name: string): boolean {
        const node = this.#nodes.get(name);
        if (!node) return false;

        node.disconnect();
        this.#nodes.delete(name);
        this.emit("nodeRemoved", { node });

        return true;
    }

    getNode(name: string): Node | undefined {
        return this.#nodes.get(name);
    }

    get nodes(): Map<string, Node> {
        return this.#nodes;
    }

    async connectAll(): Promise<void> {
        const promises = [...this.#nodes.values()].map((node) =>
            node.connect(this.#clientId).catch(() => {
                // Ignore connection errors during initial connect
            })
        );

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
        this.#playerNodes.set(guildId, node);
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

        const node = this.#playerNodes.get(guildId);
        if (node) {
            node.decrementPlayerCount();
        }

        this.#players.delete(guildId);
        this.#playerNodes.delete(guildId);
        return true;
    }

    getPlayerNode(guildId: string): Node | undefined {
        return this.#playerNodes.get(guildId);
    }

    get players(): Map<string, Player> {
        return new Map(this.#players);
    }

    get clientId(): string {
        return this.#clientId;
    }

    handleRaw(packet: { t: string; d: unknown; }): void {
        if (packet.t === "VOICE_STATE_UPDATE") {
            const data = packet.d as {
                guild_id: string;
                channel_id: string | null;
                user_id: string;
                session_id: string;
            };

            if (data.user_id !== this.#clientId) return;

            const player = this.#players.get(data.guild_id);
            player?.handleVoiceStateUpdate({
                channel_id: data.channel_id,
                session_id: data.session_id
            });
        }

        if (packet.t === "VOICE_SERVER_UPDATE") {
            const data = packet.d as {
                guild_id: string;
                token: string;
                endpoint: string;
            };

            const player = this.#players.get(data.guild_id);
            player?.handleVoiceServerUpdate(data);
        }
    }

    _sendToShard(guildId: string, payload: GatewayPayload): void {
        this.#sendToShard(guildId, payload);
    }

    #setupNodeListeners(node: Node): void {
        node.on("ready", (data) => this.emit("ready", data));
        node.on("playerUpdate", (data) => this.#handlePlayerUpdate(node, data));
        node.on("trackStart", (data) => this.#forwardPlayerEvent(node, data.guild_id, "trackStart", data));
        node.on("trackEnd", (data) => this.#forwardPlayerEvent(node, data.guild_id, "trackEnd", data));
        node.on("trackError", (data) => this.#forwardPlayerEvent(node, data.guild_id, "trackError", data));
        node.on("voiceConnected", (data) => this.#forwardPlayerEvent(node, data.guild_id, "voiceConnected", data));
        node.on("voiceDisconnected", (data) => this.#forwardPlayerEvent(node, data.guild_id, "voiceDisconnected", data));
        node.on("pong", () => this.emit("pong", undefined));
        node.on("stats", (data) => this.emit("stats", data));
        node.on("nodeDraining", (data) => this.#handleNodeDraining(node, data));
        node.on("migrateReady", (data) => this.#handleMigrateReady(node, data));
        node.on("close", (data) => this.emit("close", data));
        node.on("error", (data) => this.emit("error", data));
    }

    #forwardPlayerEvent<K extends keyof LinkDaveEvents>(
        node: Node,
        guildId: string,
        event: K,
        data: LinkDaveEvents[K]
    ) {
        if (this.#playerNodes.get(guildId) !== node) return;
        this.emit(event, data as LinkDaveManagerEvents[K]);
    }

    #handlePlayerUpdate(node: Node, data: PlayerUpdatePayload): void {
        if (this.#playerNodes.get(data.guild_id) !== node) {
            return;
        }

        const player = this.#players.get(data.guild_id);
        if (player) player._updateState(data);

        this.emit("playerUpdate", data);
    }

    #handleNodeDraining(node: Node, data: NodeDrainingPayload): void {
        this.emit("nodeDraining", data);

        for (const [guildId, playerNode] of this.#playerNodes) {
            if (playerNode !== node) continue;

            const player = this.#players.get(guildId);
            if (!player) continue;

            const targetNode = this.#findMigrationTarget(node);
            if (!targetNode) {
                player.destroy();
                continue;
            }

            void player.moveNode(targetNode);
        }
    }

    #handleMigrateReady(_node: Node, data: MigrateReadyPayload): void {
        this.emit("migrateReady", data);

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
        if (this.#playerNodes.get(guildId) !== oldNode) return;

        oldNode.decrementPlayerCount();
        this.#playerNodes.set(guildId, newNode);
        newNode.incrementPlayerCount();
    }
}