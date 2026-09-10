package main

// The command line's quieter corners: flags that take a value after an equals
// sign, positional arguments that arrive in the wrong number, and the checks
// that are on by default and therefore need a way off.
//
// A mistyped flag that is silently ignored is worse than one that is rejected:
// the build succeeds and the setting the person asked for is simply absent.

import (
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
)

// fcovParse runs the flag parser over one argument and returns the config it
// produced.
func fcovParse(t *testing.T, args ...string) *config.Config {
	t.Helper()
	cfg := &config.Config{}
	parseFlags(args, cfg)
	return cfg
}

// TestNumericFlagsTakeTheirValueAndRefuseNonsense.
//
// Each of these has a range, and a value outside it means the person meant
// something else. Taking it anyway produces a build with, say, a WebP quality
// of 4000, which the encoder then interprets however it likes.
func TestNumericFlagsTakeTheirValueAndRefuseNonsense(t *testing.T) {
	cfg := fcovParse(t, "--avif-quality=55", "--webp-quality=70", "--max-conns=12",
		"--workers=3", "--paginate=25", "--feed-items=8")
	if cfg.AVIFQuality != 55 || cfg.WebPQuality != 70 {
		t.Errorf("image quality = %d/%d", cfg.AVIFQuality, cfg.WebPQuality)
	}
	if cfg.MaxConns != 12 || cfg.Paginate != 25 || cfg.FeedItems != 8 {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.BuildWorkers == nil || *cfg.BuildWorkers != 3 {
		t.Errorf("workers = %v", cfg.BuildWorkers)
	}

	// Out of range leaves the value alone rather than writing a nonsense one.
	outOfRange := fcovParse(t, "--webp-quality=4000", "--avif-quality=0")
	if outOfRange.WebPQuality != 0 || outOfRange.AVIFQuality != 0 {
		t.Errorf("an out-of-range quality was accepted: %+v", outOfRange)
	}
}

// TestImageFormatsKeepThePreferenceOrderTheyWereGivenIn, because
// --image-formats=avif,webp means offer AVIF first, not "these two in some
// order".
func TestImageFormatsKeepThePreferenceOrderTheyWereGivenIn(t *testing.T) {
	cfg := fcovParse(t, "--image-formats=avif,webp")
	if len(cfg.ImageFormats) != 2 || cfg.ImageFormats[0] != "avif" || cfg.ImageFormats[1] != "webp" {
		t.Errorf("formats = %v", cfg.ImageFormats)
	}
}

// TestPermalinkFlagsReachTheirContentType.
func TestPermalinkFlagsReachTheirContentType(t *testing.T) {
	cfg := fcovParse(t, "--permalink-post=/:year/:slug/")
	if cfg.Permalinks["post"] != "/:year/:slug/" {
		t.Errorf("permalinks = %v", cfg.Permalinks)
	}
	// An empty pattern is not a permalink, and writing it would replace the
	// default with nothing.
	if empty := fcovParse(t, "--permalink-post="); empty.Permalinks["post"] != "" {
		t.Errorf("permalinks = %v", empty.Permalinks)
	}
}

// TestTheChecksThatAreOnByDefaultCanBeTurnedOff.
//
// check_markup warns by default (#127) and check_meta follows the same shape
// (#111), so each needs both a bare form and a way off — a project that cannot
// silence an advisory ends up ignoring all of them.
func TestTheChecksThatAreOnByDefaultCanBeTurnedOff(t *testing.T) {
	if cfg := fcovParse(t, "--no-check-markup"); cfg.CheckMarkup != "" {
		t.Errorf("check_markup = %q", cfg.CheckMarkup)
	}
	for _, level := range []string{"warn", "strict", "off"} {
		if cfg := fcovParse(t, "--check-markup="+level); cfg.CheckMarkup != level {
			t.Errorf("--check-markup=%s → %q", level, cfg.CheckMarkup)
		}
	}
	// A level nobody defined is ignored rather than written through.
	if cfg := fcovParse(t, "--check-markup=loud"); cfg.CheckMarkup != "" {
		t.Errorf("an unknown level was accepted: %q", cfg.CheckMarkup)
	}
	for _, level := range []string{"warn", "strict"} {
		if cfg := fcovParse(t, "--check-meta="+level); cfg.CheckMeta != level {
			t.Errorf("--check-meta=%s → %q", level, cfg.CheckMeta)
		}
	}
	if cfg := fcovParse(t, "--check-meta=loud"); cfg.CheckMeta != "" {
		t.Errorf("an unknown level was accepted: %q", cfg.CheckMeta)
	}
}

// TestExtraPositionalArgumentsAreReportedRatherThanSwallowed.
//
// `ssg site simple example.com --paginate 10` works, but a typo in that flag
// leaves "10" sitting as a fourth positional. Ignoring it in silence is how a
// setting goes missing without anybody noticing (GO-053).
func TestExtraPositionalArgumentsAreReportedRatherThanSwallowed(t *testing.T) {
	out := captureStderr(t, func() {
		cfg := &config.Config{}
		args := []string{"site", "simple", "example.com", "10", "extra"}
		parseFlags(args, cfg)
		validateRequiredFields(args, cfg)
	})
	if !strings.Contains(out, "Ignoring unexpected positional") {
		t.Errorf("stderr = %q", out)
	}
	if !strings.Contains(out, "10") || !strings.Contains(out, "extra") {
		t.Errorf("the report should name what it ignored: %q", out)
	}
}

// TestATemplateAndDomainAreEnoughWhenTheSourceComesFromTheConfig.
//
// content_sources can supply the whole site, so the source argument becomes
// optional — and then two positionals mean template and domain, not source and
// template.
func TestATemplateAndDomainAreEnoughWhenTheSourceComesFromTheConfig(t *testing.T) {
	cfg := &config.Config{}
	args := []string{"--content-source=docs", "simple", "example.com"}
	parseFlags(args, cfg)
	validateRequiredFields(args, cfg)
	if cfg.Template != "simple" || cfg.Domain != "example.com" {
		t.Errorf("template = %q, domain = %q", cfg.Template, cfg.Domain)
	}
}
