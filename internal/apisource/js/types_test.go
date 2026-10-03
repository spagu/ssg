package js

import (
	"testing"

	"github.com/spagu/ssg/internal/apimodel"
)

func TestTypedefs(t *testing.T) {
	m, _ := extractIndex(t, `
/**
 * @typedef {Object} Pair
 * @property {T} left
 * @template T
 * @typedef {Pair<number>|null} MaybePair - A pair or nothing.
 * @callback Done
 */
/** @typedef {string} Shadow */
/** @hidden @typedef {string} Gone */
/**
 * @internal
 * @typedef {Object} Bare
 */
/** @typedef {number} */
/** The Pair type is not this one. */
export class Shadow {}`)
	pair := symbolNamed(t, m, "Pair")
	if pair.Type.Kind != apimodel.TypeObject || pair.Type.Fields[0].Type.Ref != "" || len(pair.TypeParams) != 1 || pair.Doc != nil {
		t.Errorf("Pair: %+v", pair)
	}
	maybe := symbolNamed(t, m, "MaybePair")
	if maybe.Type.Kind != apimodel.TypeUnion || maybe.Type.Args[0].Ref != "p/index#Pair" || maybe.Doc.Summary != "A pair or nothing." {
		t.Errorf("MaybePair: %v %+v", maybe.Type, maybe.Doc)
	}
	if done := symbolNamed(t, m, "Done"); done.Type.Kind != apimodel.TypeFunction {
		t.Errorf("Done: %v", done.Type)
	}
	if shadow := symbolNamed(t, m, "Shadow"); shadow.Kind != apimodel.KindClass {
		t.Error("an export wins over a typedef of the same name")
	}
	if bare := symbolNamed(t, m, "Bare"); !bare.Flags.Internal || bare.Type == nil || bare.Type.Name != "Object" {
		t.Errorf("Bare: %+v %v", bare.Flags, bare.Type)
	}
	if hasSymbol(m, "Gone") {
		t.Error("hidden typedef listed")
	}
}
