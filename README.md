[![](https://img.shields.io/discord/828676951023550495?color=5865F2&logo=discord&logoColor=white)](https://discord.com/invite/yYd6YKHQZH)
![](https://img.shields.io/github/repo-size/shi-gg/linkdave?maxAge=3600)

[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/I3I6AFVAP)

**⚠️ In development, breaking changes ⚠️**

## About
Linkdave is a golang rewrite of lavalink, aimed at performance, memory efficiency (193mib vs 12mib), stability and several other things for [Wamellow](https://wamellow.com/docs/text-to-speech).

Interoperability in the server and client libraries is an absolute non-goal. Read the [Linkdave TypeScript library documentation](https://npmx.dev/package-docs/linkdave/v/latest) for more details and how to use it. Linkdave is built from the ground up to support [Discord Audio & Video End-to-End Encryption (DAVE)](https://daveprotocol.com/), which is also where the name comes from.

The following sources are currently supported:
- Remote MP3 files and streams
- Text to Speech (using the [Wamellow TTS API](https://wamellow.com/docs/text-to-speech))

If you need help using this, [join our Discord Server](https://discord.com/invite/yYd6YKHQZH).

## Setup & Usage

### ⚙️ How the Server Works
Linkdave consists of a Go-based audio server that manages Discord voice connections and playback.

- **WebSocket (`/ws`):** Provides a persistent connection for real-time events. The server dispatches track updates (start, end, error), voice connection states, player updates, and node statistics.
- **REST API:** Exposes endpoints to control playback operations such as Play, Pause, Resume, Stop, ~~Seek, and Volume~~ adjustments.

The server leverages custom audio processing to directly pipeline from an audio source into Discord's UDP socket for low-latency streaming without relying on external bloat.

### 💻 Using the Client Library (TypeScript)
Linkdave provides a robust, fully type-safe, TypeScript client for seamless interaction.

There is a quick example bot inside the [`example/`](https://github.com/shi-gg/linkdave/blob/main/example/index.ts) directory, and a deploy ready 24/7 radio music bot at [github.com/shi-gg/radio-bot](https://github.com/shi-gg/radio-bot).

```ts
import { Client, GatewayIntentBits, Events } from "discord.js";
import { LinkDaveClient, EventName } from "linkdave";

const discord = new Client({ intents: [GatewayIntentBits.Guilds, GatewayIntentBits.GuildVoiceStates] });

const linkdave = new LinkDaveClient({
    token: process.env.DISCORD_TOKEN,
    nodes: [
        { name: "main", url: "ws://localhost:8080" },
        { name: "backup", url: "ws://localhost:8081" }
    ],
    sendToShard: (guildId, payload) => {
        discord.guilds.cache.get(guildId)?.shard.send(payload);
    }
});

// Intercept raw voice packets from Discord and forward to LinkDave
discord.on(Events.Raw, (packet) => linkdave.handleRaw(packet));

// Connect to all configured nodes
discord.on(Events.ClientReady, async () => {
    await linkdave.connectAll();
    console.log("Ready!");
});

linkdave.on(EventName.TrackStart, ({ track }) => console.log(`Playing: ${track.url}`));

// Playing a track
const player = linkdave.getPlayer("GUILD_ID");
await player.connect("VOICE_CHANNEL_ID");
await player.play("https://icepool.silvacast.com/GAYFM.mp3");

discord.login(process.env.DISCORD_TOKEN);
```

### 🔄 Seamless Node Migrations & Graceful Shutdown
Linkdave is built for high availability. When a server node needs to shut down (e.g., for an update or maintenance), it can do so without dropping any active audio streams across your bots using **Graceful Migrations**.

1. **Draining Mode:** Upon receiving a `SIGINT` or `SIGTERM`, the Linkdave server enters "draining" mode. It stops accepting new players and broadcasts a `NodeDraining` message via the WebSocket to all connected clients.
2. **Client Handoff:** Clients intercept this event and search for an available fallback node from the pool.
3. **State Transfer:** The client initiates a `PlayerMigrate` handshake, and transfers the track URL, exact timestamp position, volume, and Discord voice session parameters to the new target node.
4. **Playback Resumes:** Once the new node takes over the voice UDP connection, playback resumes transparently.
5. **Zero Downtime:** The draining server waits until its tracked player count drops to zero (or hits a 30-second hard timeout deadline) before fully powering off, ensuring no track is ever forcefully interrupted.

The gap between closing the UDP connection on the old node and sending the first opus frame from the new node is typically under 500ms, providing a near-seamless experience for listeners.

One test migration log from a local test run:
- `13:05:00.785138722`: voice gateway close received;
- `13:05:01.241348505`: sending voice gateway command;