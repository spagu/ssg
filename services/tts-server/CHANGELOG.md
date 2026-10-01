# Changelog — tts-server

All notable changes to `services/tts-server` are recorded here.

## [Unreleased] — ships with ssg 1.8.65

### Added
- Self-hosted TTS HTTP API: `POST /v1/speech` (MP3/WAV), `GET /v1/voices`,
  admin `POST /v1/voices` (multipart Piper upload) and `DELETE /v1/voices/{id}`,
  `/healthz`, `/readyz`, `/openapi.yaml` (OpenAPI 3.1) and `/docs` (Swagger UI 5.33.1).
- Engines: espeak-ng (all languages) and Piper (rhasspy/piper 2023.11.14-2
  standalone build); lame for MP3.
- Voice discovery from `TTS_VOICES_DIR`, optional strict `voices.yaml`,
  per-language default voices.
- Bearer-key auth, per-key token-bucket rate limiting, bounded concurrency with
  a short queue (503 + `Retry-After`), per-request timeout.
- Memory or disk LRU cache keyed by voice, revision, language, speed, format
  and text; `ETag` / `If-None-Match` support.
- Docker image (non-root, read-only root fs, Go healthcheck) with optional
  checksum-verified Piper voices via `WITH_VOICES`; docker compose file.
