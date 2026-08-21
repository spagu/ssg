package generator

// The timestamp a build stamps into templates (#186).
//
// The generator never calls time.Now while rendering, and that is deliberate:
// two pages of one build must not disagree about when they were built, and a
// rebuild of unchanged sources must produce unchanged bytes. So the clock is
// read exactly once, when the generator is constructed, and every template sees
// that one value.
//
// SOURCE_DATE_EPOCH — the reproducible-builds convention — overrides it. A CI
// job, a test or a distribution package that pins it gets a build whose output
// is a pure function of its input; everyone else gets the real clock, which is
// what makes `© 2007-{{.BuildTime.Year}}` correct the night the year turns
// instead of the next time someone remembers to edit the footer.

import (
	"os"
	"strconv"
	"time"
)

// sourceDateEpoch is the environment variable that pins a build's timestamp.
const sourceDateEpoch = "SOURCE_DATE_EPOCH"

// resolveBuildTime returns the timestamp for this build: SOURCE_DATE_EPOCH when
// it holds a valid Unix second count, otherwise now().
//
// A malformed value is ignored rather than fatal. The variable is frequently set
// by a surrounding toolchain the site owner does not control, and refusing to
// build over it would turn someone else's typo into a broken deploy — the build
// is still internally consistent either way.
func resolveBuildTime(now func() time.Time) time.Time {
	if raw, ok := os.LookupEnv(sourceDateEpoch); ok {
		if secs, err := strconv.ParseInt(raw, 10, 64); err == nil {
			// UTC, because a pinned epoch is meant to be reproducible on any
			// machine, and a local zone would make the rendered date depend on
			// where the build ran.
			return time.Unix(secs, 0).UTC()
		}
	}
	return now()
}
