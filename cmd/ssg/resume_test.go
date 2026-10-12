package main

import (
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
)

// resume: reaches the generator unchanged.
func TestCreateGeneratorConfigResume(t *testing.T) {
	cfg := &config.Config{Resume: config.ResumeConfig{File: "cv.json", URL: "/cv/", Title: "CV", Contact: true}}
	got := createGeneratorConfig(cfg).Resume
	if got != (generator.ResumeOptions{File: "cv.json", URL: "/cv/", Title: "CV", Contact: true}) {
		t.Errorf("Resume = %+v", got)
	}
}
