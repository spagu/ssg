package main

// mddb.sources through the CLI: mapped onto the generator, watched as one (#361).

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/config"
	"github.com/spagu/ssg/internal/generator"
	"github.com/spagu/ssg/internal/mddb"
)

func TestMddbSourcesOf(t *testing.T) {
	if got := mddbSourcesOf(&config.Config{}); got != nil {
		t.Errorf("no sources = %v", got)
	}
	cfg := &config.Config{Mddb: config.MddbConfig{Sources: []config.MddbSource{{
		Collection: "articles", Filter: map[string][]string{"visibility": {"public"}}, Type: "post", Section: "articles",
	}}}}
	want := []generator.MddbSource{{
		Collection: "articles", Filter: map[string][]string{"visibility": {"public"}}, Type: "post", Section: "articles",
	}}
	if got := mddbSourcesOf(cfg); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

// TestMddbChecksumCoversEveryCollection: a change in any collection must
// change the checksum, and one collection keeps its own value.
func TestMddbChecksumCoversEveryCollection(t *testing.T) {
	srv, _ := wcovChecksumServer(t, []string{"a", "b"}, http.StatusOK)
	client := mddb.NewClient(mddb.Config{BaseURL: srv.URL})

	got, err := mddbChecksum(client, []string{"news", "kb"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Checksum != "a+b" || got.DocumentCount != 6 {
		t.Errorf("combined = %+v", got)
	}
	single, err := mddbChecksum(client, []string{"news"})
	if err != nil || single.Checksum != "b" {
		t.Errorf("single = %+v, %v", single, err)
	}

	down, _ := wcovChecksumServer(t, []string{"x"}, http.StatusInternalServerError)
	if _, err := mddbChecksum(mddb.NewClient(mddb.Config{BaseURL: down.URL}), []string{"news", "kb"}); err == nil ||
		!strings.HasPrefix(err.Error(), "news:") {
		t.Errorf("a failing collection must be named: %v", err)
	}
}
