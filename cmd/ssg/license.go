package main

// Which license this binary is under, said where people look for it (1.9.1).
//
// ssg changed from BSD-3-Clause to the Business Source License in 1.9.0. A
// binary that only says "ssg version 1.9.0" leaves someone who installed
// `latest` no way to notice that, short of reading the repository. So the
// license is named wherever the version is: `ssg --version`, the help text,
// the first line of a build log, and the hint a bare `ssg` prints.
// `ssg --license` gives the terms in a few lines.

import (
	"fmt"
	"io"
)

const (
	// licenseName and licenseSPDX name the license of this version.
	licenseName = "Business Source License 1.1"
	licenseSPDX = "BUSL-1.1"
	// licenseURL is the license text with ssg's parameters; licensingURL
	// explains it in plain terms.
	licenseURL   = "https://github.com/spagu/ssg/blob/main/LICENSE"
	licensingURL = "https://github.com/spagu/ssg/blob/main/docs/LICENSING.md"
)

// licenseLine is the one-line form, for the places that already print the
// version.
func licenseLine() string {
	return fmt.Sprintf("License: %s (%s); run `ssg --license` for the terms", licenseName, licenseSPDX)
}

// writeLicense prints the terms of the license in a few lines. The LICENSE
// file is the text that counts, and the last lines point at it.
func writeLicense(w io.Writer) {
	lines := []string{
		fmt.Sprintf("ssg %s is licensed under the %s (%s).", Version, licenseName, licenseSPDX),
		"",
		"  Free to use for any site, commercial ones included.",
		"  Non-commercial sites (personal, educational, charitable, public sector,",
		"  non-profit): no condition.",
		"  Commercial sites (a business, a shop, paid ads or sponsorship, a site",
		"  built for a client): keep the \"Built with ssg\" notice ssg writes.",
		"  A commercial license is needed to remove that notice from a commercial",
		"  site, or to offer ssg itself as a hosted service, inside a paid product",
		"  or as a renamed copy.",
		"  Your content and the sites you generate are yours.",
		"  Each version becomes BSD-3-Clause four years after its release",
		"  (1.9.0 on 2030-10-31). Versions up to 1.8.85 remain BSD-3-Clause.",
		"",
		"License text: " + licenseURL,
		"Explained:    " + licensingURL,
	}
	for _, l := range lines {
		_, _ = fmt.Fprintln(w, l)
	}
}
