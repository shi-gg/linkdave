import type { RESTError } from "./types.js";
import { unwrap } from "./utils.js";

export class RESTClient {
    readonly #baseUrl: string;
    readonly #password: string | undefined;

    constructor(
        wsUrl: string,
        password?: string
    ) {
        const url = new URL(wsUrl);
        url.protocol = url.protocol === "wss:" ? "https:" : "http:";

        this.#baseUrl = url.origin;
        this.#password = password;
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
        const headers: Record<string, string> = {
            "Content-Type": "application/json"
        };

        if (this.#password) {
            headers.Authorization = `Bearer ${this.#password}`;
        }

        const init: RequestInit = {
            method,
            headers
        };

        if (body !== undefined) {
            init.body = JSON.stringify(body);
        }

        const url = `${this.#baseUrl}${route}`;
        const res = await fetch(url, init);

        if (!res.ok) {
            const [error] = await unwrap(res.json() as Promise<RESTError>);
            throw new Error(error?.error ?? `REST request failed: ${res.status} ${res.statusText}`);
        }
    }
}