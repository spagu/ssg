package main

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
)

func TestBuildPageToolOptions(t *testing.T) {
	off := false
	p := buildPrintOptions(config.PrintConfig{Enabled: true, Label: "Drukuj", Auto: &off,
		Sections: []string{"pages"}, Labels: map[string]string{"recipe": "Przepis"}})
	if !p.Enabled || p.Label != "Drukuj" || !p.NoAuto || len(p.Sections) != 1 || p.Labels["recipe"] != "Przepis" {
		t.Errorf("print options = %+v", p)
	}
	if buildPrintOptions(config.PrintConfig{}).NoAuto {
		t.Error("print: auto is on unless set to false")
	}
	c := buildCookModeOptions(config.CookModeConfig{Enabled: true, Label: "Gotuj", Auto: &off,
		Labels: map[string]string{"next": "Dalej"}})
	if !c.Enabled || c.Label != "Gotuj" || !c.NoAuto || c.Labels["next"] != "Dalej" {
		t.Errorf("cook options = %+v", c)
	}
	if buildCookModeOptions(config.CookModeConfig{}).NoAuto {
		t.Error("cook_mode: auto is on unless set to false")
	}
}
