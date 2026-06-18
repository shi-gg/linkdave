import type { FiltersPayload, Player } from "linkdave";
import { Client, Events, GatewayIntentBits } from "discord.js";
import { constructUri, EventName, Filter, LinkDaveClient } from "linkdave";

const DISCORD_TOKEN = process.env.DISCORD_TOKEN!;
if (!DISCORD_TOKEN) {
    console.error("DISCORD_TOKEN required");
    process.exit(1);
}

function parseFilter(name: string): Filter | null {
    const id = Number(name);
    if (!Number.isNaN(id) && Filter[id] !== undefined) return id;
    const key = Object.keys(Filter).find(k => k.toLowerCase() === name.toLowerCase());
    return key !== undefined ? Filter[key as keyof typeof Filter] : null;
}

function parseFilters(args: string[]): FiltersPayload | undefined {
    const filter = parseFilter(args[0]);
    if (filter === null) return undefined;
    return { enabled: [filter] };
}

const discord = new Client({
    intents: [
        GatewayIntentBits.Guilds,
        GatewayIntentBits.GuildMessages,
        GatewayIntentBits.GuildVoiceStates,
        GatewayIntentBits.MessageContent
    ]
});

const linkdave = new LinkDaveClient({
    token: DISCORD_TOKEN,
    nodes: [
        { name: "main", url: "ws://localhost:18080", password: process.env.LINKDAVE_PASSWORD }
    ],
    sendToShard: (guildId, payload) => {
        discord.guilds.cache.get(guildId)?.shard.send(payload);
    }
});

discord.on(Events.Raw, (packet) => linkdave.handleRaw(packet));

linkdave.on(EventName.Ready, (d) => console.log(`LinkDave session: ${d.session_id}`));
linkdave.on(EventName.PlayerUpdate, (d) => console.log(`Update: ${d.state}`));
linkdave.on(EventName.TrackStart, (d) => console.log(`Playing: ${d.track.url}`));
linkdave.on(EventName.TrackEnd, (d) => console.log(`Track ended: ${d.reason}`));
linkdave.on(EventName.TrackError, (d) => console.error(`Error: ${d.error}`));
linkdave.on(EventName.VoiceConnect, (d) => console.log(`Voice connected: ${d.channel_id}`));
linkdave.on(EventName.VoiceDisconnect, (d) => console.log(`Voice connected: ${d.guild_id} reason: ${d.reason}`));
linkdave.on(EventName.Close, (d) => console.log(`Connection closed: ${d.code} ${d.reason}`));
linkdave.on(EventName.Error, console.error);

discord.on(Events.ClientReady, async () => {
    console.log(`Bot ready as ${discord.user?.tag}`);
    await linkdave.connectAll();
});

discord.on(Events.MessageCreate, async (msg) => {
    if (msg.author.bot || !msg.guild) return;
    const [cmd, ...args] = msg.content.split(" ");

    if (cmd === "!join") {
        const vc = (await msg.member?.fetch())?.voice.channel;
        if (!vc) {
            void msg.reply("Join a voice channel first!");
            return;
        }

        const player = linkdave.getPlayer(msg.guild.id);
        await player.connect(vc.id);

        void msg.reply(`Joining ${vc.name}...`);
    }

    const player = linkdave.players.get(msg.guild.id);
    if (!player) return;

    switch (cmd) {
        case "!play":
            player.queue.add(args[0]);
            if (!player.playing) await player.queue.start();
            break;
        case "!play-filter": {
            const filters = parseFilters(args);
            if (!filters) {
                void msg.reply("Unknown filter. Try: Nightcore, Vaporwave, Tremolo, Vibrato, Rotation, LowPass");
                return;
            }
            if (!args[1]) {
                void msg.reply("Usage: !play-filter <filter> <url>");
                return;
            }
            player.queue.add(args[1], { filters });
            if (!player.playing) await player.queue.start();
            break;
        }
        case "!play-now": {
            const filters = parseFilters(args);
            const url = filters ? args[1] : args[0];
            if (!url) {
                void msg.reply("Usage: !play-now [filter] <url>");
                return;
            }
            await player.play(url, filters ? { filters } : {});
            break;
        }
        case "!tts":
            player.queue.add(constructUri.tts(args.join(" "), "en_us_001"));
            if (!player.playing) await player.queue.start();
            break;
        case "!skip": await player.queue.skip(); break;
        case "!clear": {
            player.queue.clear();
            void msg.reply("Queue cleared");
            break;
        }
        case "!pause": await player.pause(); break;
        case "!resume": await player.resume(); break;
        case "!stop": await player.stop(); break;
        case "!leave": await player.destroy(); break;
        case "!get-channel": await msg.reply(`Current channel: <#${player.voiceChannelId}>`); break;
        case "!current-track": await msg.reply(`Current track: ${player.current?.url}`); break;
        case "!set-filter": {
            const filter = parseFilter(args[0]);
            if (filter === null) {
                void msg.reply("Unknown filter. Try: Nightcore, Vaporwave, Tremolo, Vibrato, Rotation, LowPass");
                return;
            }
            player.filters.toggle(filter, true);
            void msg.reply(`Active filters: ${player.filters.activeFilters.map(f => Filter[f]).join(", ")}`);
            break;
        }
        case "!clear-filters": {
            player.filters.clear();
            break;
        }
        case "!status": {
            msg.reply(`\`\`\`${JSON.stringify(playerToJson(player))}\`\`\``)
            break;
        }
        case "!kill": linkdave.removeNode(player.node.name); break;
    }
});

function playerToJson(player: Player) {
    return {
        connected: player.connected,
        playing: player.playing,
        paused: player.paused,
        state: player.state,
        current: player.current,
        filters: {
            activeFilters: player.filters.activeFilters,
            pitch: player.filters.pitch,
            speed: player.filters.speed
        },
        queue: {
            size: player.queue.size,
            active: player.queue.active,
            next: player.queue.tracks[0]
        }
    };
}

process.on("SIGINT", () => {
    linkdave.disconnectAll();
    discord.destroy();
    process.exit(0);
});

process.on("uncaughtException", console.error);
process.on("unhandledRejection", console.error);

discord.login(DISCORD_TOKEN);