import type { Filter, FiltersPayload } from "./types.js";

export class PlayerFilters {
    #state = new Map<Filter, boolean>();
    #pitch = 0;
    #speed = 0;

    /**
     * Pitch multiplier applied on top of any preset pitch.
     *
     * - **Default:** `0` (no override — preset value is used as-is)
     * - **Normal playback:** `1.0`
     * - **Recommended range:** `0.5` – `2.0`
     * - Values below `0` are clamped to `0`. Values above `2.0` will work
     *   but progressively degrade audio quality due to resampling artifacts.
     *
     * When a preset like {@link Filter.Nightcore} is active (which sets pitch
     * to `1.3×`), this value is **multiplied** on top: e.g. `pitch = 0.5`
     * with Nightcore → effective pitch = `1.3 × 0.5 = 0.65`.
     */
    get pitch() {
        return this.#pitch;
    }

    set pitch(value: number) {
        this.#pitch = Math.max(0, value);
    }

    /**
     * Speed multiplier applied on top of any preset speed.
     *
     * - **Default:** `0` (no override — preset value is used as-is)
     * - **Normal playback:** `1.0`
     * - **Recommended range:** `0.25` – `3.0`
     * - Values below `0` are clamped to `0`. Extreme values (e.g. `>5.0`)
     *   will work but may cause audible quality loss since more source
     *   samples are consumed per output frame.
     *
     * When a preset like {@link Filter.Vaporwave} is active (which sets speed
     * to `0.8×`), this value is **multiplied** on top: e.g. `speed = 1.25`
     * with Vaporwave → effective speed = `0.8 × 1.25 = 1.0`.
     */
    get speed() {
        return this.#speed;
    }

    set speed(value: number) {
        this.#speed = Math.max(0, value);
    }

    /**
     * Toggle a filter on or off. If `enabled` is omitted the filter is
     * flipped from its current state.
     *
     * **Preset filters** ({@link Filter.Nightcore}, {@link Filter.Vaporwave})
     * adjust both speed and pitch by fixed amounts (1.3× and 0.8×
     * respectively). Enabling both simultaneously multiplies their effects
     * together (effective speed = `1.3 × 0.8 = 1.04`).
     *
     * **DSP filters** ({@link Filter.Tremolo}, {@link Filter.Vibrato},
     * {@link Filter.Rotation}, {@link Filter.LowPass}) modify the audio
     * signal in-place and can all be enabled simultaneously — they are
     * applied in sequence (tremolo → vibrato → rotation → lowpass).
     */
    toggle(filter: Filter, enabled?: boolean) {
        const next = enabled ?? !this.#state.get(filter);
        this.#state.set(filter, next);
        return this;
    }

    /**
     * @returns `true` if any filter is active or if pitch or speed are non-zero.
     */
    get active() {
        for (const v of this.#state.values()) {
            if (v) return true;
        }
        return this.#pitch > 0 || this.#speed > 0;
    }

    /**
     * @returns an array of all active filters.
     */
    get activeFilters() {
        const result: Filter[] = [];
        for (const [k, v] of this.#state) if (v) result.push(k);
        return result;
    }

    get(filter: Filter) {
        return this.#state.get(filter);
    }

    clear() {
        this.#state.clear();
        this.#pitch = 0;
        this.#speed = 0;
    }

    toPayload() {
        if (!this.active) return undefined;

        const enabled: Filter[] = [];
        for (const [k, v] of this.#state) {
            if (v) enabled.push(k);
        }

        const payload: FiltersPayload = {};
        if (enabled.length > 0) payload.enabled = enabled;
        if (this.#pitch > 0) payload.pitch = this.#pitch;
        if (this.#speed > 0) payload.speed = this.#speed;

        return payload;
    }
}