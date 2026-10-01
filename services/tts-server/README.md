# tts-server

[![Go Version](https://img.shields.io/badge/Go-1.27.1+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![OpenAPI](https://img.shields.io/badge/OpenAPI-3.1-6BA539?style=flat&logo=openapiinitiative)](internal/httpapi/openapi.yaml)
[![Engines](https://img.shields.io/badge/engines-espeak--ng%20%7C%20Piper-blueviolet?style=flat)](#engines)
[![Coverage](https://img.shields.io/badge/coverage-98.7%25-brightgreen?style=flat)](#development)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](../../LICENSE)

A small text-to-speech HTTP API you run yourself. It turns text into MP3 or
WAV with [espeak-ng](https://github.com/espeak-ng/espeak-ng) (100+ languages,
robotic but always there) and [Piper](https://github.com/rhasspy/piper)
neural voices (natural-sounding, one model per voice).

ssg uses it to generate an MP3 for each blog post at build time. Its request
format is the contract of ssg's `generic` TTS provider, so anything that
speaks the same contract can stand in for it.

It lives in the ssg repository as a separate Go module
(`github.com/spagu/ssg/services/tts-server`), so the root module's `./...`
never builds or tests it.

## Quick start

```sh
cd services/tts-server
cp .env.example .env        # set TTS_API_KEYS and TTS_ADMIN_KEYS (openssl rand -hex 32)
docker compose up -d --build
curl -s http://127.0.0.1:8080/readyz
```

`.env.example` sets `WITH_VOICES=en_US-lessac-medium,pl_PL-gosia-medium`, so
the image ships one Piper voice for English and one for Polish. They are
copied into the `tts-voices` volume when the volume is first created. Leave
`WITH_VOICES` empty for an espeak-only image (about 270 MB instead of 510 MB).

From the repository root you can use `make tts-server-up`,
`make tts-server-down`, `make tts-server-build` and `make tts-server-test`.

Interactive docs: <http://127.0.0.1:8080/docs> (Swagger UI). The raw spec is
at `/openapi.yaml`.

### Speech

```sh
export TTS_API_KEY=...   # one of TTS_API_KEYS

# English, default voice for the language
curl -s -H "Authorization: Bearer $TTS_API_KEY" \
  -d '{"text":"Most websites are documents.","lang":"en"}' \
  -o hello.mp3 http://127.0.0.1:8080/v1/speech

# Polish, explicit voice, 10% faster, WAV
curl -s -H "Authorization: Bearer $TTS_API_KEY" \
  -d '{"text":"Dzień dobry.","voice":"pl_PL-gosia-medium","speed":1.1,"format":"wav"}' \
  -o dzien-dobry.wav http://127.0.0.1:8080/v1/speech
```

### Voices

```sh
curl -s -H "Authorization: Bearer $TTS_API_KEY" "http://127.0.0.1:8080/v1/voices?lang=pl"
```

```json
{"voices":[
  {"id":"pl_PL-gosia-medium","engine":"piper","lang":"pl-pl","name":"gosia","quality":"medium",
   "sampleRate":22050,"default":true,"defaults":{"speed":1,"lengthScale":1}},
  {"id":"espeak:pl","engine":"espeak","lang":"pl","name":"Polish","gender":"male","quality":"low",
   "sampleRate":22050,"default":false,"defaults":{"speed":1}}],
 "count":2}
```

Filters: `lang` (`pl` matches `pl` and `pl-pl`) and `engine` (`piper`, `espeak`).

### Adding a voice

Pick a voice from [rhasspy/piper-voices](https://huggingface.co/rhasspy/piper-voices)
(samples: <https://rhasspy.github.io/piper-samples/>), download its `.onnx` and
`.onnx.json`, then either drop both into the voices volume and restart, or
upload them with an admin key:

```sh
curl -s -H "Authorization: Bearer $TTS_ADMIN_KEY" \
  -F model=@de_DE-thorsten-medium.onnx \
  -F config=@de_DE-thorsten-medium.onnx.json \
  http://127.0.0.1:8080/v1/voices                    # 201, id = file name without .onnx

curl -s -X DELETE -H "Authorization: Bearer $TTS_ADMIN_KEY" \
  http://127.0.0.1:8080/v1/voices/de_DE-thorsten-medium   # 204
```

An optional `id` form field overrides the id. Only Piper voices stored as
`<id>.onnx` + `<id>.onnx.json` can be deleted over the API; espeak voices and
voices declared in `voices.yaml` with custom paths cannot.

To bake more voices into the image, add their two checksums to
[`voices.sha256`](voices.sha256) and list the ids in `WITH_VOICES`. The build
downloads from a pinned Hugging Face revision and fails on any checksum
mismatch.

## API contract

| Method & path | Auth | Purpose |
|---|---|---|
| `POST /v1/speech` | API key | Synthesize speech |
| `GET /v1/voices` | API key | List voices |
| `POST /v1/voices` | admin key | Upload a Piper voice (multipart: `model`, `config`, optional `id`) |
| `DELETE /v1/voices/{id}` | admin key | Delete a Piper voice |
| `GET /healthz` | none | Liveness |
| `GET /readyz` | none | Readiness: espeak-ng and lame present, at least one voice |
| `GET /openapi.yaml`, `GET /docs` | none | OpenAPI 3.1 spec and Swagger UI |

`POST /v1/speech` body (unknown fields are rejected):

| Field | Type | Default | Notes |
|---|---|---|---|
| `text` | string | (required) | Plain UTF-8, at most `TTS_MAX_CHARS` characters |
| `voice` | string | | Voice id from `/v1/voices`; wins over `lang` |
| `lang` | string | `TTS_DEFAULT_LANG` | BCP-47 (`en`, `pl`, `en-GB`); picks that language's default voice |
| `speed` | number | `1.0` | 0.25 to 4.0, multiplies the voice's own default speed |
| `format` | string | `mp3` | `mp3` or `wav` |

Success is `200` with `Content-Type: audio/mpeg` (or `audio/wav`) and the audio
as the body, plus:

- `ETag`: a hash of voice, voice revision, language, speed, format and text.
  Send it back in `If-None-Match` and you get `304 Not Modified` without any
  synthesis. ssg can use this to skip unchanged posts.
- `X-TTS-Voice`: the voice that spoke. `X-TTS-Cache`: `hit` or `miss`.

Errors are JSON `{"error": "...", "code": "..."}`:

| Status | `code` | When |
|---|---|---|
| 400 | `bad_request`, `text_too_long`, `unknown_voice`, `unknown_lang` | Invalid input |
| 401 | `unauthorized` | Missing or wrong bearer key |
| 403 | `forbidden` | Voice management with no `TTS_ADMIN_KEYS` configured |
| 404 | `not_found` | No such voice or endpoint |
| 409 | `conflict` | Uploading an id that already exists |
| 413 | `payload_too_large` | Body over the limit |
| 429 | `rate_limited` | Token bucket empty; see `Retry-After` |
| 503 | `busy`, `not_ready` | All engine slots and the queue are full (`Retry-After: 5`), or the engine is missing |
| 500 | `engine_failure` | The engine or the encoder failed |
| 504 | `timeout` | Synthesis took longer than `TTS_TIMEOUT` |

Clients should retry 429, 503 and 504 after `Retry-After`. They should not
retry 400.

### How a voice is chosen

1. `voice` given: that voice, or `400 unknown_voice`.
2. Otherwise `lang` (or `TTS_DEFAULT_LANG`): a `defaults:` entry in
   `voices.yaml` for the exact tag, then for its primary subtag (`en-GB` → `en`).
3. Otherwise the best match: any Piper voice before any espeak voice, an exact
   language before a related one, and espeak's own priorities among related
   ones (`en` → `espeak:en-gb` rather than `espeak:en-029`).

## Using it from ssg

```yaml
tts:
  provider: generic
  api_url: http://localhost:8080/v1/speech
  api_key: $TTS_API_KEY
  voice: en_US-lessac-medium   # optional; omit to choose by lang
  lang: en
```

## Configuration

Every variable is optional. [`.env.example`](.env.example) lists all of them.

| Variable | Default | Meaning |
|---|---|---|
| `TTS_ADDR` | `:8080` | Listen address |
| `TTS_API_KEYS` | (empty) | Comma-separated bearer keys for `/v1`. Empty disables auth and logs a warning |
| `TTS_ADMIN_KEYS` | (empty) | Keys for voice upload and delete. Empty disables those endpoints (403). Admin keys also work as API keys |
| `TTS_MAX_CHARS` | `20000` | Longest accepted text |
| `TTS_MAX_CONCURRENCY` | CPU count | Syntheses running at once |
| `TTS_QUEUE_SIZE` | CPU count | Requests allowed to wait for a slot; beyond that, 503 |
| `TTS_QUEUE_TIMEOUT` | `10s` | Longest wait for a slot before 503 |
| `TTS_TIMEOUT` | `60s` | Per-request synthesis deadline |
| `TTS_RATE_LIMIT` | `60` | Requests per minute per key (per client IP without auth); `0` disables |
| `TTS_RATE_BURST` | `10` | Token-bucket size |
| `TTS_MAX_UPLOAD_MB` | `256` | Largest voice model upload |
| `TTS_DEFAULT_LANG` | `en` | Language when a request names neither voice nor lang |
| `TTS_VOICES_DIR` | `/voices` | Piper models and `voices.yaml` |
| `TTS_CACHE_DIR` | (empty; image: `/cache`) | On-disk cache. Empty keeps the cache in memory |
| `TTS_CACHE_MB` | `128` | Cache budget, least recently used evicted first; `0` disables |
| `TTS_ESPEAK_BIN` | `espeak-ng` | espeak-ng binary |
| `TTS_PIPER_BIN` | `piper` (image: `/opt/piper/piper`) | Piper binary |
| `TTS_LAME_BIN` | `lame` | MP3 encoder |
| `TTS_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

docker compose also reads `TTS_PORT` (host port, bound to 127.0.0.1),
`TTS_CPUS` (CPU limit) and `WITH_VOICES` (build arg).

### voices.yaml

Optional, in the voices directory. Unknown keys fail startup, so a typo can't
fall back to defaults without anyone noticing. See
[`voices.example.yaml`](voices.example.yaml).

```yaml
defaults:
  en: en_US-lessac-medium
  pl: pl_PL-gosia-medium
voices:
  - id: pl_PL-gosia-medium     # override a discovered model
    engine: piper
    speed: 1.05
  - id: pl-robot               # an espeak-ng variant
    engine: espeak
    lang: pl
    espeakVoice: pl+f3
    pitch: 60
```

Keys per voice: `id`, `engine` (`piper`|`espeak`), `lang`, `model` and
`config` (Piper, relative to the voices directory, default `<id>.onnx` and
`<model>.json`), `espeakVoice`, `name`, `gender`, `quality`, `speed`, `pitch`
(0-99, espeak), `speaker` (multi-speaker Piper models).

Without `voices.yaml`, every `<id>.onnx` with a valid `<id>.onnx.json` is a
Piper voice, and every espeak-ng language is a voice named `espeak:<lang>`.

## Engines

- **espeak-ng** (Debian package, 1.52) is always installed: 140 languages,
  small and fast, clearly synthetic.
- **Piper**: the image uses the standalone binary of
  [rhasspy/piper 2023.11.14-2](https://github.com/rhasspy/piper/releases/tag/2023.11.14-2)
  (amd64 and arm64, SHA-256 verified). It needs no Python, bundles its own
  onnxruntime and espeak-ng data, and runs as is on `debian:trixie-slim`.
  The rhasspy repository is archived; development moved to
  [OHF-Voice/piper1-gpl](https://github.com/OHF-Voice/piper1-gpl), whose
  releases are Python wheels only (v1.8.0) and would add a Python runtime to
  the image. Voice models are the same for both, so switching later only
  changes the Dockerfile and `internal/engine/piper.go`.
- **MP3**: `lame` at 64 kbit/s CBR mono. Piper and espeak-ng both output
  22.05 kHz, so the files are MPEG-2 Layer III (frame sync `FF F3`, not
  `FF FB`), which every browser and podcast player plays.

Engines sit behind the `engine.Engine` interface in `internal/engine`. To add
one, implement `Available`, `MaxChunk` and `Synthesize`, and register it in
`internal/app/app.go` under the name used in `voices.yaml`.

Long text is split into sentence chunks (1,500 characters for Piper, 4,000 for
espeak-ng). The chunks are synthesized in order and the PCM is joined before a
single encode. Measured in the image on one request: 5,400 characters of
Polish through Piper gave 5.5 minutes of audio in 13 s.

## Security

- Keys are compared as SHA-256 digests in constant time. Logs never contain
  keys, request text or headers, only method, path, status, size, duration
  and client IP.
- Request bodies are capped (speech: 4 bytes per allowed character + 4 KiB;
  uploads: `TTS_MAX_UPLOAD_MB` + 4 MiB). JSON is decoded strictly.
- Engines run through `exec.CommandContext` with an argv list, never a shell.
  The text goes in on stdin and never appears in argv. Each run is bounded by
  `TTS_TIMEOUT`.
- Uploads and deletes go through `os.Root`, so no id or file name can reach
  outside the voices directory. Ids must match `^[a-zA-Z0-9_.-]{1,64}$` and
  not start with a dot. The config JSON is validated, and the model must
  start like an ONNX protobuf.
- The container runs as uid 10001 with a read-only root filesystem, no
  capabilities and `no-new-privileges`. The port is published on 127.0.0.1
  only: put a reverse proxy or a cloudflared tunnel in front rather than
  opening it.
- Rate limiting uses the peer address and ignores `X-Forwarded-For`. Behind a
  proxy, every request shares one bucket unless clients use their own keys.
- `/docs` loads Swagger UI 5.33.1 from jsDelivr with SRI hashes under a strict
  CSP. Everything else is self-contained.

## Licences

- tts-server's Go code: BSD-3-Clause, like ssg.
- espeak-ng: GPL-3.0-or-later. lame: LGPL-2.0. Both come from Debian packages
  in the image.
- Piper (rhasspy/piper): MIT. Its release bundle includes onnxruntime (MIT)
  and a GPL-3 espeak-ng library. OHF-Voice/piper1-gpl is GPL-3.0.
- The image therefore contains GPL software. If you distribute it, you must
  meet the GPL source obligations. Running it yourself does not trigger them.
- **Every voice has its own licence.** Read the `MODEL_CARD` next to each model
  on Hugging Face before you publish audio. For example, `en_US-lessac` is
  trained on the Blizzard 2013 Lessac dataset, which has its
  [own licence terms](https://www.cstr.ed.ac.uk/projects/blizzard/2013/lessac_blizzard2013/license.html).
  `pl_PL-gosia` lists a CC0 dataset but was fine-tuned from lessac.

## Development

```sh
cd services/tts-server
go test -race -cover ./...     # 98.7% of statements
golangci-lint run ./... && gosec ./...
go run . -version
```

Layout: `main.go` (flags, graceful shutdown, `-healthcheck`),
`internal/config`, `internal/app` (wiring), `internal/httpapi` (router,
handlers, auth, rate limiting, embedded OpenAPI and Swagger UI),
`internal/synth` (voice resolution, cache, queue, chunking pipeline),
`internal/engine` (espeak-ng, Piper, sentence splitting), `internal/audio`
(PCM, WAV, lame), `internal/voices` (discovery, `voices.yaml`, upload store),
`internal/cache` (memory or disk LRU), `internal/limit` (concurrency gate,
token bucket), `internal/execx` (safe process runner).

Tests use fake engines and runners, so `go test` needs neither espeak-ng nor
Piper.
