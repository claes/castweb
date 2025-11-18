package http

import (
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// Ensure that thumbnails containing special characters like '#' are correctly
// URL-encoded in HTML and can be fetched from the server.
func TestThumbWithHash_IsEncodedAndServed(t *testing.T) {
    root := t.TempDir()
    if err := os.Mkdir(filepath.Join(root, "HN"), 0o755); err != nil {
        t.Fatalf("mkdir: %v", err)
    }
    // Create image with '#'
    img := filepath.Join(root, "HN", "a#b.jpg")
    if err := os.WriteFile(img, []byte{0xFF, 0xD8, 0xFF}, 0o644); err != nil {
        t.Fatalf("write img: %v", err)
    }
    // Minimal .strm and .nfo pair
    if err := os.WriteFile(filepath.Join(root, "HN", "x.strm"), []byte("plugin://plugin.video.youtube/?video_id=abc123\n"), 0o644); err != nil {
        t.Fatalf("write strm: %v", err)
    }
    nfo := `<movie><title>T</title><plot>P</plot><thumb>a#b.jpg</thumb><tag>t</tag></movie>`
    if err := os.WriteFile(filepath.Join(root, "HN", "x.nfo"), []byte(nfo), 0o644); err != nil {
        t.Fatalf("write nfo: %v", err)
    }

    mux := NewServer(root, "", "")

    // Request directory; expect trailing-slash redirect first
    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/HN", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 301 {
        t.Fatalf("expected 301 redirect, got %d", rr.Code)
    }
    loc := rr.Header().Get("Location")
    if loc != "/HN/" {
        t.Fatalf("expected redirect to /HN/, got %q", loc)
    }

    // Follow to /HN/
    rr = httptest.NewRecorder()
    req = httptest.NewRequest("GET", "/HN/", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 200 {
        t.Fatalf("expected 200, got %d", rr.Code)
    }
    body := rr.Body.String()
    if want := `/HN/a%23b.jpg`; !strings.Contains(body, want) {
        t.Fatalf("expected body to contain %q, got: %s", want, body)
    }

    // Fetch the encoded image path
    rr = httptest.NewRecorder()
    req = httptest.NewRequest("GET", "/HN/a%23b.jpg", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 200 {
        t.Fatalf("expected 200 for image, got %d", rr.Code)
    }
    if ct := rr.Header().Get("Content-Type"); ct != "image/jpeg" {
        t.Fatalf("expected image/jpeg, got %q", ct)
    }
}
