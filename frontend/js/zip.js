// Release manifest loading, ZIP building (stored, no deps) and downloads.

const MANIFEST_URL = 'dist/latest.json'

const TARGETS = ['linux-amd64', 'linux-arm64', 'windows-amd64']

function detectTarget() {
  const ua = navigator.userAgent
  if (/windows|win32|win64/i.test(ua)) return 'windows-amd64'
  if (/aarch64|arm64/i.test(ua) && !/android/i.test(ua)) return 'linux-arm64'
  return 'linux-amd64'
}

async function loadManifest() {
  try {
    const res = await fetch(MANIFEST_URL, { cache: 'no-store' })
    if (!res.ok) return null
    const data = await res.json()
    if (!data || typeof data !== 'object' || !data.targets) return null
    return data
  } catch {
    return null
  }
}

const CRC_TABLE = (() => {
  const table = new Uint32Array(256)
  for (let n = 0; n < 256; n++) {
    let c = n
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    table[n] = c >>> 0
  }
  return table
})()

function crc32(buf) {
  let c = 0xffffffff
  for (let i = 0; i < buf.length; i++) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8)
  return (c ^ 0xffffffff) >>> 0
}

function dosDateTime(date) {
  const time =
    ((date.getHours() & 0x1f) << 11) | ((date.getMinutes() & 0x3f) << 5) | ((date.getSeconds() >> 1) & 0x1f)
  const year = Math.max(1980, date.getFullYear())
  const day = (((year - 1980) & 0x7f) << 9) | ((date.getMonth() + 1) << 5) | (date.getDate() & 0x1f)
  return { time, day }
}

function u16(value) {
  return [value & 0xff, (value >>> 8) & 0xff]
}

function u32(value) {
  return [value & 0xff, (value >>> 8) & 0xff, (value >>> 16) & 0xff, (value >>> 24) & 0xff]
}

// files: [{ name: string, data: Uint8Array }] — stored (no compression)
function zipStore(files) {
  const enc = new TextEncoder()
  const { time, day } = dosDateTime(new Date())
  const chunks = []
  const central = []
  let offset = 0

  for (const file of files) {
    const nameBytes = enc.encode(file.name)
    const data = file.data
    const crc = crc32(data)

    const local = new Uint8Array([
      ...u32(0x04034b50),
      ...u16(20),
      ...u16(0),
      ...u16(0),
      ...u16(time),
      ...u16(day),
      ...u32(crc),
      ...u32(data.length),
      ...u32(data.length),
      ...u16(nameBytes.length),
      ...u16(0),
    ])

    chunks.push(local, nameBytes, data)

    central.push(
      new Uint8Array([
        ...u32(0x02014b50),
        ...u16(20),
        ...u16(20),
        ...u16(0),
        ...u16(0),
        ...u16(time),
        ...u16(day),
        ...u32(crc),
        ...u32(data.length),
        ...u32(data.length),
        ...u16(nameBytes.length),
        ...u16(0),
        ...u16(0),
        ...u16(0),
        ...u16(0),
        ...u32(0),
        ...u32(offset),
      ]),
      nameBytes,
    )

    offset += local.length + nameBytes.length + data.length
  }

  const centralSize = central.reduce((sum, part) => sum + part.length, 0)

  const eocd = new Uint8Array([
    ...u32(0x06054b50),
    ...u16(0),
    ...u16(0),
    ...u16(files.length),
    ...u16(files.length),
    ...u32(centralSize),
    ...u32(offset),
    ...u16(0),
  ])

  const total = offset + centralSize + eocd.length
  const out = new Uint8Array(total)
  let pos = 0
  for (const part of [...chunks, ...central, eocd]) {
    out.set(part, pos)
    pos += part.length
  }
  return out
}

function readmeText(hasToken) {
  return `FLUXER BOT
==========

1. Create a bot application on your Fluxer instance and copy the token
   (it is shown once).
2. Put it in .env as:  FLUXER_TOKEN=<token>
   ${hasToken ? '(already written for you — keep this file private)' : '(.env for this build is empty)'}
3. Run it:
     Linux/macOS   ./bot
     Windows       bot.exe
     Docker        docker build -t my-bot . && docker run --env-file .env my-bot

Config lives in config.toml. Every id is a string, quoted.

Useful flags:
  --config <path>   config file to load (default: config.toml)
  --dry-run         log what would happen, send nothing
  --doctor          check token, instance and permissions, then exit
  --version

Never commit .env. Rotating the token invalidates every running session.
`
}

const DOCKERFILE = `FROM alpine:3.20
WORKDIR /app
COPY config.toml bot ./
RUN chmod +x bot
ENV FLUXER_TOKEN=""
ENTRYPOINT ["/app/bot", "--config", "/app/config.toml"]
`

function runSh(binary) {
  return `#!/bin/sh
set -eu
cd "$(dirname "$0")"
if [ -f .env ]; then set -a; . ./.env; set +a; fi
exec ./${binary} --config config.toml "$@"
`
}

const RUN_BAT = `@echo off
cd /d "%~dp0"
if exist .env (
  for /f "usebackq tokens=1,* delims==" %%a in (".env") do set "%%a=%%b"
)
bot.exe --config config.toml %*
`

async function buildZip(toml, exampleToml, options) {
  const enc = new TextEncoder()
  const files = []
  const names = []
  let binaryIncluded = false

  const add = (name, content) => {
    files.push({ name, data: enc.encode(content) })
    names.push(name)
  }

  add('config.toml', toml)
  add('config.example.toml', exampleToml)
  add('README.txt', readmeText(options.token.trim().length > 0))
  add('run.sh', runSh(options.target === 'windows-amd64' ? 'bot.exe' : 'bot'))
  add('run.bat', RUN_BAT)
  add('Dockerfile', DOCKERFILE)
  add('.gitignore', '.env\nbot\nbot.exe\nlogs/\n')

  if (options.token.trim()) {
    add('.env', `FLUXER_TOKEN=${options.token.trim()}\n`)
  } else {
    add('.env.example', 'FLUXER_TOKEN=\n')
  }

  const target = options.manifest?.targets?.[options.target]
  if (target?.url) {
    try {
      const res = await fetch(target.url)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = new Uint8Array(await res.arrayBuffer())
      const name = options.target === 'windows-amd64' ? 'bot.exe' : 'bot'
      files.push({ name, data })
      names.push(name)
      binaryIncluded = true
    } catch {
      add(
        'BINARY-MISSING.txt',
        `The prebuilt binary for ${options.target} could not be downloaded.\n${target.url}\n\nDownload it manually and place it next to config.toml, or build from source.\n`,
      )
    }
  } else {
    add(
      'BINARY-MISSING.txt',
      `No prebuilt binary is published for this build yet.\n\nYour config.toml is complete and valid. Download the binary for ${options.target}\nfrom the releases page and place it next to config.toml, or build from source.\n`,
    )
  }

  const zipped = zipStore(files)
  const blob = new Blob([zipped], { type: 'application/zip' })
  return { blob, names, binaryIncluded }
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 10_000)
}
