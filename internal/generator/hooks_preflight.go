package generator

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spagu/ssg/internal/snapenv"
)

// fatalHookPhases are the phases whose failure stops the build; their programs
// are resolved before anything renders. post_page failures are logged and the
// build goes on, so a missing post_page program is reported when it runs.
var fatalHookPhases = []string{"pre_build", "post_build"}

// checkHookCommands resolves the program of every pre_build and post_build
// hook before the build starts (#397). A post_build hook used to fail only
// after every page had rendered; a typo, an uninstalled interpreter or the
// snap's confinement now stops the build at once, before pre_build runs.
func (g *Generator) checkHookCommands() error {
	for _, phase := range fatalHookPhases {
		for _, cmdline := range g.config.Hooks[phase] {
			fields := strings.Fields(cmdline)
			if len(fields) == 0 {
				continue
			}
			if _, err := exec.LookPath(fields[0]); err != nil {
				// Same shape as the error the hook would have produced when run.
				return fmt.Errorf("%s hook: %w", phase, hookError(cmdline, phase, err))
			}
		}
	}
	return nil
}

// hookError wraps a hook failure. When the program was not found and this is
// the strictly confined snap, it says why: the snap cannot see programs
// installed on the host (node, python, a shell script's interpreter), and no
// PATH setting changes that (#397). The cause stays reachable via errors.Is.
func hookError(cmdline, phase string, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		if root := snapenv.Root(); root != "" {
			return fmt.Errorf("hook %q (%s): %w — this ssg is the snap (%s), and strict confinement hides programs installed on the host; "+
				"install ssg another way (install.sh, .deb/.rpm, Homebrew, go install) to run hooks that call host programs",
				cmdline, phase, err, root)
		}
	}
	return fmt.Errorf("hook %q (%s): %w", cmdline, phase, err)
}
