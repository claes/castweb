package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPairPageRenders(t *testing.T) {
	mux := NewServer(t.TempDir(), "living-room", "", "")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pair/", nil)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}

	body := rr.Body.String()
	checks := []string{
		"Pair and select playback targets",
		`hx-get="/ytcast/pair"`,
		`hx-get="/ytcast/list"`,
		"/ytcast/set-code",
		"living-room",
	}
	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Fatalf("expected body to contain %q", check)
		}
	}
}

func TestPairPageRedirectsWithoutTrailingSlash(t *testing.T) {
	mux := NewServer(t.TempDir(), "", "", "")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pair", nil)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", rr.Code)
	}
	if location := rr.Header().Get("Location"); location != "/pair/" {
		t.Fatalf("expected redirect to /pair/, got %q", location)
	}
}
