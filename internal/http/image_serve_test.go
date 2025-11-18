package http

import (
    "net/http/httptest"
    "os"
    "path/filepath"
    "testing"
)

func TestServeImage_JPEG(t *testing.T) {
    root := t.TempDir()
    // Create a fake jpg file
    data := []byte{0xFF, 0xD8, 0xFF, 0xDB, 0x00, 0x00, 0x00} // minimal JPEG-like header bytes
    if err := os.WriteFile(filepath.Join(root, "test.jpg"), data, 0o644); err != nil {
        t.Fatalf("write jpg: %v", err)
    }
    mux := NewServer(root, "", "")

    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/test.jpg", nil)
    mux.ServeHTTP(rr, req)

    if rr.Code != 200 {
        t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
    }
    ct := rr.Header().Get("Content-Type")
    if ct != "image/jpeg" {
        t.Fatalf("expected image/jpeg, got %q", ct)
    }
    cc := rr.Header().Get("Cache-Control")
    if cc == "" || cc != "public, max-age=60" {
        t.Fatalf("expected Cache-Control public, max-age=60, got %q", cc)
    }
}

func TestServeImage_PNG_Nested(t *testing.T) {
    root := t.TempDir()
    // Nested directory
    if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
        t.Fatalf("mkdir: %v", err)
    }
    data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG signature
    if err := os.WriteFile(filepath.Join(root, "a", "b", "pic.PNG"), data, 0o644); err != nil {
        t.Fatalf("write png: %v", err)
    }
    mux := NewServer(root, "", "")

    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/a/b/pic.PNG", nil)
    mux.ServeHTTP(rr, req)

    if rr.Code != 200 {
        t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
    }
    ct := rr.Header().Get("Content-Type")
    if ct != "image/png" {
        t.Fatalf("expected image/png, got %q", ct)
    }
    cc := rr.Header().Get("Cache-Control")
    if cc == "" || cc != "public, max-age=60" {
        t.Fatalf("expected Cache-Control public, max-age=60, got %q", cc)
    }
}

func TestServeImage_NotFound(t *testing.T) {
    root := t.TempDir()
    mux := NewServer(root, "", "")

    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/nope.jpg", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 404 {
        t.Fatalf("expected 404, got %d", rr.Code)
    }
}
