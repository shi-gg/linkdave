import { Client, Events, GatewayIntentBits } from "discord.js";
import { constructUri, EventName, LinkDaveClient } from "linkdave";

const DISCORD_TOKEN = process.env.DISCORD_TOKEN!;
if (!DISCORD_TOKEN) {
    console.error("DISCORD_TOKEN required");
    process.exit(1);
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
        case "!volume": await player.setVolume(parseInt(args[0], 10) * 10); break;
        case "!get-channel": await msg.reply(`Current channel: <#${player.voiceChannelId}>`); break;
    }
});

process.on("SIGINT", () => {
    linkdave.disconnectAll();
    discord.destroy();
    process.exit(0);
});

process.on("uncaughtException", console.error);
process.on("unhandledRejection", console.error);

discord.login(DISCORD_TOKEN);