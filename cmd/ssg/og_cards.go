package main

// Wiring for social preview cards (1.9.0): `og_cards:` becomes the
// generator's options, and the image passes are told to leave the cards be.

import (
	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
	"github.com/spagu/ssg/internal/ogcard"
)

// buildOGCardOptions turns `og_cards:` into generator options.
func buildOGCardOptions(c config.OGCardsConfig) generator.OGCardOptions {
	if !c.Enabled {
		return generator.OGCardOptions{}
	}
	return generator.OGCardOptions{
		Enabled: true, Dir: c.Dir, Sections: c.Sections,
		Spec: ogcard.Spec{
			Width: c.Width, Height: c.Height,
			Background: c.Background, Text: c.TextColor, Accent: c.AccentColor,
			BackgroundImage: c.BackgroundImage, Overlay: c.Overlay, Logo: c.Logo, Font: c.Font,
			Format: c.Format, Quality: c.Quality,
		},
	}
}

// ogCardSkipDirs is the card directory, which the WebP and AVIF passes must
// not convert: LinkedIn and WhatsApp do not reliably render WebP previews,
// and the replace-in-place WebP mode would delete the PNG og:image names.
func ogCardSkipDirs(c config.OGCardsConfig) []string {
	if !c.Enabled {
		return nil
	}
	return []string{buildOGCardOptions(c).OGCardDir()}
}
