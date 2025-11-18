package http

import (
    "context"
    "net/http/httptest"
    "net/url"
    "strings"
    "testing"
)

func TestSVTPlay_ForwardsToEndpoint_OK(t *testing.T) {
    // Stub svtDoRequest to avoid network
    prev := svtDoRequest
    defer func() { svtDoRequest = prev }()
    called := false
    var gotReqURL string
    svtDoRequest = func(ctx context.Context, requestURL string) (int, error) {
        called = true
        gotReqURL = requestURL
        return 200, nil
    }

    mux := NewServer(t.TempDir(), "", "", "http://example.local/play")

    rr := httptest.NewRecorder()
    u := url.QueryEscape("https://www.svtplay.se/video/abc?video=visa")
    req := httptest.NewRequest("GET", "/play?type=svtplay&url="+u, nil)
    mux.ServeHTTP(rr, req)

    if rr.Code != 204 {
        t.Fatalf("expected 204, got %d; body=%s", rr.Code, rr.Body.String())
    }
    if !called {
        t.Fatalf("expected svtDoRequest to be called")
    }
    expected := "http://example.local/play?url=" + url.QueryEscape("https://www.svtplay.se/video/abc?video=visa")
    if gotReqURL != expected {
        t.Fatalf("wrong request URL: got %q want %q", gotReqURL, expected)
    }
}

func TestSVTPlay_ForwardsToEndpoint_Fails(t *testing.T) {
    prev := svtDoRequest
    defer func() { svtDoRequest = prev }()
    svtDoRequest = func(ctx context.Context, requestURL string) (int, error) { return 500, nil }
    mux := NewServer(t.TempDir(), "", "", "http://example.local/play")
    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/play?type=svtplay&url="+url.QueryEscape("https://www.svtplay.se/x"), nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 502 {
        t.Fatalf("expected 502 on endpoint failure, got %d", rr.Code)
    }
}

func TestSVTPlay_MissingURL_BadRequest(t *testing.T) {
    mux := NewServer(t.TempDir(), "", "", "http://example.local/play")
    rr := httptest.NewRecorder()
    req := httptest.NewRequest("GET", "/play?type=svtplay", nil)
    mux.ServeHTTP(rr, req)
    if rr.Code != 400 {
        t.Fatalf("expected 400 for missing url, got %d", rr.Code)
    }
    if msg := strings.TrimSpace(rr.Body.String()); msg == "" {
        t.Fatalf("expected error body, got empty")
    }
}
