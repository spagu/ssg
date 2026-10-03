package js

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spagu/ssg/internal/apisource"
)

// generatedPackage writes n small modules and an index re-exporting them.
func generatedPackage(tb testing.TB, n int) apisource.Config {
	files := map[string]string{}
	var index strings.Builder
	for i := range n {
		fmt.Fprintf(&index, "export * from './m/m%d.js';\n", i)
		files[fmt.Sprintf("m/m%d.js", i)] = fmt.Sprintf(`/**
 * Adds %[1]d.
 *
 * @param {number} x - The input.
 * @returns {number}
 */
export function add%[1]d(x = 0) { return x + %[1]d; }

/** A counter. */
export class Counter%[1]d {
  /** @type {number} */
  value = %[1]d;
  get double() { return this.value * 2; }
}
`, i)
	}
	files["index.js"] = index.String()
	return apisource.Config{Name: "gen", Root: writeTree(tb, files)}
}

func TestThousandModulesUnderASecond(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	cfg := generatedPackage(t, 1000)
	start := time.Now()
	pkg, diags, err := Extract(cfg)
	elapsed := time.Since(start)
	if err != nil || len(diags) != 0 {
		t.Fatalf("err=%v diags=%v", err, diags)
	}
	if got := len(pkg.Modules[0].Symbols); got != 2000 {
		t.Fatalf("symbols = %d, want 2000", got)
	}
	// One second is the goal; the bound is loose for race and coverage builds.
	if elapsed > 5*time.Second {
		t.Errorf("1000 modules took %v", elapsed)
	}
	t.Logf("1000 modules in %v", elapsed)
}

func BenchmarkExtract1000(b *testing.B) {
	cfg := generatedPackage(b, 1000)
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := Extract(cfg); err != nil {
			b.Fatal(err)
		}
	}
}
