package http

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	nethttp "net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/claes/ytplv/internal/browse"
	"github.com/claes/ytplv/internal/store"
	"sync"
)

type server struct {
	root         string
	tpl          *template.Template
	pairTpl      *template.Template
	ytcastDevice string
	ytcastCode   string
	stateDir     string
	svtEndpoint  string
	mu           sync.RWMutex
}

const execTimeout = 15 * time.Second

// svtDoRequest is used by handlePlay to forward SVT URLs to an external endpoint.
// It is declared as a variable to allow tests to stub it out without network access.
var svtDoRequest = func(ctx context.Context, requestURL string) (int, error) {
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, requestURL, nil)
	if err != nil {
		return 0, err
	}
	client := &nethttp.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

// NewServer creates an HTTP handler for browsing video metadata rooted at dir.
func NewServer(root string, ytcastDevice string, stateDir string, svtEndpoint string) nethttp.Handler {
	tpl := newBrowseTemplate()
	pairTpl := newPairTemplate()
	s := &server{root: root, tpl: tpl, pairTpl: pairTpl, ytcastDevice: ytcastDevice, stateDir: stateDir, svtEndpoint: svtEndpoint}
	// Load state if present; do not create directories/files here (packaging/systemd owns it).
	if stateDir != "" {
		statePath := filepath.Join(stateDir, "state.json")
		if st, err := store.LoadState(statePath); err != nil {
			slog.Warn("state load failed", "path", statePath, "err", err)
		} else if st.YtcastCode != "" {
			s.ytcastCode = st.YtcastCode
			slog.Info("state loaded", "path", statePath)
		}
	}
	mux := nethttp.NewServeMux()
	mux.HandleFunc("/", s.handleBrowse)
	mux.HandleFunc("/pair", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.URL.Path != "/pair" {
			s.handlePairPage(w, r)
			return
		}
		nethttp.Redirect(w, r, "/pair/", nethttp.StatusMovedPermanently)
	})
	mux.HandleFunc("/pair/", s.handlePairPage)
	mux.HandleFunc("/health", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		HealthHandler().ServeHTTP(w, r)
	})
	mux.HandleFunc("/play", s.handlePlay)
	mux.HandleFunc("/queue", s.handleQueue)
	mux.HandleFunc("/ytcast/pair", s.handleYtcastPair)
	mux.HandleFunc("/ytcast/set-code", s.handleYtcastSetCode)
	mux.HandleFunc("/ytcast/list", s.handleYtcastList)
	return mux
}

func (s *server) handlePlay(w nethttp.ResponseWriter, r *nethttp.Request) {
	typ, u, ok := parsePlayParams(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	switch typ {
	case "svtplay":
		if code, err := s.playSVT(ctx, u); err != nil {
			httpError(w, code, err.Error())
			return
		}
		w.WriteHeader(nethttp.StatusNoContent)
		return
	default:
		if code, err := s.playYouTube(ctx, u); err != nil {
			httpError(w, code, err.Error())
			return
		}
		w.WriteHeader(nethttp.StatusNoContent)
		return
	}
}

// handleQueue queues a YouTube URL on the configured device (ytcast -a).
// Only YouTube URLs are supported; SVT is not applicable.
func (s *server) handleQueue(w nethttp.ResponseWriter, r *nethttp.Request) {
	typ, u, ok := parsePlayParams(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	// Only allow YouTube for queuing
	switch typ {
	case "", "youtube":
		if code, err := s.queueYouTube(ctx, u); err != nil {
			httpError(w, code, err.Error())
			return
		}
		w.WriteHeader(nethttp.StatusNoContent)
		return
	default:
		httpError(w, nethttp.StatusBadRequest, "queue supported only for youtube")
		return
	}
}

// parsePlayParams parses form/query and extracts type and url.
// Writes a 400 error on failure and returns ok=false.
func parsePlayParams(w nethttp.ResponseWriter, r *nethttp.Request) (typ, u string, ok bool) {
	if err := r.ParseForm(); err != nil {
		slog.Warn("/play parse error", "err", err)
		httpError(w, nethttp.StatusBadRequest, "invalid form")
		return "", "", false
	}
	typ = r.FormValue("type")
	if typ == "" {
		typ = r.URL.Query().Get("type")
	}
	u = r.FormValue("url")
	if u == "" {
		u = r.URL.Query().Get("url")
	}
	if typ == "svtplay" {
		if u == "" {
			slog.Warn("/play svtplay missing url")
			httpError(w, nethttp.StatusBadRequest, "missing url")
			return "", "", false
		}
		return typ, u, true
	}
	if u == "" {
		slog.Warn("/play missing url")
		httpError(w, nethttp.StatusBadRequest, "missing url")
		return "", "", false
	}
	return typ, u, true
}

// playSVT forwards the SVT URL to the configured endpoint. Returns an HTTP status
// code to send on error.
func (s *server) playSVT(ctx context.Context, svtURL string) (int, error) {
	endpoint := s.svtEndpoint
	if endpoint == "" {
		endpoint = "http://localhost:18492/play"
	}
	// Compose full request URL including encoded svtURL
	ep, err := url.Parse(endpoint)
	if err != nil {
		slog.Error("/play svtplay invalid endpoint", "endpoint", endpoint, "err", err)
		return nethttp.StatusBadGateway, fmt.Errorf("invalid svt endpoint")
	}
	q := ep.Query()
	q.Set("url", svtURL)
	ep.RawQuery = q.Encode()
	reqURL := ep.String()
	slog.Info("/play svtplay request", "url", reqURL)

	// Perform request via indirection (allows tests to stub)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	status, err := svtDoRequest(ctx, reqURL)
	if err != nil {
		slog.Error("/play svtplay call failed", "url", reqURL, "err", err)
		return nethttp.StatusBadGateway, fmt.Errorf("svt call failed")
	}
	if status < 200 || status >= 300 {
		slog.Warn("/play svtplay non-2xx", "status", status, "url", reqURL)
		return nethttp.StatusBadGateway, fmt.Errorf("svt endpoint error")
	}
	slog.Info("/play svtplay forwarded", "url", reqURL)
	return 0, nil
}

// playYouTube validates the URL and invokes ytcast with the configured device.
// Returns an HTTP status code to send on error.
func (s *server) playYouTube(ctx context.Context, u string) (int, error) {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		slog.Warn("/play invalid url", "url", u, "err", err)
		return nethttp.StatusBadRequest, fmt.Errorf("invalid url")
	}
	host := strings.ToLower(parsed.Host)
	if !isYouTubeHost(host) {
		slog.Warn("/play unsupported url host", "host", host)
		return nethttp.StatusBadRequest, fmt.Errorf("unsupported url")
	}
	device := s.getYtcastDevice()
	if device == "" {
		slog.Warn("/play device not configured", "hint", "set -ytcast, YTCAST_DEVICE, or /ytcast/set-code")
		return nethttp.StatusBadRequest, fmt.Errorf("ytcast device not configured")
	}
	// Execute ytcast with the provided URL
	cctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	bin, _ := exec.LookPath("ytcast")
	args := []string{"-d", device, u}
	cmd := exec.CommandContext(cctx, "ytcast", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Log full command line with quoting for troubleshooting
	qargs := make([]string, 0, len(args))
	for _, a := range args {
		qargs = append(qargs, fmt.Sprintf("%q", a))
	}
	prog := bin
	if prog == "" {
		prog = "ytcast"
	}
	slog.Info("/play casting", "device", device, "url", u)
	slog.Debug("/play exec", "prog", prog, "args", strings.Join(qargs, " "))
	if err := cmd.Run(); err != nil {
		exitCode := 0
		if ee, ok := err.(*exec.ExitError); ok && ee.ProcessState != nil {
			exitCode = ee.ProcessState.ExitCode()
		}
		outStr := strings.TrimSpace(stdout.String())
		errStr := strings.TrimSpace(stderr.String())
		slog.Error("/play ytcast failed", "err", err, "exit", exitCode, "stdout", outStr, "stderr", errStr)
		return nethttp.StatusInternalServerError, fmt.Errorf("failed to cast")
	}
	return 0, nil
}

func isYouTubeHost(host string) bool {
	return strings.HasSuffix(host, "youtube.com") || strings.HasSuffix(host, "youtu.be")
}

// queueYouTube validates the URL and invokes ytcast with -a to add to queue.
// Returns an HTTP status code to send on error.
func (s *server) queueYouTube(ctx context.Context, u string) (int, error) {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		slog.Warn("/queue invalid url", "url", u, "err", err)
		return nethttp.StatusBadRequest, fmt.Errorf("invalid url")
	}
	host := strings.ToLower(parsed.Host)
	if !isYouTubeHost(host) {
		slog.Warn("/queue unsupported url host", "host", host)
		return nethttp.StatusBadRequest, fmt.Errorf("unsupported url")
	}
	device := s.getYtcastDevice()
	if device == "" {
		slog.Warn("/queue device not configured", "hint", "set -ytcast, YTCAST_DEVICE, or /ytcast/set-code")
		return nethttp.StatusBadRequest, fmt.Errorf("ytcast device not configured")
	}
	// Execute ytcast with the provided URL and add flag
	cctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	bin, _ := exec.LookPath("ytcast")
	args := []string{"-d", device, "-a", u}
	cmd := exec.CommandContext(cctx, "ytcast", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Log full command line with quoting for troubleshooting
	qargs := make([]string, 0, len(args))
	for _, a := range args {
		qargs = append(qargs, fmt.Sprintf("%q", a))
	}
	prog := bin
	if prog == "" {
		prog = "ytcast"
	}
	slog.Info("/queue casting", "device", device, "url", u)
	slog.Debug("/queue exec", "prog", prog, "args", strings.Join(qargs, " "))
	if err := cmd.Run(); err != nil {
		exitCode := 0
		if ee, ok := err.(*exec.ExitError); ok && ee.ProcessState != nil {
			exitCode = ee.ProcessState.ExitCode()
		}
		outStr := strings.TrimSpace(stdout.String())
		errStr := strings.TrimSpace(stderr.String())
		slog.Error("/queue ytcast failed", "err", err, "exit", exitCode, "stdout", outStr, "stderr", errStr)
		return nethttp.StatusInternalServerError, fmt.Errorf("failed to cast")
	}
	return 0, nil
}

func (s *server) handleBrowse(w nethttp.ResponseWriter, r *nethttp.Request) {
	rel := decodeRelPath(requestRelPath(r.URL.Path))
	if s.serveImage(w, r, rel) {
		return
	}

	listing, err := browse.BuildListing(s.root, rel)
	if err != nil {
		httpError(w, nethttp.StatusNotFound, "unable to read path")
		return
	}

	// Ensure directory paths have a trailing slash so that relative URLs
	// within the page (e.g., image src="file.jpg") resolve under the
	// directory rather than from the root. Redirect /foo to /foo/.
	if listing.Path != "" && !strings.HasSuffix(r.URL.Path, "/") {
		u := *r.URL
		u.Path = r.URL.Path + "/"
		nethttp.Redirect(w, r, u.String(), nethttp.StatusMovedPermanently)
		return
	}
	data := browsePageFromRequest(r, listing, rel)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tpl.Execute(w, data)
}

func (s *server) handlePairPage(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.URL.Path != "/pair/" {
		httpError(w, nethttp.StatusNotFound, "not found")
		return
	}
	data := pairPageData{ActiveDevice: s.getYtcastDevice()}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.pairTpl.Execute(w, data)
}

func httpError(w nethttp.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(msg))
}

// getYtcastDevice returns the active device to pass to ytcast -d.
// If a code has been set via /ytcast/set-code, that takes precedence;
// otherwise the configured ytcastDevice from startup is used.
func (s *server) getYtcastDevice() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ytcastCode != "" {
		return s.ytcastCode
	}
	return s.ytcastDevice
}

// handleYtcastPair validates a 12-digit pairing code and invokes
// `ytcast -pair <code>`. Returns 204 on success, 400 on validation error,
// and 500 on execution failure.
func (s *server) handleYtcastPair(w nethttp.ResponseWriter, r *nethttp.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		slog.Warn("/ytcast/pair missing code")
		httpError(w, nethttp.StatusBadRequest, "missing code")
		return
	}
	if len(code) != 12 {
		slog.Warn("/ytcast/pair invalid code length", "code", code)
		httpError(w, nethttp.StatusBadRequest, "code must be 12 digits")
		return
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			slog.Warn("/ytcast/pair non-digit in code", "code", code)
			httpError(w, nethttp.StatusBadRequest, "code must be 12 digits")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), execTimeout)
	defer cancel()
	// Run: ytcast -pair <code>
	cmd := exec.CommandContext(ctx, "ytcast", "-pair", code)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	slog.Info("/ytcast/pair exec", "code", code)
	if err := cmd.Run(); err != nil {
		outStr := strings.TrimSpace(stdout.String())
		errStr := strings.TrimSpace(stderr.String())
		slog.Error("/ytcast/pair failed", "err", err, "stdout", outStr, "stderr", errStr)
		httpError(w, nethttp.StatusInternalServerError, "failed to pair")
		return
	}
	slog.Info("/ytcast/pair success", "code", code)
	w.WriteHeader(nethttp.StatusNoContent)
}

// handleYtcastList invokes `ytcast -l` and writes its stdout as text/plain.
// Returns 200 on success, 500 on failure.
func (s *server) handleYtcastList(w nethttp.ResponseWriter, r *nethttp.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), execTimeout)
	defer cancel()
	defer cancel()
	cmd := exec.CommandContext(ctx, "ytcast", "-l")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	slog.Info("/ytcast/list exec")
	if err := cmd.Run(); err != nil {
		outStr := strings.TrimSpace(stdout.String())
		errStr := strings.TrimSpace(stderr.String())
		slog.Error("/ytcast/list failed", "err", err, "stdout", outStr, "stderr", errStr)
		httpError(w, nethttp.StatusInternalServerError, "failed to list devices")
		return
	}
	slog.Info("/ytcast/list success", "bytes", stdout.Len())
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	_, _ = w.Write(stdout.Bytes())
}

// handleYtcastSetCode stores a code (as-is) to be used as the device
// argument for subsequent `ytcast -d` calls (e.g., in /play).
// Returns 204 on success, 400 when missing the code parameter.
func (s *server) handleYtcastSetCode(w nethttp.ResponseWriter, r *nethttp.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		slog.Warn("/ytcast/set-code missing code")
		httpError(w, nethttp.StatusBadRequest, "missing code")
		return
	}
	s.mu.Lock()
	s.ytcastCode = code
	s.mu.Unlock()
	slog.Info("/ytcast/set-code set", "code", code)
	if s.stateDir != "" {
		statePath := filepath.Join(s.stateDir, "state.json")
		if err := store.SaveState(statePath, store.State{YtcastCode: code}); err != nil {
			slog.Error("/ytcast/set-code persist failed", "err", err)
		} else {
			slog.Info("/ytcast/set-code persisted", "path", statePath)
		}
	}
	w.WriteHeader(nethttp.StatusNoContent)
}
