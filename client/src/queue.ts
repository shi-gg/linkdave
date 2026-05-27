import type { Player, PlayOptions } from "./player.js";
import { EventName } from "./types.js";

export interface QueueItem {
    uri: string;
    options: PlayOptions;
}

export class Queue {
    readonly #player: Player;
    readonly #tracks: QueueItem[] = [];
    #active = false;

    constructor(player: Player) {
        this.#player = player;
    }

    add(uri: string, options: PlayOptions = {}) {
        this.#tracks.push({ uri, options });
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

        this.#player
            .play(item.uri, item.options, true)
            .then(
                () => null,
                (error: unknown) => {
                    const message = error instanceof Error ? error.message : String(error);
                    this.#player.node.emit(EventName.QueueError, { guild_id: this.#player.guildId, item, error: message });
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

        await this.#player.play(item.uri, item.options, true);
    }
}