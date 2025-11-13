package browse

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildListing_PairsOnly(t *testing.T) {
	root := t.TempDir()
	// paired
	write(t, filepath.Join(root, "a", "vid1.strm"), "plugin://plugin.video.youtube/play/?video_id=abc123")
	write(t, filepath.Join(root, "a", "vid1.nfo"), "<movie><title>T</title><plot>P</plot><thumb>u</thumb><tag>x</tag></movie>")
	// unpaired
	write(t, filepath.Join(root, "a", "vid2.strm"), "plugin://plugin.video.youtube/play/?video_id=zzz")
	write(t, filepath.Join(root, "a", "other.nfo"), "<movie></movie>")

	l, err := BuildListing(root, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Videos) != 1 {
		t.Fatalf("expected 1 video, got %d", len(l.Videos))
	}
	if l.Videos[0].VideoID != "abc123" {
		t.Fatalf("bad video id: %q", l.Videos[0].VideoID)
	}
}
