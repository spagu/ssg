package main

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestBuildAudioOptions(t *testing.T) {
	cfg := &config.Config{}
	if opts := buildAudioOptions(cfg); opts.Client != nil {
		t.Error("tts disabled must give no client")
	}
	t.Setenv("SSG_TEST_TTS_KEY", "secret")
	cfg.TTS = config.TTSConfig{Enabled: true, APIURL: "http://localhost:8080/v1/speech", APIKey: "$SSG_TEST_TTS_KEY",
		Timeout: "5s", Feed: true, FeedTitle: "T", OnFailure: "skip", Dir: "listen"}
	opts := buildAudioOptions(cfg)
	if opts.Client == nil || !opts.Feed || opts.FeedTitle != "T" || opts.OnFailure != "skip" || opts.Dir != "listen" {
		t.Errorf("options = %+v", opts)
	}
	cfg.TTS = config.TTSConfig{Enabled: true, Provider: "nope"}
	if opts := buildAudioOptions(cfg); opts.Client != nil {
		t.Error("an unknown provider turns audio off rather than failing the build")
	}
}

func TestBuildListenOptions(t *testing.T) {
	off := false
	got := buildListenOptions(config.ListenConfig{Enabled: true, Label: "Hear", Auto: &off, Sections: []string{"pages"}})
	if !got.Enabled || got.Label != "Hear" || !got.NoAuto || len(got.Sections) != 1 {
		t.Errorf("options = %+v", got)
	}
	if buildListenOptions(config.ListenConfig{}).NoAuto {
		t.Error("auto is on unless set to false")
	}
}

func TestEnvValue(t *testing.T) {
	t.Setenv("SSG_TEST_ENV_VALUE", "v")
	if envValue("$SSG_TEST_ENV_VALUE") != "v" || envValue("literal") != "literal" || envValue("$SSG_TEST_UNSET_XYZ") != "" {
		t.Error("envValue")
	}
}
