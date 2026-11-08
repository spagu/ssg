package main

import (
	"reflect"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
	"github.com/spagu/ssg/internal/ogcard"
)

func TestBuildOGCardOptions(t *testing.T) {
	if got := buildOGCardOptions(config.OGCardsConfig{Dir: "x"}); !reflect.DeepEqual(got, generator.OGCardOptions{}) {
		t.Errorf("disabled: %+v", got)
	}
	got := buildOGCardOptions(config.OGCardsConfig{Enabled: true, Dir: "cards", Sections: []string{"posts"},
		Width: 1, Height: 2, Background: "#000", TextColor: "#fff", AccentColor: "#8ab4f8",
		BackgroundImage: "bg.jpg", Overlay: 0.4, Logo: "l.png", Font: "f.ttf", Format: "jpeg", Quality: 70})
	want := generator.OGCardOptions{Enabled: true, Dir: "cards", Sections: []string{"posts"}, Spec: ogcard.Spec{
		Width: 1, Height: 2, Background: "#000", Text: "#fff", Accent: "#8ab4f8",
		BackgroundImage: "bg.jpg", Overlay: 0.4, Logo: "l.png", Font: "f.ttf", Format: "jpeg", Quality: 70}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestOGCardSkipDirs(t *testing.T) {
	if got := ogCardSkipDirs(config.OGCardsConfig{}); got != nil {
		t.Errorf("disabled: %v", got)
	}
	if got := ogCardSkipDirs(config.OGCardsConfig{Enabled: true}); !reflect.DeepEqual(got, []string{"og"}) {
		t.Errorf("default: %v", got)
	}
	if got := ogCardSkipDirs(config.OGCardsConfig{Enabled: true, Dir: "/share/cards/"}); !reflect.DeepEqual(got, []string{"share/cards"}) {
		t.Errorf("custom: %v", got)
	}
}
