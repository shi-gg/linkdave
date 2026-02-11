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
        if (!vc) return msg.reply("Join a voice channel first!");

        const player = linkdave.getPlayer(msg.guild.id);
        player.connect(vc.id);
        await msg.reply(`Joining ${vc.name}...`);
    }

    if (cmd === "!play" && args[0]) {
        const player = linkdave.getExistingPlayer(msg.guild.id);
        if (!player) return msg.reply("Use !join first");
        player.play(args[0]);
        await msg.reply(`Playing: ${args[0]}`);
    }

    if (cmd === "!pause") linkdave.getExistingPlayer(msg.guild.id)?.pause();
    if (cmd === "!resume") linkdave.getExistingPlayer(msg.guild.id)?.resume();
    if (cmd === "!stop") linkdave.getExistingPlayer(msg.guild.id)?.stop();
    if (cmd === "!leave") linkdave.getExistingPlayer(msg.guild.id)?.destroy();
    if (cmd === "!volume" && args[0]) linkdave.getExistingPlayer(msg.guild.id)?.setVolume(parseInt(args[0], 10) * 10);
});

process.on("SIGINT", () => {
    linkdave.disconnectAll();
    discord.destroy();
    process.exit(0);
});

discord.login(DISCORD_TOKEN);
