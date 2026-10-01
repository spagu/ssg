package synth

import (
	"github.com/spagu/ssg/services/tts-server/internal/engine"
	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

type engineIface = engine.Engine

// catalogOf builds a catalog through the public Source API.
func catalogOf(vs ...voices.Voice) voices.Catalog {
	return voices.CatalogOf(vs...)
}
