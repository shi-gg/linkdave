import { Client, GatewayIntentBits, Events } from "discord.js";
import { EventName, LinkDaveClient } from "linkdave";

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
    clientId: "1116414956972290119",
    nodes: [{ name: "main", url: "ws://localhost:18080" }],
    sendToShard: (guildId, payload) => {
        discord.guilds.cache.get(guildId)?.shard.send(payload);
    }
});

discord.on(Events.Raw, (packet) => linkdave.handleRaw(packet));

linkdave.on(EventName.Ready, (d) => console.log(`LinkDave session: ${d.session_id}`));
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
            msg.reply("Join a voice channel first!");
            return;
        }

        const player = linkdave.getPlayer(msg.guild.id);
        player.connect(vc.id);

        void msg.reply(`Joining ${vc.name}...`);
    }

    const player = linkdave.players.get(msg.guild.id);
    if (!player) return;

    switch (cmd) {
        case "!play": player.play(args[0]); break;
        case "!pause": player.pause(); break;
        case "!resume": player.resume(); break;
        case "!stop": player.stop(); break;
        case "!leave": player.destroy(); break;
        case "!volume": player.setVolume(parseInt(args[0], 10) * 10); break;
    }
});

process.on("SIGINT", () => {
    linkdave.disconnectAll();
    discord.destroy();
    process.exit(0);
});

discord.login(DISCORD_TOKEN);
