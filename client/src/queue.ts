import type { Player, PlayOptions } from "./player.js";
import { EventName } from "./types.js";
import { unwrap } from "./utils.js";

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

    start() {
        if (this.#tracks.length === 0) return false;
        this.#active = true;

        return this.#playCurrentTrack();
    }

    async skip() {
        if (!this.#active) return false;

        if (this.#tracks.length === 0) {
            this.#active = false;
            await this.#player.stop();
            return true;
        }

        return this.#playCurrentTrack(true);
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

    _onTrackEnd(finished: boolean, isFromSkip = false) {
        if (!this.#active || !finished) return;

        if (this.#tracks.length === 0) {
            this.#active = false;

            if (!isFromSkip) {
                this.#player._onQueueEmpty();
            }

            return;
        }

        void this.#playCurrentTrack(isFromSkip);
    }

    _deactivate() {
        this.#active = false;
    }

    async #playCurrentTrack(isFromSkip = false) {
        const item = this.#tracks.shift();
        if (!item) return false;

        const [, error] = await unwrap(this.#player.play(item.uri, item.options, true));
        if (!error) return true;

        const message = error instanceof Error ? error.message : String(error);
        this.#player.node.emit(EventName.QueueError, { guild_id: this.#player.guildId, item, error: message });
        this._onTrackEnd(true, isFromSkip);

        if (!this.#active && !isFromSkip) {
            this.#player._onQueueEmpty();
        }

        return this.#active;
    }
}