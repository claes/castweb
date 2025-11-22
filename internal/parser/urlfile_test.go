package parser

import (
    "os"
    "path/filepath"
    "testing"
)

func TestParseURLFile_OK(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "x.url")
    if err := os.WriteFile(p, []byte("https://youtu.be/abc123\n"), 0o644); err != nil {
        t.Fatal(err)
    }
    u, err := ParseURLFile(p)
    if err != nil {
        t.Fatal(err)
    }
    if want := "https://youtu.be/abc123"; u != want {
        t.Fatalf("got %q, want %q", u, want)
    }
}

func TestParseURLFile_Empty(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "x.url")
    if err := os.WriteFile(p, []byte("\n\n"), 0o644); err != nil {
        t.Fatal(err)
    }
    u, err := ParseURLFile(p)
    if err != nil {
        t.Fatal(err)
    }
    if u != "" {
        t.Fatalf("expected empty, got %q", u)
    }
}

