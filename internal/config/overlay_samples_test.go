package config

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// The designer-ui worker writes .ssg.designer.json files; the build must
// accept every one it produces. The samples are the worker's own output
// (workers/designer-ui/test/overlay-samples.json, regenerated there with
// UPDATE_OVERLAY_SAMPLES=1), so the two sides cannot drift apart unseen.
func TestWorkerOverlaySamplesValidate(t *testing.T) {
	raw, err := os.ReadFile("../../workers/designer-ui/test/overlay-samples.json")
	if err != nil {
		t.Fatal(err)
	}
	var samples []struct {
		Name string `json:"name"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &samples); err != nil {
		t.Fatal(err)
	}
	if len(samples) == 0 {
		t.Fatal("no samples")
	}
	for _, s := range samples {
		dec := json.NewDecoder(bytes.NewReader([]byte(s.Text)))
		dec.UseNumber()
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			t.Errorf("%s: not JSON: %v", s.Name, err)
			continue
		}
		if err := ValidateOverlay(doc); err != nil {
			t.Errorf("%s: ssg refuses what the worker wrote: %v", s.Name, err)
		}
	}
}
