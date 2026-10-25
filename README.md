# Fluxer Forge

[![Join the Fluxer community](https://img.shields.io/badge/Fluxer-Join%20the%20community-4641D9)](https://fluxer.gg/s1evrala)

Build a [Fluxer](https://fluxer.app) bot without writing code. Pick the features
you want in the browser, download a ready-to-run bot, and host it yourself.

**[Open the builder](https://korrykatti.github.io/fluxer-forge/)** ·
[Docs](docs.md)

No account, no backend, no upload. Your config is generated in the browser, and
the optional bot token never leaves the page.

## How it works

1. Open the builder and choose your Fluxer server.
2. Pick the modules you want and fill in the few IDs Fluxer shows in the app.
3. Download the ZIP, extract it, and start the bot.
4. Use the generated invite link to add the bot to your server.

Start with `dry_run = true` if you want to see what the bot would do before it
posts anything.

## Modules

| Module | What it does |
| --- | --- |
| `welcome` | Greet new members, announce leaves, optional DM |
| `autorole` | Give roles automatically when someone joins |
| `reaction_roles` | Members pick roles by reacting to a message |
| `custom_commands` | Prefix commands that reply with text |
| `automod_lite` | Block invites, banned words, and mention floods |

Fluxer bots use prefix commands and emoji reactions, not slash commands.

## Running the bot

The ZIP includes the bot, a starter config, `.env`, and one-click runners:

- **Linux/macOS:** `./run.sh`
- **Windows:** `run.bat`
- **Docker:** `docker build -t my-bot . && docker run --env-file .env my-bot`

Keep `.env` private. Never commit it or share your bot token.

## Troubleshooting

- **Nothing happens:** check `dry_run` is `false`, the module is enabled, and the
  bot is in your server. In dry-run mode it only logs actions.
- **`MISSING_PERMISSIONS` when giving a role:** move the bot's role above the
  roles it should manage. Admin does not bypass Fluxer's role hierarchy.
- **`MISSING_PERMISSIONS` in a channel:** give the bot **View Channel** and
  **Send Messages** there.
- **Token check fails:** copy the token again into `FLUXER_TOKEN` or `.env`.
  Tokens are shown only once.

Still stuck? Ask in the [community server](https://fluxer.gg/s1evrala).

## License

[MIT](LICENSE)
