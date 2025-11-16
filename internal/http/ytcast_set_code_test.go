package http

import (
    "net/http/httptest"
    "os"
    "path/filepath"
    "runtime"
    "strings"
    "testing"
)

// createTracingYtcast creates a temporary executable named "ytcast" that
// writes all received arguments to the file specified by TRACE_PATH env var.
func createTracingYtcast(t *testing.T, dir string) string {
    t.Helper()
    exe := "ytcast"
    if runtime.GOOS == "windows" {
        exe += ".bat"
    }
    path := filepath.Join(dir, exe)
    var content string
    if runtime.GOOS == "windows" {
        // Very simple tracer for Windows
        content = "@echo off\r\n" +
            "setlocal enabledelayedexpansion\r\n" +
            "if \"%TRACE_PATH%\"==\"\" exit /b 0\r\n" +
            "set args=%*\r\n" +
            "echo %args%> %TRACE_PATH%\r\n" +
            "exit /b 0\r\n"
    } else {
        content = "#!/usr/bin/env sh\n" +
            ": \"${TRACE_PATH:?}\"\n" +
            "printf '%s' \"$*\" > \"$TRACE_PATH\"\n" +
            "exit 0\n"
    }
    if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
        t.Fatalf("write tracer ytcast: %v", err)
    }
    return path
}

func TestYtcastSetCode_Validation(t *testing.T) {
    mux := NewServer(t.TempDir(), "")

    // missing
    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/ytcast/set-code", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 400 {
        t.Fatalf("expected 400, got %d", rr.Code)
    }
    // invalid
    rr = httptest.NewRecorder()
    req = httptest.NewRequest("GET", "/ytcast/set-code?code=abc", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 400 {
        t.Fatalf("expected 400, got %d", rr.Code)
    }
}

func TestYtcastSetCode_AppliesToPlay(t *testing.T) {
    tmp := t.TempDir()
    trace := filepath.Join(tmp, "trace.txt")
    _ = createTracingYtcast(t, tmp)
    t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
    t.Setenv("TRACE_PATH", trace)

    mux := NewServer(t.TempDir(), "")

    // Set the code
    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/ytcast/set-code?code=123456789012", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 204 {
        t.Fatalf("expected 204, got %d; body=%s", rr.Code, rr.Body.String())
    }

    // Trigger play
    rr = httptest.NewRecorder()
    req = httptest.NewRequest("GET", "/play?url=https://youtu.be/abc123", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 204 {
        t.Fatalf("expected 204 from play, got %d; body=%s", rr.Code, rr.Body.String())
    }

    // Inspect trace for -d <code>
    data, err := os.ReadFile(trace)
    if err != nil {
        t.Fatalf("reading trace: %v", err)
    }
    got := string(data)
    if !strings.Contains(got, "-d 123456789012") {
        t.Fatalf("expected args to include -d 123456789012, got %q", got)
    }
    if !strings.Contains(got, "https://youtu.be/abc123") {
        t.Fatalf("expected args to include the URL, got %q", got)
    }
}
