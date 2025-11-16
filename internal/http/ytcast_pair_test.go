package http

import (
    "net/http/httptest"
    "os"
    "path/filepath"
    "runtime"
    "testing"
)

// createFakeYtcast creates a temporary executable named "ytcast" that records
// its arguments to a file and exits 0.
func createFakeYtcast(t *testing.T, dir string) string {
    t.Helper()
    exe := "ytcast"
    if runtime.GOOS == "windows" {
        exe += ".bat"
    }
    path := filepath.Join(dir, exe)
    var content string
    if runtime.GOOS == "windows" {
        // Basic batch file that succeeds
        content = "@echo off\r\nexit /b 0\r\n"
    } else {
        content = "#!/usr/bin/env sh\n" +
            "# fake ytcast for tests\n" +
            "exit 0\n"
    }
    if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
        t.Fatalf("write fake ytcast: %v", err)
    }
    return path
}

func TestYtcastPair_Validation(t *testing.T) {
    mux := NewServer(t.TempDir(), "")

    // missing code
    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/ytcast/pair", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 400 {
        t.Fatalf("expected 400 for missing code, got %d", rr.Code)
    }

    // invalid (non-digits)
    rr = httptest.NewRecorder()
    req = httptest.NewRequest("GET", "/ytcast/pair?code=abc123", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 400 {
        t.Fatalf("expected 400 for invalid code, got %d", rr.Code)
    }

    // invalid (wrong length)
    rr = httptest.NewRecorder()
    req = httptest.NewRequest("GET", "/ytcast/pair?code=1234567890", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 400 {
        t.Fatalf("expected 400 for wrong length, got %d", rr.Code)
    }
}

func TestYtcastPair_Executes(t *testing.T) {
    tmp := t.TempDir()
    // Ensure a fake ytcast exists in PATH
    _ = createFakeYtcast(t, tmp)
    t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

    mux := NewServer(t.TempDir(), "")

    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/ytcast/pair?code=123456789012", nil)
    mux.ServeHTTP(rr, req)

    if rr.Code != 204 {
        t.Fatalf("expected 204 on successful pairing, got %d; body=%s", rr.Code, rr.Body.String())
    }
}
