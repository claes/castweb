package http

import (
    "net/http/httptest"
    "os"
    "path/filepath"
    "testing"
)

func TestDirectoryRedirectsToTrailingSlash(t *testing.T) {
    root := t.TempDir()
    if err := os.Mkdir(filepath.Join(root, "HN"), 0o755); err != nil {
        t.Fatalf("mkdir: %v", err)
    }
    mux := NewServer(root, "", "")

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
}

