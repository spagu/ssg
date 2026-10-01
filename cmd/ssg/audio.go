package main

// Wiring for the two read-aloud features (1.8.65): `tts:` becomes a client and
// the generator's audio options, `listen:` the button's.

import (
	"os"
	"strings"
	"time"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
	"github.com/spagu/ssg/internal/tts"
)

// buildAudioOptions turns `tts:` into generator options. A disabled block, or
// one the client rejects (unknown provider, missing api_url), turns the
// feature off with one line saying why — a build is never stopped by audio
// configuration unless strict asks for it, and then generateAudio does.
func buildAudioOptions(cfg *config.Config) generator.AudioOptions {
	t := cfg.TTS
	if !t.Enabled {
		return generator.AudioOptions{}
	}
	timeout, _ := time.ParseDuration(t.Timeout) // 0 ⇒ the client default
	client, err := tts.New(tts.Config{
		Provider: t.Provider, APIURL: t.APIURL, APIKey: envValue(t.APIKey),
		Voice: t.Voice, Lang: t.Lang, Model: t.Model, Speed: t.Speed, Instructions: t.Instructions,
		Timeout: timeout, Retries: t.Retries, MaxChars: t.MaxChars, Breaker: t.Breaker,
	})
	if err != nil {
		if strings.EqualFold(t.OnFailure, "fail") || cfg.Strict {
			errf("❌ %v\n", err)
			os.Exit(1)
		}
		errf("⚠️  %v — audio is off for this build\n", err)
		return generator.AudioOptions{}
	}
	return generator.AudioOptions{
		Client: client, OnFailure: t.OnFailure, Sections: t.Sections, Dir: t.Dir,
		JingleURL: t.JingleURL, Timeout: timeout, Feed: t.Feed, FeedPath: t.FeedPath,
		FeedTitle: t.FeedTitle, FeedAuthor: t.FeedAuthor, FeedImage: t.FeedImage, FeedLimit: t.FeedLimit,
	}
}

// buildListenOptions turns `listen:` into generator options.
func buildListenOptions(l config.ListenConfig) generator.ListenOptions {
	return generator.ListenOptions{Enabled: l.Enabled, Label: l.Label,
		NoAuto: l.Auto != nil && !*l.Auto, Sections: l.Sections}
}

// envValue resolves "$NAME" to the environment variable, so a key lives in
// the environment and the config can be committed.
func envValue(v string) string {
	if name, ok := strings.CutPrefix(strings.TrimSpace(v), "$"); ok {
		return os.Getenv(name)
	}
	return v
}
