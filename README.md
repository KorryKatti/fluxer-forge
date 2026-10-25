# Fluxer Forge

Build a [Fluxer](https://fluxer.app) bot without touching a terminal: toggle the
features you want in the browser, download a ZIP, run it. The binary is the same
for everyone — your `config.toml` is what makes it yours.

No accounts, no backend, no hosting. Nothing is uploaded: the config is generated
and the ZIP is assembled entirely in your browser, and the optional bot token
lives in page memory only.

**Fluxer has no slash commands, buttons or modals** — bots there speak prefix
commands and react with emoji. Everything Fluxer can't do is left out of the UI
instead of shipped broken.

## Modules

| Module | What it does |
| --- | --- |
| `welcome` | Greet joins, announce leaves, optional DM |
| `autorole` | Grant roles the moment someone joins |
| `reaction_roles` | Members pick roles by reacting to a message |
| `custom_commands` | Prefix commands that reply with text |
| `automod_lite` | Drop invites, banned words, mention floods |

## What you download

```
fluxer-bot.zip
  bot(.exe)           prebuilt binary (from the release manifest, if published)
  config.toml         generated from your selections
  config.example.toml every module, including the disabled ones
  README.txt          run instructions
  run.sh / run.bat    one-click runners
  Dockerfile          docker build -t my-bot .
  .env                only if you pasted a token (else .env.example)
```

Extract, run `./bot` (or `bot.exe`), done. It works against `fluxer.app` and any
self-hosted Fluxer instance. Keep `dry_run = true` on the first run — the bot
logs what it would do instead of doing it.

## Config

TOML, human-readable, max three levels deep, all IDs as strings:

```toml
schema_version = 1
instance_url   = "https://fluxer.app"

[bot]
prefix  = "!"
dry_run = true

[modules.welcome]
enabled = true

[modules.welcome.on_join]
channel_id = "1501314428688998182"
message    = "Welcome {user} to {guild}!"
```

Template tokens are a fixed set — `{user}`, `{user.name}`, `{user.id}`, `{guild}`,
`{guild.id}`, `{channel}`, `{member.count}` — no scripting, so a shared config is
never executable code. Unknown keys warn and are ignored; validation errors name
the exact path and line.

## Repo layout

```
README.md
frontend/      static site — plain HTML/CSS/JS, zero dependencies, no build step
  index.html
  styles.css
  js/
    modules.js   module definitions + config state
    toml.js      config.toml serializer
    zip.js       release manifest fetch + in-browser ZIP writer
    app.js       wizard rendering + events
runtime/       Go bot — one static binary, no dependencies at runtime
  cmd/bot/                flags, startup sequence, event loop
  internal/config/        config.toml structs, defaults, validation
  internal/fluxer/        discovery, REST client, gateway client
  internal/dispatch/      event router, guild filter, loop guards
  internal/modules/       welcome, autorole, reaction_roles, custom_commands, automod
  internal/log/           stdout + daily JSONL log file
  internal/token/         token from env, .env, token file, or prompt
```

## Running the site

Any static file server works:

```sh
cd frontend
python3 -m http.server 8000
# → http://localhost:8000
```

No npm, no bundler. `index.html` also opens directly from disk.

The site is a three-step wizard — **Connection** → **Modules** → **Review &
download** — with a live `config.toml` preview and a config-only download for
people who already have the binary. Binaries are listed per target
(`linux-amd64`, `linux-arm64`, `windows-amd64`) in an optional `dist/latest.json`
manifest; when none is published yet, the ZIP still ships config, scripts and
Dockerfile with a `BINARY-MISSING.txt` note.

## Status

Frontend is done. The runtime connects, dispatches events, and ships all five
modules: `welcome`, `autorole`, `reaction_roles`, `custom_commands` and
`automod_lite`. Next: `--doctor`, the release pipeline and hosted binaries.

Build it with:

```sh
cd runtime
go build -o bot ./cmd/bot
./bot -config config.toml
```

The token is read from `FLUXER_TOKEN`, then `./.env`, then `./token`, then a
no-echo prompt (saved to `./token` with `0600`).
