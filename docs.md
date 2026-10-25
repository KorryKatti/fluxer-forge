# Fluxer Forge documentation

Technical details for the builder, the generated bot, and the release pipeline.
For the quick start and common problems, see the [README](README.md).

- **Builder:** <https://korrykatti.github.io/fluxer-forge/>
- **Source:** <https://github.com/KorryKatti/fluxer-forge>
- **Community:** <https://fluxer.gg/s1evrala>

## What you download

```
fluxer-bot.zip
  bot(.exe)            prebuilt binary for your platform
  config.toml          generated from your selections
  config.example.toml  every module, including disabled ones
  README.txt           run instructions
  run.sh / run.bat     one-click runners
  Dockerfile           docker build -t my-bot .
  .env                 only if you pasted a token (otherwise .env.example)
  logs/                created on first run
```

The binary is the same for everyone; `config.toml` makes it yours. Downloads
work without a published binary too — the ZIP then carries the config, runners,
Dockerfile, and a `BINARY-MISSING.txt` note.

## Configuration

The config is TOML, at most three levels deep, with every ID stored as a string.

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

A complete starter lives in [`config.example.toml`](config.example.toml): welcome
on, every other module present but off, and `dry_run = true`. The test suite
parses it, so it always stays valid. Copy it to `config.toml` and replace the
placeholder channel ID.

Unknown keys warn and are ignored. Validation errors name the exact path and
line. The browser mirrors the Go checks; errors block download, warnings do not.

Template tokens are a fixed set — `{user}`, `{user.name}`, `{user.id}`, `{guild}`,
`{guild.id}`, `{channel}`, `{member.count}`. There is no scripting, so a shared
config is never executable code.

## Command line

```sh
cd runtime
go build -o bot ./cmd/bot

./bot --config config.toml
./bot --dry-run --config config.toml
./bot --doctor --config config.toml
./bot --version
./bot --print-schema > ../schema/schema.json
```

| Flag | Purpose |
| --- | --- |
| `--config <path>` | Config file to load (default `config.toml`) |
| `--dry-run` | Force dry-run, whatever the config says |
| `--doctor` | Check instance, token, gateway and channels, then exit |
| `--version` | Print version and supported schema version |
| `--print-schema` | Emit the config schema as JSON |

The token is never accepted as a flag. It is read from `FLUXER_TOKEN`, then
`./.env`, then `./token`, then a no-echo prompt (saved to `./token` with `0600`).

## Running the site

Any static file server works; there is no build step and no dependencies.

```sh
cd frontend
python3 -m http.server 8000
# → http://localhost:8000
```

`index.html` also opens directly from disk. The wizard is three steps —
**Connection** → **Modules** → **Review & download** — with a live TOML preview.

Binaries come from an optional `dist/latest.json` manifest, fetched from the
`dist` branch first and the local `dist/` directory second. A manifest built for
a different schema is refused, and each binary is checked against its SHA-256
and size before it enters the archive.

The invite URL pre-fills the union of enabled modules' permission bits. It is
built as `<web>/oauth2/authorize?client_id=<id>&scope=bot&permissions=<mask>`,
with the client ID taken from the bot token's `<application_id>.<secret>` form.
The token stays in page memory.

## Repo layout

```
README.md
docs.md
LICENSE
config.example.toml   valid starter config, checked by the test suite
frontend/             static site — plain HTML/CSS/JS, zero dependencies
  index.html
  styles.css
  js/
    modules.js        module definitions + config state
    validate.js       in-browser checks mirroring the Go validator
    toml.js           config.toml serializer
    zip.js            release manifest fetch + in-browser ZIP writer
    app.js            wizard rendering + events
schema/               generated schema.json + frontend drift check
.github/              ci, release and Pages workflows
runtime/              Go bot — one static binary, no runtime dependencies
  cmd/bot/            flags, startup sequence, event loop
  internal/config/    config structs, defaults, validation
  internal/fluxer/    discovery, REST client, gateway client
  internal/dispatch/  event router, guild filter, loop guards
  internal/modules/   welcome, autorole, reaction_roles, custom_commands, automod
  internal/log/       stdout + daily JSONL log file
  internal/token/     token from env, .env, token file, or prompt
  internal/doctor/    --doctor preflight checks
  internal/schema/    schema.json generation
```

## Releases and schema

- `schema/schema.json` is generated from the Go structs. CI compares it against
  the binary output and checks the hand-written frontend forms for drift.
- Pushing a `v*` tag runs tests, builds `linux-amd64`, `linux-arm64` and
  `windows-amd64`, writes `latest.json` with hashes, publishes the `dist` branch,
  and creates a GitHub release.
- Pushes to `main` that touch `frontend/` deploy the site to GitHub Pages.
- `dist/latest.json` records the version, schema version, and each target's URL,
  SHA-256, and size.

## Development checks

```sh
cd runtime
go test ./...
go vet ./...

go run ./cmd/bot -print-schema > /tmp/schema.json
diff -u ../schema/schema.json /tmp/schema.json

for file in ../frontend/js/*.js; do node --check "$file"; done
node ../schema/check-frontend.mjs
```

The runtime connects, dispatches events, reconnects with backoff and resumes the
last gateway session. All five modules ship. Still open: zstd, update checks and
generated docs.

## License

[MIT](LICENSE)
