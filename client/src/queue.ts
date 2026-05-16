import type { Player } from "./player.js";
import { EventName, type QueueErrorPayload } from "./types.js";

export interface QueueItem {
    uri: string;
    requesterId?: string;
}

export class Queue {
    readonly #player: Player;
    readonly #tracks: QueueItem[] = [];
    #active = false;

    constructor(player: Player) {
        this.#player = player;
    }

    add(uri: string, requesterId?: string) {
        const item: QueueItem = { uri };
        if (requesterId !== undefined) item.requesterId = requesterId;
        this.#tracks.push(item);
        return this;
    }

    async start() {
        if (this.#tracks.length === 0) return;
        this.#active = true;
        await this.#playCurrentTrack();
    }

    async skip() {
        if (!this.#active) return;

        if (this.#tracks.length === 0) {
            this.#active = false;
            await this.#player.stop();
            return;
        }

        await this.#playCurrentTrack();
    }

    remove(index: number) {
        if (index < 0 || index >= this.#tracks.length) return undefined;
        const [removed] = this.#tracks.splice(index, 1);
        return removed;
    }

    clear() {
        this.#tracks.length = 0;
        this.#active = false;
    }

    get tracks(): readonly QueueItem[] {
        return this.#tracks;
    }

    get size() {
        return this.#tracks.length;
    }

    get active() {
        return this.#active;
    }

    _onTrackEnd(finished: boolean) {
        if (!this.#active || !finished) return;

        if (this.#tracks.length === 0) {
            this.#active = false;
            return;
        }

        const item = this.#tracks.shift();
        if (!item) return;

        const playOptions: { requesterId?: string; } = {};
        if (item.requesterId !== undefined) playOptions.requesterId = item.requesterId;
        this.#player
            .play(item.uri, playOptions, true)
            .then(
                () => null,
                (error_: unknown) => {
                    const error = error_ instanceof Error ? error_ : new Error(String(error_));
                    const payload: QueueErrorPayload = { guild_id: this.#player.guildId, url: item.uri, error };
                    this.#player.node.emit(EventName.QueueError, payload);
                    this._onTrackEnd(true);
                }
            );
    }

    _deactivate() {
        this.#active = false;
    }

    async #playCurrentTrack() {
        const item = this.#tracks.shift();
        if (!item) return;

        const playOptions: { requesterId?: string; } = {};
        if (item.requesterId !== undefined) playOptions.requesterId = item.requesterId;
        await this.#player.play(item.uri, playOptions, true);
    }
}