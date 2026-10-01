---
title: "Two Ways to Read an Article Aloud"
slug: "two-ways-to-read-an-article-aloud"
status: publish
type: post
date: 2026-10-01
tags: [release, audio, accessibility, podcast]
excerpt: "1.8.65 adds a Listen button that uses the reader's own voice engine, and build-time MP3 from a TTS API you choose, with a podcast feed and a plan for when the API is down."
---

People listen to articles while they cook, drive or rest their eyes. A site can
help in two ways, and they cost very different amounts. ssg 1.8.65 does both,
and you can turn on either one or both.

## The browser does it

Every current desktop and phone browser ships a speech engine, exposed to pages
as the Web Speech API. A button that hands it the article costs the site
nothing. There is no service to run, no audio to store, and the text never
leaves the reader's device.

```yaml
listen:
  enabled: true
  label: "Listen"
```

The button appears under the post title. It reads one paragraph at a time.
That is deliberate: Chrome stops a single long utterance after about fifteen
seconds, so one utterance per paragraph is what lets a long article finish.
Press it again to stop. It reports its state with `aria-pressed`, so a screen
reader announces it, and it stays hidden where the API is missing or JavaScript
is off. Nobody sees a button that does nothing.

What this cannot control is the voice. A recent Mac, iPhone or Android phone
has good neural voices for the major languages. A minimal Linux desktop may have
a robotic one, or none. The reader gets whatever their device has.

## The build does it

The second way makes an MP3 of each article at build time, through a
text-to-speech API, and puts a player on the page instead of the button.
Everyone hears the same voice, the file plays offline, and it can go into a
podcast feed.

```yaml
tts:
  enabled: true
  provider: openai            # or generic, elevenlabs, google
  api_key: $OPENAI_API_KEY
  voice: alloy
  jingle_url: https://example.com/intro.mp3
  feed: true
```

Four providers are built in:

| Provider | Per request | What you set |
|---|---|---|
| `openai` | 4,096 characters | `voice`, `model`, and `instructions` such as "calm, unhurried" |
| `elevenlabs` | split at 4,500 | `voice` (the voice id), `model` |
| `google` | 5,000 bytes | `voice` such as `pl-PL-Wavenet-A` |
| `generic` | 20,000 characters | `api_url`; any server that takes the JSON below |

Longer articles are split at sentence ends and the parts are joined into one
file. Code blocks are left out, because "func main, open brace" helps no
listener. The jingle is downloaded once and played before every article.

`generic` is a small contract you can implement in an afternoon:
`POST {"text", "voice", "lang", "speed", "format": "mp3"}` with a bearer key,
answered with `audio/mpeg`. The repository ships a server that implements it.

## A TTS server on your own machine

`services/tts-server` is a Go API in a Docker image. It uses espeak-ng, which
lists about 140 voices across its languages, and Piper for neural voices. You can bake voices into the
image at build time or upload them to a running server.

```bash
cd services/tts-server
cp .env.example .env          # set TTS_API_KEYS and TTS_ADMIN_KEYS
WITH_VOICES=en_US-lessac-medium,pl_PL-gosia-medium docker compose up -d --build
open http://127.0.0.1:8080/docs   # Swagger UI
```

Then point the site at it:

```yaml
tts:
  enabled: true
  provider: generic
  api_url: http://127.0.0.1:8080/v1/speech
  api_key: $TTS_API_KEY
```

On a 16-core desktop, the Polish Piper voice read 5,433 characters, about five
and a half minutes of speech, in 13.1 seconds. The result was a 2.6 MB MP3.
espeak-ng is faster still and sounds like the machine it is. Check the voice's
licence before you publish what it says: Piper voices are trained on datasets
with their own terms, and the server's README links them.

## Paying once

A TTS API charges per character, so the build must not send the same article
twice. Each MP3 is cached under a key made from everything that changes the
sound: provider, voice, model, speed, language, jingle and text. A rebuild calls
the API only for articles whose words changed. In our test, the second build of
a two-article site printed:

```text
🎧 audio: 0 made, 2 from cache, 0 stale, 0 without
```

Keep `.ssg-cache/` between CI runs. Without it, every deploy pays for every
article again.

## When the API is down

A build that depends on someone else's service needs a plan for when that
service does not answer. This one has three parts.

1. **Retry what is worth retrying.** A timeout, a network error, `429` or a
   `5xx` is tried again after 1 s, 2 s, 4 s, or after what `Retry-After`
   asks for. A `401` or an unknown voice is not, because asking again
   changes nothing.
2. **Stop asking.** After three articles fail in a row, the API is not called
   for the rest of the build. A down API would otherwise cost the timeout times
   the retries times every article on the site.
3. **Decide what the page gets.** `on_failure: stale`, the default, publishes
   that page's last good MP3, even if the text has changed since. `skip`
   publishes no audio. `fail` stops the build, and `strict: true` does the same.
   A page left without audio still gets the browser button.

We stopped the server, edited one article and rebuilt. The build said why once,
used the old reading for the edited page and the cache for the other, and
finished in three seconds:

```text
⚠️  audio for /2026/10/01/hello/: … connect: connection refused
🎧 audio: 0 made, 1 from cache, 1 stale, 0 without
```

## The podcast feed

`tts.feed: true` writes `podcast.xml`: RSS 2.0 with an `<enclosure>` for every
MP3 and the iTunes tags podcast apps read. Subscribe to it in any podcast app
and the blog arrives as episodes, with the jingle at the start of each.

## Which one to use

Use the button if you want listening at no cost and do not mind each reader
hearing their own device. Use the MP3 if the voice matters, if people listen
away from a screen, or if you want a podcast. Use both if you want the button to
cover the pages the API could not do.

Configuration reference: [docs/AUDIO.md](https://github.com/spagu/ssg/blob/main/docs/AUDIO.md).
