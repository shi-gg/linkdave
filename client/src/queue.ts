import type { Player } from "./player.js";

export class Queue {
    readonly #player: Player;
    readonly #tracks: string[] = [];
    #active = false;

    constructor(player: Player) {
        this.#player = player;
    }

    add(uri: string) {
        this.#tracks.push(uri);
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

    get tracks(): readonly string[] {
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

        void this.#playCurrentTrack();
    }

    _deactivate() {
        this.#active = false;
    }

    async #playCurrentTrack() {
        const item = this.#tracks.shift();
        if (!item) return;

        await this.#player.play(item, undefined, true);
    }
}