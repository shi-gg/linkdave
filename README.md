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

- **WebSocket (`/ws`):** Provides a persistent connection for real-time events.
- **REST API:** Exposes endpoints to control playback operations.

The server leverages custom audio processing to directly pipeline from an audio source into Discord's UDP socket for low-latency streaming without relying on external bloat.

#### Docker Deployment
To deploy Linkdave using Docker, you can use the following `compose.yml` configuration. This setup exposes the WebSocket API on port 8080 and includes health checks to ensure the service is running properly.

`compose.yml`:
```yml
services:
    linkdave:
        image: ghcr.io/shi-gg/linkdave:latest
        container_name: radio-linkdave
        restart: unless-stopped
        ports:
            - "8080:8080"
        healthcheck:
            test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
            interval: 30s
            timeout: 3s
            start_period: 5s
            retries: 3
        logging:
            driver: json-file
            options:
                max-size: "10m"
                max-file: "3"
        environment:
            LINKDAVE_SOURCE_HTTPS_ENABLED: true
            LINKDAVE_SOURCE_IP_ADDRESS_PUBLIC_ENABLED: true
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

#### Binary Deployment
Alternatively, you can download the latest binary release from the [GitHub Releases page](https://github.com/shi-gg/linkdave/releases) and run it directly on your server.

> [!CAUTION]
> Linkdave relies on a C library, `libdave.so` from [discord/libdave](https://github.com/discord/libdave), for [Discord Audio E2EE](https://daveprotocol.com). This library is included in the Docker image, but not the binary.

When using the standalone `linkdave-linux-amd64` binary, the system needs to know where to find the `libdave.so` shared library. Without it, the application will fail to launch, and running `ldd linkdave-linux-amd64` will report `libdave.so => not found`.

To download the library, go to the [libdave releases page](https://github.com/discord/libdave/releases) and download the `libdave-Linux-X64-boringssl.zip` asset. Extract the `lib/libdave.so` file and follow one of the installation methods below.

<details>

<summary>Installing the `libdave.so` library</summary>

Choose one of the methods below based on your OS and whether you want a system-wide or portable installation.

##### Method 1: System-Wide Installation (Recommended)
Installing the library system-wide allows you to run `linkdave-linux-amd64` from anywhere without needing to set environment variables.

###### Ubuntu / Debian
On Ubuntu and Debian systems, the standard directory for local 64-bit shared libraries is `/usr/local/lib`.

```bash
# 1. Copy the library to the standard path
sudo cp libdave.so /usr/local/lib/

# 2. Update the dynamic linker cache
sudo ldconfig

# 3. Make the binary executable
chmod +x ./linkdave-linux-amd64

# 4. Verify the library is found (it should map to /usr/local/lib/libdave.so)
ldd ./linkdave-linux-amd64 | grep libdave
```

###### RHEL / Fedora / CentOS
On Red Hat-based systems, 64-bit libraries belong in `/usr/local/lib64`.
Depending on your system configuration, you might also need to explicitly add this directory to the linker's search path.

```bash
# 1. Create the directory (if it doesn't exist) and copy the library
sudo mkdir -p /usr/local/lib64
sudo cp libdave.so /usr/local/lib64/

# 2. Ensure the directory is in the linker's search path
echo "/usr/local/lib64" | sudo tee /etc/ld.so.conf.d/local64.conf

# 3. Update the dynamic linker cache
sudo ldconfig

# 4. Make the binary executable
chmod +x ./linkdave-linux-amd64

# 5. Verify the library is found (it should map to /usr/local/lib64/libdave.so)
ldd ./linkdave-linux-amd64 | grep libdave
```

---

##### Method 2: Portable Setup (No Root Required)
If you do not have `sudo` access or prefer to keep the binary and library together in a self-contained folder, you can run the binary by explicitly passing the library path at runtime.

Ensure `libdave.so` and `linkdave-linux-amd64` are in the same folder.

```bash
# 1. Make the binary executable
chmod +x ./linkdave-linux-amd64

# 2. Run the binary with LD_LIBRARY_PATH set to the current directory
LD_LIBRARY_PATH=. ./linkdave-linux-amd64
```

---

##### Run the binary

```bash
LINKDAVE_SOURCE_HTTPS_ENABLED=true LINKDAVE_SOURCE_IP_ADDRESS_PUBLIC_ENABLED=true ./linkdave-linux-amd64
```

</details>

### 💻 Using the Client Library (TypeScript)
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

### 🔄 Seamless Node Migrations & Graceful Shutdown
Linkdave is built for high availability. When a server node needs to shut down (e.g., for an update or maintenance), it can do so without dropping any active audio streams across your bots using **Graceful Migrations**.

1. **Draining Mode:** Upon receiving a `SIGINT` or `SIGTERM`, the Linkdave server enters "draining" mode. It stops accepting new players and broadcasts a `NodeDraining` message via the WebSocket to all connected clients.
2. **Client Handoff:** Clients intercept this event and search for an available fallback node from the pool.
3. **State Transfer:** The client initiates a `PlayerMigrate` handshake, and transfers the track URL, exact timestamp position, volume, and Discord voice session parameters to the new target node.
4. **Playback Resumes:** Once the new node takes over the voice UDP connection, playback resumes transparently.
5. **Zero Downtime:** The draining server waits until its tracked player count drops to zero (or hits a 30-second hard timeout deadline) before fully powering off, ensuring no track is ever forcefully interrupted.

The gap between closing the UDP connection on the old node and sending the first opus frame from the new node is under 500ms, providing a near-seamless experience for listeners.