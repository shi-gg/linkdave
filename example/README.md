[![](https://img.shields.io/discord/828676951023550495?color=5865F2&logo=discord&logoColor=white)](https://discord.com/invite/yYd6YKHQZH)
![](https://img.shields.io/github/repo-size/shi-gg/linkdave?maxAge=3600)

[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/I3I6AFVAP)

**⚠️ In development, breaking changes ⚠️**

## About
A minimal Discord.js example bot demonstrating how to use the [Typescript Linkdave client library](https://npmx.dev/package-docs/linkdave/v/latest). There is a full deploy ready 24/7 radio music bot at [github.com/shi-gg/radio-bot](https://github.com/shi-gg/radio-bot).

If you need help using this, join **[our Discord Server](https://discord.com/invite/yYd6YKHQZH)**.

## Setup

1. Install dependencies:
```bash
bun install
```

2. Set your Discord bot token:
```bash
echo "DISCORD_TOKEN=your-bot-token" >> .env
```

3. Start LinkDave server:
```bash
docker compose up -d
```

4. Run the bot:
```bash
bun run start
```

## Commands

> [!WARNING]
> Make sure your bot has permission to connect to voice channels and read messages.

| Command | Description |
|---------|-------------|
| `!join` | Join your current voice channel |
| `!play <url>` | Play audio from a URL (MP3) |
| `!tts <...text>` | Generate Text-to-Speech (using [Wamellow TTS](https://wamellow.com/docs/text-to-speech)) |
| `!pause` | Pause playback |
| `!resume` | Resume playback |
| `!stop` | Stop playback |
| `!leave` | Leave voice channel |
| `!get-channel` | Returns the current voice channel |

---

 Try out the following commands:
 `!join`, `!play https://icepool.silvacast.com/GAYFM.mp3`