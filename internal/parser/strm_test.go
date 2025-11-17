package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStream_ExtractsYouTubeID(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "x.strm")
    content := "plugin://plugin.video.youtube/play/?video_id=zbKjqHqy2no\n"
    if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
        t.Fatal(err)
    }
    typ, id, err := ParseStream(p)
    if err != nil { t.Fatal(err) }
    if typ != "youtube" { t.Fatalf("want type youtube, got %q", typ) }
    if id != "zbKjqHqy2no" { t.Fatalf("want zbKjqHqy2no, got %q", id) }
}
