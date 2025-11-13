package parser

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleNFO = `<?xml version="1.0" encoding="UTF-8"?>
<movie><title>Strange Filters</title><plot>Exploring...</plot><thumb>https://i3.ytimg.com/vi/zbKjqHqy2no/hqdefault.jpg</thumb><tag>Posy</tag><tag>Demo</tag></movie>`

func TestParseNFO_OK(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.nfo")
	if err := os.WriteFile(p, []byte(sampleNFO), 0o644); err != nil {
		t.Fatal(err)
	}
	title, plot, thumb, tags, err := ParseNFO(p)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Strange Filters" {
		t.Fatalf("bad title: %q", title)
	}
	if plot == "" {
		t.Fatalf("empty plot")
	}
	if thumb == "" {
		t.Fatalf("empty thumb")
	}
	if len(tags) != 2 {
		t.Fatalf("want 2 tags, got %d", len(tags))
	}
}
