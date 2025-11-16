//go:build integration

package http

import (
    "net/http/httptest"
    "os"
    "runtime"
    "testing"
)

func TestYtcastList_Success(t *testing.T) {
    tmp := t.TempDir()
    // Create a fake ytcast that prints two devices
    exe := createFakeYtcast(t, tmp)
    // Overwrite with content that prints devices
    if runtime.GOOS == "windows" {
        _ = os.WriteFile(exe, []byte("@echo off\r\necho device-one\r\necho device-two\r\nexit /b 0\r\n"), 0o755)
    } else {
        _ = os.WriteFile(exe, []byte("#!/bin/sh\necho device-one\necho device-two\n"), 0o755)
    }
    t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

    mux := NewServer(t.TempDir(), "")
    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/ytcast/list", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 200 {
        t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
    }
    got := rr.Body.String()
    if got == "" || (got != "device-one\ndevice-two\n" && got != "device-one\r\ndevice-two\r\n") {
        t.Fatalf("unexpected body: %q", got)
    }
}

func TestYtcastList_Failure(t *testing.T) {
    tmp := t.TempDir()
    exe := createFakeYtcast(t, tmp)
    // Overwrite with non-zero exit
    if runtime.GOOS == "windows" {
        _ = os.WriteFile(exe, []byte("@echo off\r\necho error 1>&2\r\nexit /b 1\r\n"), 0o755)
    } else {
        _ = os.WriteFile(exe, []byte("#!/bin/sh\necho error 1>&2\nexit 1\n"), 0o755)
    }
    t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

    mux := NewServer(t.TempDir(), "")
    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/ytcast/list", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 500 {
        t.Fatalf("expected 500, got %d; body=%s", rr.Code, rr.Body.String())
    }
}
