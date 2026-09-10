package editui

// AI actions in the editing panel (GO-102, phase 3).
//
// Four buttons over the model the site already has configured: shorten this
// excerpt to the length search engines display, propose a title that fits,
// write alt text for this image, translate this block.
//
// Two rules make them safe enough to have.
//
// **An action proposes; it never saves.** The answer lands in the field the
// person is editing, and the save is still the save they make themselves,
// through the same endpoint and the same validation. A model that misreads the
// instruction produces a suggestion someone rejects, not a change someone finds
// later.
//
// **The key never reaches the browser.** The dev server holds it exactly as the
// build does, calls the model itself, and hands back text. The panel has no
// credential and no way to talk to a provider.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AIAction is one of the things a button can ask for.
type AIAction struct {
	// Name is what the panel sends.
	Name string
	// Label is what the button says.
	Label string
	// Prompt builds the question from the text in the field.
	Prompt func(text string, limit int) string
	// Limit is the character budget the action works to, or zero.
	Limit int
}

// aiTimeout bounds one action. A person is waiting for it with a panel open.
const aiTimeout = 30 * time.Second

// aiActions are the four the ticket names, and no more: each one is a thing a
// person actually asks for while looking at a page.
func aiActions(excerptLimit int) map[string]AIAction {
	return map[string]AIAction{
		"shorten": {
			Name: "shorten", Label: "Shorten", Limit: excerptLimit,
			Prompt: func(text string, limit int) string {
				return fmt.Sprintf("Rewrite this so it reads naturally in at most %d characters. "+
					"Keep the meaning and the voice. Reply with the rewritten text only, no quotes and no preamble.\n\n%s",
					limit, text)
			},
		},
		"title": {
			Name: "title", Label: "Suggest a title", Limit: 60,
			Prompt: func(text string, limit int) string {
				return fmt.Sprintf("Suggest one title of at most %d characters for the text below. "+
					"It must be specific to this text rather than a category label. "+
					"Reply with the title only, no quotes and no preamble.\n\n%s", limit, text)
			},
		},
		"alt": {
			Name: "alt", Label: "Alt text",
			Prompt: func(text string, _ int) string {
				return "Write alternative text for the image described below, for a reader using a screen " +
					"reader. Say what the image shows, not that it is an image. One sentence. " +
					"Reply with the text only.\n\n" + text
			},
		},
		"translate": {
			Name: "translate", Label: "Translate",
			Prompt: func(text string, _ int) string {
				return "Translate the text below. Keep any Markdown formatting exactly as it is. " +
					"Reply with the translation only.\n\n" + text
			},
		},
	}
}

// aiRequest is one button press.
type aiRequest struct {
	Action string `json:"action"`
	Text   string `json:"text"`
	// Lang is the target for a translation.
	Lang string `json:"lang"`
	// Limit overrides the action's character budget.
	Limit int `json:"limit"`
}

// handleAI answers one action with a proposal.
func (s *Server) handleAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	if s.opts.AI == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error": "this site has no AI model configured — see docs/CONFIGURATION.md, ai:",
		})
		return
	}
	var req aiRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unreadable request: " + err.Error()})
		return
	}
	action, ok := aiActions(s.opts.ExcerptLimit)[req.Action]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown action " + req.Action})
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "there is no text to work from"})
		return
	}
	limit := action.Limit
	if req.Limit > 0 {
		limit = req.Limit
	}
	question := action.Prompt(text, limit)
	if req.Action == "translate" && req.Lang != "" {
		question = "Translate into " + req.Lang + ". " + question
	}

	answer, err := s.opts.AI(question, aiTimeout)
	if err != nil {
		s.opts.Logf("   ⚠️  edit: ai %s: %v", req.Action, err)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "the model did not answer: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"action": req.Action,
		// A proposal, named as one. The panel puts it in the field and the
		// person decides; nothing here writes to a file.
		"proposal": strings.TrimSpace(answer),
	})
}

// aiActionList describes the buttons for the panel, so the UI does not carry a
// second copy of the list.
func aiActionList(excerptLimit int) []map[string]any {
	actions := aiActions(excerptLimit)
	out := make([]map[string]any, 0, len(actions))
	for _, name := range []string{"shorten", "title", "alt", "translate"} {
		a := actions[name]
		out = append(out, map[string]any{"name": a.Name, "label": a.Label, "limit": a.Limit})
	}
	return out
}
