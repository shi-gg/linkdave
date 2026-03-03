import type { RESTError } from "./types.js";
import { unwrap } from "./utils.js";

export class RESTClient {
    readonly #baseUrl: string;

    constructor(wsUrl: string) {
        const url = new URL(wsUrl);
        url.protocol = url.protocol === "wss:" ? "https:" : "http:";
        this.#baseUrl = url.origin;
    }

    async put(route: string, body?: unknown): Promise<void> {
        await this.#request("PUT", route, body);
    }

    async post(route: string, body?: unknown): Promise<void> {
        await this.#request("POST", route, body);
    }

    async patch(route: string, body?: unknown): Promise<void> {
        await this.#request("PATCH", route, body);
    }

    async delete(route: string): Promise<void> {
        await this.#request("DELETE", route);
    }

    async #request(method: string, route: string, body?: unknown): Promise<void> {
        const url = `${this.#baseUrl}${route}`;
        const init: RequestInit = {
            method,
            headers: { "Content-Type": "application/json" }
        };

        if (body !== undefined) {
            init.body = JSON.stringify(body);
        }

        const res = await fetch(url, init);

        if (!res.ok) {
            const [error] = await unwrap(res.json() as Promise<RESTError>);
            throw new Error(error?.error ?? `REST request failed: ${res.status} ${res.statusText}`);
        }
    }
}