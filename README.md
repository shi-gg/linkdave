[![](https://img.shields.io/discord/828676951023550495?color=5865F2&logo=discord&logoColor=white)](https://discord.com/invite/yYd6YKHQZH)
![](https://img.shields.io/github/repo-size/shi-gg/linkdave?maxAge=3600)

[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/I3I6AFVAP)

## About
Linkdave is a golang rewrite of lavalink, aimed at performance, memory efficiency (lavalink @ 393mb-5.4gb vs linkdave @ 3mb with 38 players*), stability and several other things for [Wamellow Text-to-Speech](https://wamellow.com/docs/text-to-speech).

Interoperability in the server and client libraries is an absolute non-goal. Read the [Linkdave TypeScript library documentation](https://npmx.dev/package-docs/linkdave/v/latest) for more details and how to use it. Linkdave is built from the ground up to support [Discord Audio & Video End-to-End Encryption (DAVE)](https://daveprotocol.com/), which is also where the name comes from.

A big difference is that tracks do not need to be resolved first, and therefore are only fetched once at play time without needing another roundtrip.

**You can use the following sources to play audio**
- Remote MP3 files and streams
- Text to Speech (using the [Wamellow TTS API](https://wamellow.com/docs/text-to-speech))
<br />

```ts
const player = linkdave.getPlayer("GUILD_ID"); // <- GET or CREATE player
await player.connect("VOICE_CHANNEL_ID");

await player.play("https://icepool.silvacast.com/GAYFM.mp3"); // 24/7 radio stream
await player.play(constructUri.tts("Hello world", "en_female_samc")); // Text to Speech
```

Tracks can also be queued
```ts
const player = linkdave.getPlayer("GUILD_ID");
await player.connect("VOICE_CHANNEL_ID");

player.queue.add("https://icepool.silvacast.com/GAYFM.mp3");
await player.queue.start();
```
<br />

**You can use the following filters to modify audio**
- Vaporwave
- Nightcore
- Rotation
- Tremolo
- Vibrato
- LowPass
- Customizable Pitch
- Customizable Speed
<br />

```ts
// every track after the current one
player.filters.toggle(Filter.Nightcore)
player.filters.speed = 0.5;
player.filters.pitch = 0.5;

// single track only (works on `player.play` as well)
player.queue.add(
    constructUri.tts(fullText, voice, translate),
    {
        requesterId: message.author.id,
        filters: {
            enabled: [Filter.Vaporwave],
            speed: 0.5,
            pitch: 0.5
        }
    }
);
```

If you need help using this, [join our Discord Server](https://discord.com/invite/yYd6YKHQZH).

## Setup & Usage

Linkdave consists of a Go-based audio server that manages Discord voice connections and playback.

- **WebSocket:** Provides a persistent connection for real-time events.
- **API:** Exposes endpoints to control playback operations.

The server leverages custom audio processing to directly pipeline from an audio source into Discord's UDP socket for low-latency streaming without relying on external bloat.

### Docker Deployment
To deploy Linkdave using Docker, you can use the following `compose.yml` configuration. This setup exposes the WebSocket API on port 8080 and includes health checks to ensure the service is running properly.

`compose.yml`:
```yml
services:
    linkdave:
        image: ghcr.io/shi-gg/linkdave:latest
        container_name: linkdave
        restart: unless-stopped
        ports:
            - "8080:8080"
        logging:
            driver: json-file
            options:
                max-size: "10m"
                max-file: "3"
        environment:
            LINKDAVE_PASSWORD: ${LINKDAVE_PASSWORD}
            LINKDAVE_SOURCE_HTTPS_ENABLED: true
            LINKDAVE_SOURCE_IP_ADDRESS_PUBLIC_ENABLED: true
```

To generate a random password, you can use the following command in your terminal:
```bash
echo "LINKDAVE_PASSWORD=$(openssl rand -hex 16)" >> .env
cat .env
```

To start the server, run:
```bash
docker compose up -d
```

To update the server, simply pull the latest image and restart the container:
```bash
docker pull ghcr.io/shi-gg/linkdave:latest
docker compose restart linkdave
```

### Binary Deployment
Alternatively, you can download the latest binary release from the [GitHub Releases page](https://github.com/shi-gg/linkdave/releases) and run it directly on your server. Linkdave bundles its own DAVE library for [Discord Audio E2EE](https://daveprotocol.com) inside the binary and the Docker image, therefore no `libdave.so` has to be installed.

To start the server, run:
```bash
curl -L -o linkdave https://github.com/shi-gg/linkdave/releases/latest/download/linkdave-linux-amd64
chmod +x linkdave

LINKDAVE_SOURCE_HTTPS_ENABLED=true LINKDAVE_SOURCE_IP_ADDRESS_PUBLIC_ENABLED=true ./linkdave
```

---

The following env variables can be set for the server.

| Variable | Type | Default | Description |
|---|---|---|---|
| `LINKDAVE_SOURCE_HTTP_ENABLED` | bool | `false` | Enable HTTP source checking |
| `LINKDAVE_SOURCE_HTTPS_ENABLED` | bool | `false` | Enable HTTPS source checking |
| `LINKDAVE_SOURCE_IP_ADDRESS_PUBLIC_ENABLED` | bool | `false` | Enable public IP address source |
| `LINKDAVE_SOURCE_IP_ADDRESS_PRIVATE_ENABLED` | bool | `false` | Enable private IP address source |
| `LINKDAVE_SOURCE_TEXT_TO_SPEECH_ENABLED` | bool | `false` | Enable text-to-speech source |
| `LINKDAVE_SOURCE_TEXT_TO_SPEECH_URL` | string | `tts.wamellow.com/api/invoke` | Text-to-speech API endpoint |
| `LINKDAVE_SOURCE_TEXT_TO_SPEECH_TOKEN` | string | — | Authentication token for the TTS API |
| `LINKDAVE_PASSWORD` | string | — | Application password |
| `LINKDAVE_PORT` | string | `8080` | Server port (don't use with docker) |
| `LINKDAVE_LOG_LEVEL` | string | `INFO` | Log level (`DEBUG`, `INFO`, `WARN`, `ERROR`) |

## Using the Client Library (TypeScript)
Linkdave provides a robust, fully type-safe, TypeScript client for seamless interaction.

There is a quick example bot inside the [`example/`](https://github.com/shi-gg/linkdave/blob/main/example/index.ts) directory, and a deploy ready 24/7 radio music bot at [github.com/shi-gg/radio-bot](https://github.com/shi-gg/radio-bot).

```ts
import { Client, GatewayIntentBits, Events } from "discord.js";
import { LinkDaveClient, EventName } from "linkdave";

// `GatewayIntentBits.GuildVoiceStates` is required!
const discord = new Client({ intents: [GatewayIntentBits.GuildVoiceStates] });

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

## Seamless Node Migrations & Graceful Shutdown
Linkdave is built for high availability. When a server node needs to shut down (e.g., for an update or maintenance), it can do so without dropping any active audio streams across your bots using **Graceful Migrations**.

1. **Draining Mode:** Upon receiving a `SIGINT` or `SIGTERM`, the Linkdave server enters "draining" mode. It stops accepting new players and broadcasts a `NodeDraining` message via the WebSocket to all connected clients.
2. **Client Handoff:** Clients intercept this event and search for an available fallback node from the pool.
3. **State Transfer:** The client initiates a `PlayerMigrate` handshake, and transfers the track URL, exact timestamp position, volume, and Discord voice session parameters to the new target node.
4. **Playback Resumes:** Once the new node takes over the voice UDP connection, playback resumes transparently.
5. **Zero Downtime:** The draining server waits until its tracked player count drops to zero (or hits a 30-second hard timeout deadline) before fully powering off, ensuring no track is ever forcefully interrupted.

The gap between closing the UDP connection on the old node and sending the first opus frame from the new node is under 500ms, providing a near-seamless experience for listeners.

<sub>*I'm not sure what's going on with Lavalink 4.2.2, but 38 players use anything from 393mb to 5,300mb in production, with an average of above 1000mb.</sub>