package httpapi

import (
	"net/http"

	"github.com/spagu/ssg/services/tts-server/internal/voices"
)

// voiceList is the body of GET /v1/voices.
type voiceList struct {
	Voices []voices.Voice `json:"voices"`
	Count  int            `json:"count"`
}

// listVoices handles GET /v1/voices[?lang=pl|engine=piper].
func (a *api) listVoices(w http.ResponseWriter, r *http.Request) {
	lang := voices.NormalizeLang(r.URL.Query().Get("lang"))
	eng := r.URL.Query().Get("engine")
	out := voiceList{Voices: []voices.Voice{}}
	for _, v := range a.Registry.Catalog().List() {
		if (lang == "" || sameLanguage(v.Lang, lang)) && (eng == "" || v.Engine == eng) {
			out.Voices = append(out.Voices, v)
		}
	}
	out.Count = len(out.Voices)
	writeJSON(w, http.StatusOK, out)
}

// sameLanguage matches "pl" against "pl", "pl-pl"; and "en-gb" only against
// tags starting with it.
func sameLanguage(voiceLang, filter string) bool {
	n := len(filter)
	return voiceLang == filter || (len(voiceLang) > n && voiceLang[:n] == filter && voiceLang[n] == '-')
}
