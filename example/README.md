[![](https://img.shields.io/discord/828676951023550495?color=5865F2&logo=discord&logoColor=white)](https://discord.com/invite/yYd6YKHQZH)
![](https://img.shields.io/github/repo-size/shi-gg/linkdave?maxAge=3600)

[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/I3I6AFVAP)

**⚠️ In development, breaking changes ⚠️**

## About
A minimal Discord.js example bot demonstrating how to use the linkdave client library.

If you need help using this, join **[our Discord Server](https://discord.com/invite/yYd6YKHQZH)**.

## Setup

1. Install dependencies:
```bash
bun install
```

2. Build the client library:
```bash
cd ../client
bun run build
cd ../example
```

3. Set your Discord bot token:
```bash
echo "DISCORD_TOKEN=your-bot-token" >> .env
```

4. Start LinkDave server:
```bash
cd .. && docker compose up -d
```

5. Run the bot:
```bash
bun run start
```

## Commands

| Command | Description |
|---------|-------------|
| `!join` | Join your current voice channel |
| `!play <url>` | Play audio from a URL (MP3) |
| `!pause` | Pause playback |
| `!resume` | Resume playback |
| `!stop` | Stop playback |
| `!volume <0-100>` | Set volume |
| `!leave` | Leave voice channel |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DISCORD_TOKEN` | (required) | Discord bot token |
| `LINKDAVE_URL` | `ws://localhost:8080` | LinkDave WebSocket URL |

## Notes

- The bot requires the following Discord intents:
  - Guilds
  - GuildMessages
  - GuildVoiceStates
  - MessageContent (privileged)

- Make sure your bot has permission to connect to voice channels and read messages.
