# Articles read aloud

ssg can make an article listenable in two ways. They work alone or together.

| | Browser button (`listen:`) | Build-time MP3 (`tts:`) |
|---|---|---|
| Where the voice comes from | The visitor's device (Web Speech API) | A TTS API, called while the site builds |
| Cost | Nothing | The API's price, once per changed article |
| Same voice for everyone | No: whatever the device has | Yes |
| Works offline, in a podcast app | No | Yes, and there is a podcast feed |
| Needs JavaScript | Yes | No: a plain `<audio>` element |
| Leaves the device | Nothing | The article text goes to the API at build time |

When both are on, a page with an MP3 shows the player and a page without one
shows the button. That includes a page whose MP3 could not be made because the
API was down, so a failed build-time call still leaves the reader a way to
listen.

## The browser button

```yaml
listen:
  enabled: true
  label: "Listen"        # button text
  sections: [posts]      # posts (default), pages, or both
  auto: true             # place it where the theme did not; default true
```

The button reads the article with `speechSynthesis` in the page's language
(`<html lang>`), one paragraph at a time. Chrome stops a single long utterance
after about 15 seconds, so one utterance per paragraph is what lets a long
article finish. Pressing it again stops. It reports its state with
`aria-pressed`, and it stays hidden in browsers without the API and when
JavaScript is off, so nobody sees a button that does nothing.

Voice quality depends on the device. Recent macOS, iOS, Android and Windows
ship good neural voices for major languages; a minimal Linux desktop may have
only a robotic one, or none.

## The MP3

```yaml
tts:
  enabled: true
  provider: generic                       # generic | openai | elevenlabs | google
  api_url: http://localhost:8080/v1/speech
  api_key: $TTS_API_KEY                   # from the environment; never a literal
  voice: en_US-lessac-medium
  lang: ""                                # default: each page's language
  speed: 1.0
  jingle_url: https://example.com/intro.mp3   # optional, played before every article
  sections: [posts]
  dir: audio                              # → /audio/<page>.mp3
  timeout: 60s                            # per request
  retries: 2                              # extra attempts on 429, 5xx, network errors
  breaker: 3                              # failed articles in a row → stop calling
  on_failure: stale                       # stale | skip | fail
  feed: true                              # podcast feed
  feed_path: podcast.xml
  feed_title: "Example, read aloud"
  feed_author: "Example"
  feed_image: /images/podcast.png
  feed_limit: 50
```

What is read: the title, then the article text with code blocks removed and
markup stripped. A listener does not need `func main() {` spelled out.

### Cache

Each MP3 is cached in `.ssg-cache/tts/` under a key made from everything that
changes the sound: provider, endpoint, model, voice, speed, instructions,
language, the jingle and the text. A rebuild calls the API only for articles
whose words changed. `ssg cache clean --namespace=tts` forgets them all. Keep
`.ssg-cache/` between CI runs (see [DEPLOYMENT.md](DEPLOYMENT.md#build-cache-in-ci)),
or every build pays for every article again.

The jingle is downloaded once per URL and cached next to the audio. It is joined
in front of each article's MP3, after the ID3 tags between the two are removed.
Both should use the same encoding, for example 44.1 kHz stereo MP3.

### When the API does not answer

A TTS API is a network dependency, and a build should not wait on it once it is
plainly down:

1. **Timeout and retries.** Each request has `timeout`. A network error, a
   timeout, `429` or a `5xx` is retried `retries` times, waiting 1 s, 2 s, 4 s…
   or what `Retry-After` asks for, capped at 30 s. A `4xx` other than `429`
   (bad key, unknown voice) is not retried, because asking again changes nothing.
2. **Breaker.** After `breaker` articles fail in a row, the API is not called
   again for the rest of the build. One line says so. Without it, a down API
   costs `timeout × retries × articles`.
3. **`on_failure`** decides what the page gets:
   - `stale` (default): the last MP3 that page ever had, from
     `.ssg-cache/tts/last/`, even if the text has changed since. If there is
     none, the page has no audio.
   - `skip`: no audio for that page.
   - `fail`: the build stops.

   `strict: true` makes `stale` and `skip` behave as `fail`.

A page without audio still gets the browser button when `listen` is on.

### Providers

`generic` is ssg's own contract: `POST api_url` with JSON
`{"text", "voice", "lang", "speed", "format": "mp3"}` and
`Authorization: Bearer <api_key>`, answered with `audio/mpeg`. The self-hosted
server in [`services/tts-server/`](../services/tts-server/README.md) implements
it, and so can any small adapter you write in front of another engine.

| Provider | Endpoint (default) | Auth | Per request | Notes |
|---|---|---|---|---|
| `generic` | your `api_url` | `Authorization: Bearer` | 20,000 chars | the contract above |
| `openai` | `https://api.openai.com/v1/audio/speech` | `Authorization: Bearer` | 4,096 chars | `model` default `gpt-4o-mini-tts`, `voice` default `alloy`; `instructions` steer delivery |
| `elevenlabs` | `https://api.elevenlabs.io/v1/text-to-speech/{voice}` | `xi-api-key` | split at 4,500 | `voice` (the voice id) is required; `model` default `eleven_multilingual_v2`; MP3 44.1 kHz 128 kbps |
| `google` | `https://texttospeech.googleapis.com/v1/text:synthesize` | `X-Goog-Api-Key` | 5,000 bytes | `voice` is a voice name such as `pl-PL-Wavenet-A`; audio arrives base64 in JSON |

Longer articles are split at sentence ends under the limit, and the parts are
joined into one file. `max_chars` overrides the split size.

Services without a simple API-key request (Amazon Polly signs requests with
SigV4, Azure Speech takes SSML) are reached through `generic` with a small
adapter in front.

### Templates

`{{ listen .Post }}` (or `.Page`) renders the player when the page has an MP3,
otherwise the button when `listen` is on, otherwise nothing. The Go-template
themes that ship with ssg (simple, ssgtheme, krowy, imd) call it under the post
title. A theme that does not call it gets the block after
the first `</h1>` of each selected page, unless `listen.auto` is `false`.

The page also exposes `.AudioURL` (site-relative, e.g. `/audio/2026-10-01-hello.mp3`)
and `.AudioLength` (bytes), for a theme that wants its own player.

### The podcast feed

With `tts.feed: true`, `podcast.xml` lists every page that has an MP3, newest
first, as RSS 2.0 with `<enclosure>` elements and the iTunes namespace. Podcast
apps and RSS readers subscribe to it directly. URLs are absolute on `domain`.
