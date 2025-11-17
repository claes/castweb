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
    stdpath "path"
    "strconv"
    "strings"
    "time"

    "github.com/claes/ytplv/internal/browse"
    "github.com/claes/ytplv/internal/store"
    "sync"
)

type server struct {
    root         string
    tpl          *template.Template
    ytcastDevice string
    ytcastCode   string
    stateDir     string
    mu           sync.RWMutex
}

const execTimeout = 15 * time.Second

// NewServer creates an HTTP handler for browsing video metadata rooted at dir.
func NewServer(root string, ytcastDevice string, stateDir string) nethttp.Handler {
    // simple HTML template without external assets
    tpl := template.Must(template.New("page").Funcs(template.FuncMap{
        "join": strings.Join,
        "q":    url.QueryEscape,
        // iso returns date only (YYYY-MM-DD)
        "iso":  func(t time.Time) string { return t.Format("2006-01-02") },
        "pjoin": func(a, b string) string {
            if a == "" {
                return b
            }
            if b == "" {
                return a
            }
            return a + "/" + b
        },
    }).Parse(pageTpl))
    s := &server{root: root, tpl: tpl, ytcastDevice: ytcastDevice, stateDir: stateDir}
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
    mux.HandleFunc("/health", func(w nethttp.ResponseWriter, r *nethttp.Request) {
        HealthHandler().ServeHTTP(w, r)
    })
    mux.HandleFunc("/play", s.handlePlay)
    mux.HandleFunc("/ytcast/pair", s.handleYtcastPair)
    mux.HandleFunc("/ytcast/set-code", s.handleYtcastSetCode)
    mux.HandleFunc("/ytcast/list", s.handleYtcastList)
    return mux
}

func (s *server) handlePlay(w nethttp.ResponseWriter, r *nethttp.Request) {
	// Accept POST (htmx) or GET. Expect parameter "url" (playable URL).
    if err := r.ParseForm(); err != nil {
        slog.Warn("/play parse error", "err", err)
        httpError(w, nethttp.StatusBadRequest, "invalid form")
        return
    }
    typ := r.FormValue("type")
    if typ == "" {
        typ = r.URL.Query().Get("type")
    }
    u := r.FormValue("url")
    if u == "" {
        u = r.URL.Query().Get("url")
    }
    if typ == "svtplay" {
        // For SVT stream type, the client constructs the URL. Just log and return.
        slog.Info("/play svtplay", "url", u)
        w.WriteHeader(nethttp.StatusNoContent)
        return
    }
    if u == "" {
        slog.Warn("/play missing url")
		httpError(w, nethttp.StatusBadRequest, "missing url")
		return
	}
	// Only support YouTube URLs for now.
    parsed, err := url.Parse(u)
    if err != nil || parsed.Scheme == "" || parsed.Host == "" {
        slog.Warn("/play invalid url", "url", u, "err", err)
        httpError(w, nethttp.StatusBadRequest, "invalid url")
        return
    }
    host := strings.ToLower(parsed.Host)
    if !isYouTubeHost(host) {
        slog.Warn("/play unsupported url host", "host", host)
        httpError(w, nethttp.StatusBadRequest, "unsupported url")
        return
    }
    device := s.getYtcastDevice()
    if device == "" {
        slog.Warn("/play device not configured", "hint", "set -ytcast, YTCAST_DEVICE, or /ytcast/set-code")
        httpError(w, nethttp.StatusBadRequest, "ytcast device not configured")
        return
    }
    // Execute ytcast with the provided URL
    ctx, cancel := context.WithTimeout(r.Context(), execTimeout)
    defer cancel()
    bin, _ := exec.LookPath("ytcast")
    args := []string{"-d", device, u}
    cmd := exec.CommandContext(ctx, "ytcast", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Log full command line with quoting for troubleshooting
	q := make([]string, 0, len(args))
	for _, a := range args {
		q = append(q, fmt.Sprintf("%q", a))
	}
	prog := bin
	if prog == "" {
		prog = "ytcast"
	}
    slog.Info("/play casting", "device", device, "url", u)
    slog.Debug("/play exec", "prog", prog, "args", strings.Join(q, " "))
	if err := cmd.Run(); err != nil {
		exitCode := 0
		if ee, ok := err.(*exec.ExitError); ok && ee.ProcessState != nil {
			exitCode = ee.ProcessState.ExitCode()
		}
		outStr := strings.TrimSpace(stdout.String())
		errStr := strings.TrimSpace(stderr.String())
        slog.Error("/play ytcast failed", "err", err, "exit", exitCode, "stdout", outStr, "stderr", errStr)
		httpError(w, nethttp.StatusInternalServerError, "failed to cast")
		return
	}
	w.WriteHeader(nethttp.StatusNoContent)
}

func isYouTubeHost(host string) bool {
    return strings.HasSuffix(host, "youtube.com") || strings.HasSuffix(host, "youtu.be")
}

func (s *server) handleBrowse(w nethttp.ResponseWriter, r *nethttp.Request) {
	// Derive relative path from URL path ("/" => "")
    p := stdpath.Clean(r.URL.Path)
    if p == "/" {
        p = ""
    } else {
        p = strings.TrimPrefix(p, "/")
    }
    // Unescape each path segment so filesystem lookups work with spaces and UTF-8.
    rel := p
    if rel != "" {
        segs := strings.Split(rel, "/")
        for i, s := range segs {
            if u, err := url.PathUnescape(s); err == nil {
                segs[i] = u
            }
        }
        rel = strings.Join(segs, "/")
    }
	// sanitize: ensure within root
	listing, err := browse.BuildListing(s.root, rel)
	if err != nil {
		httpError(w, nethttp.StatusNotFound, "unable to read path")
		return
	}
	// Pagination: 100 items per page over listing.Entries
	const limit = 100
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	total := len(listing.Entries)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	// slice entries for this page
	listing.Entries = listing.Entries[start:end]

	// Build prev/next URLs
	// base path retains the path segment; we only manipulate the page query param
	base := "/"
	if rel != "" {
		// preserve encoding of path segments
		var parts []string
		for _, seg := range strings.Split(rel, "/") {
			if seg == "" {
				continue
			}
			parts = append(parts, url.PathEscape(seg))
		}
		base = "/" + strings.Join(parts, "/")
	}
	q := r.URL.Query()
	hasPrev := page > 1
	hasNext := end < total
	prevURL := ""
	nextURL := ""
	if hasPrev {
		q.Set("page", strconv.Itoa(page-1))
		prevURL = base + "?" + q.Encode()
	}
	if hasNext {
		q.Set("page", strconv.Itoa(page+1))
		nextURL = base + "?" + q.Encode()
	}
	data := struct {
		Listing    interface{}
		Page       int
		HasPrev    bool
		HasNext    bool
		PrevURL    string
		NextURL    string
		Path       string
		ParentPath string
		Entries    interface{}
		Breadcrumbs interface{}
	}{
		Listing:    listing,
		Page:       page,
		HasPrev:    hasPrev,
		HasNext:    hasNext,
		PrevURL:    prevURL,
		NextURL:    nextURL,
		Path:       listing.Path,
		ParentPath: listing.ParentPath,
		Entries:    listing.Entries,
	}
	// Build breadcrumb trail: Root + each path segment
	{
		// crumb element type: Name, Href, Current
		type crumb struct{ Name, Href string; Current bool }
		var crumbs []crumb
		// Root crumb
		if listing.Path == "" {
			crumbs = append(crumbs, crumb{Name: "Root", Href: "/", Current: true})
		} else {
			crumbs = append(crumbs, crumb{Name: "Root", Href: "/", Current: false})
			parts := strings.Split(listing.Path, "/")
			var acc string
			for i, seg := range parts {
				if seg == "" { continue }
				if acc == "" {
					acc = "/" + url.PathEscape(seg)
				} else {
					acc = acc + "/" + url.PathEscape(seg)
				}
				crumbs = append(crumbs, crumb{Name: seg, Href: acc, Current: i == len(parts)-1})
			}
		}
		data.Breadcrumbs = crumbs
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tpl.Execute(w, data)
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

const pageTpl = `<!doctype html>
<html lang="en">
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>castweb</title>
<style>
/* Ensure padding/borders are included in element width to prevent overflow */
*, *::before, *::after { box-sizing: border-box }
html, body { height: 100%; }
/* Theme variables */
:root{
  --bg: #ffffff;
  --text: #111111;
  --panel-bg: #ffffff;
  --border: #dddddd;
  --muted: #666666;
  --hover: #f6f6f6;
  --active: #eef5ff;
  --link: #0a53c6;
  --link-hover: #0842a3;
}
@media (prefers-color-scheme: dark) {
  :root{
    --bg: #0f1115;
    --text: #e6e6e6;
    --panel-bg: #161a20;
    --border: #2a2f3a;
    --muted: #9aa4b2;
    --hover: #1f2430;
    --active: #223049;
    --link: #8ab4ff;
    --link-hover: #a6c8ff;
  }
}
:root[data-theme='light']{
  --bg: #ffffff;
  --text: #111111;
  --panel-bg: #ffffff;
  --border: #dddddd;
  --muted: #666666;
  --hover: #f6f6f6;
  --active: #eef5ff;
  --link: #0a53c6;
  --link-hover: #0842a3;
}
:root[data-theme='dark']{
  --bg: #0f1115;
  --text: #e6e6e6;
  --panel-bg: #161a20;
  --border: #2a2f3a;
  --muted: #9aa4b2;
  --hover: #1f2430;
  --active: #223049;
  --link: #8ab4ff;
  --link-hover: #a6c8ff;
}
body{font-family:system-ui,-apple-system,Segoe UI,Roboto;margin:0;padding:16px 24px;background:var(--bg);color:var(--text)}
body{display:flex;flex-direction:column;min-height:100vh}
body{font-size:19px}
header{display:flex;justify-content:space-between;align-items:center;margin-bottom:0.6rem}
header .left{display:flex;gap:8px;align-items:center}
header .up-link{display:inline-flex;align-items:center;justify-content:center;border:1px solid var(--border);border-radius:6px;padding:4px 8px;color:var(--link);text-decoration:none}
header .up-link:hover, header .up-link:focus{color:var(--link-hover);background:var(--hover)}
header a{ color: var(--link); text-decoration: none; }
header a:hover, header a:focus{ color: var(--link-hover); text-decoration: underline; }
ul{list-style:none;padding:0;margin:0}
.layout{display:grid;grid-template-columns:1fr;gap:26px;align-items:start}
.layout{flex:1 1 auto;min-height:0}
.layout{grid-auto-rows:1fr}
.layout > * { min-height: 0; }
.layout, section { width: 100%; }
section { flex: 1 1 auto; display: flex; flex-direction: column; min-height: 0; }
.panel{border:1px solid var(--border);border-radius:8px;padding:14px;background:var(--panel-bg)}
.list{height:100%;overflow:auto}
.dirs .dir-item{padding:6px;border-radius:6px;cursor:pointer;margin-bottom:4px}
.dirs .dir-item:hover{background:var(--hover)}
.dirs .dir-item.active{background:var(--active)}
.list .item{display:flex;gap:12px;align-items:center;padding:10px;border-radius:6px;cursor:pointer;font-size:1.2rem}
.list .item:hover{background:var(--hover)}
.list .item.active{background:var(--active)}
.thumb{width:112px;height:84px;object-fit:cover;border-radius:4px;flex:0 0 auto}
.title{font-weight:600;font-size:1.3rem}
 h2{font-size:1.5rem}
.details img{max-width:100%;height:auto;border-radius:6px}
.muted, small{color:var(--muted)}
/* Top actions */
.header-actions{display:flex;gap:10px;align-items:center}
.theme-toggle{border:1px solid var(--border);background:transparent;color:var(--text);padding:6px 12px;border-radius:16px;cursor:pointer}
.theme-toggle:focus{outline:2px solid var(--active)}
/* Modal overlay */
.overlay-backdrop{position:fixed;inset:0;background:rgba(0,0,0,.5);display:none;align-items:center;justify-content:center;}
.overlay-backdrop[aria-hidden="false"]{display:flex}
.overlay{background:var(--panel-bg);border:1px solid var(--border);border-radius:8px;box-shadow:0 10px 30px rgba(0,0,0,.25);max-width:90vw;max-height:90vh;width:90vw;padding:18px;overflow:auto;color:var(--text);font-size:1.2rem}
.overlay header{display:flex;flex-direction:column;align-items:flex-start;gap:8px;margin-bottom:8px}
.overlay .actions{display:flex;gap:10px;margin-top:12px}
.overlay .actions button{padding:6px 12px;font-size:1.15rem}
/* Responsive reflow: on small viewports, stack details above list.
   In this mode, list uses 75% width and details 25%. */
@media (max-width: 768px) {
  html, body{ padding:0; margin:0; overflow-x:hidden; }
  body{ padding:8px 12px; }
  header{ margin-bottom: .6rem; }
  .layout{
    grid-template-columns: 1fr;
    grid-template-areas: "list";
    gap: 8px;
    width: 100%;
  }
  .panel{ padding: 8px; }
  /* Hide preview on mobile */
  .details{ display: none; }
  .list{
    grid-area: list;
    width: 100%;
    max-width: 100%;
    margin: 0;
    max-height: none;
    overflow: auto;
  }
  .title{ word-break: break-word; overflow-wrap: anywhere; }
}
</style>
<header>
  <div class="left">
    {{if ne .Path ""}}<a class="up-link" href="/{{.ParentPath}}" aria-label="Up one level" title="Up one level">⬆</a>{{end}}
    <nav class="breadcrumb" aria-label="Breadcrumb">
      {{$n := len .Breadcrumbs}}
      {{range $i, $c := .Breadcrumbs}}
        {{if gt $i 0}}<span class="muted"> / </span>{{end}}
        {{if $c.Current}}<strong>{{$c.Name}}</strong>{{else}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}
      {{end}}
    </nav>
  </div>
  <div class="header-actions">
    <button id="theme-toggle" class="theme-toggle" type="button" aria-pressed="false" title="Toggle theme">🌓</button>
  </div>
</header>

<section>
  {{if .Entries}}
  <div class="layout">
    <div class="panel list" id="list">
      <ul role="listbox" aria-label="Items">
      {{if .HasPrev}}
        <li class="item" role="option" aria-selected="false" tabindex="0"
            data-kind="nav"
            data-title="Previous"
            data-href="{{.PrevURL}}">
          <div class="title">⟵ Previous</div>
        </li>
      {{end}}
      {{range .Entries}}
        {{if eq .Kind "dir"}}
          <li class="item" role="option" aria-selected="false" tabindex="0"
              data-kind="dir"
              data-title="{{.Name}}"
              data-path="{{.Path}}">
            <div class="title">📁 {{.Name}}</div>
          </li>
        {{else}}
          <li class="item" role="option" aria-selected="false" tabindex="0"
              data-kind="video"
              data-title="{{if .Video.Title}}{{.Video.Title}}{{else}}{{.Video.Name}}{{end}}"
              data-type="{{.Video.Type}}"
              data-id="{{.Video.VideoID}}"
              data-date="{{iso .ModTime}}"
              data-thumb="{{.Video.ThumbURL}}"
              data-tags="{{join .Video.Tags ", "}}"
              data-plot="{{.Video.Plot}}"
              >
            {{if .Video.ThumbURL}}<img class="thumb" src="{{.Video.ThumbURL}}" alt="thumb">{{end}}
            <div class="title">{{if .Video.Title}}{{.Video.Title}}{{else}}{{.Video.Name}}{{end}}</div>
          </li>
        {{end}}
      {{end}}
      {{if .HasNext}}
        <li class="item" role="option" aria-selected="false" tabindex="0"
            data-kind="nav"
            data-title="Next"
            data-href="{{.NextURL}}">
          <div class="title">Next ⟶</div>
        </li>
      {{end}}
      </ul>
    </div>
  </div>
  {{else}}
    <small>No items found in this folder</small>
  {{end}}
</section>

<!-- Overlay for playing a selected item -->
<div id="overlay-backdrop" class="overlay-backdrop" aria-hidden="true">
  <div class="overlay" role="dialog" aria-modal="true" aria-labelledby="overlay-title" tabindex="-1">
    <header>
      <div id="overlay-actions" class="actions" aria-label="Actions"></div>
      <div id="overlay-title" class="title"></div>
    </header>
    <div id="overlay-body">
      <p class="muted">No item selected.</p>
    </div>
  </div>
  <div aria-hidden="true"></div>
  <!-- focus sentinel after dialog -->
</div>

<script src="https://unpkg.com/htmx.org@1.9.12"></script>
<script>
(function(){
  // Theme handling: default to system; allow user override via toggle
  var root = document.documentElement;
  var media = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)');
  function applyStoredTheme(){
    var t = localStorage.getItem('theme'); // 'light' | 'dark' | null
    if (t === 'light' || t === 'dark') { root.setAttribute('data-theme', t); }
    else { root.removeAttribute('data-theme'); }
    updateToggleAria();
  }
  function currentIsDark(){
    var t = root.getAttribute('data-theme');
    if (t === 'dark') return true;
    if (t === 'light') return false;
    return !!(media && media.matches);
  }
  function updateToggleAria(){
    var btn = document.getElementById('theme-toggle');
    if (!btn) return;
    btn.setAttribute('aria-pressed', currentIsDark() ? 'true' : 'false');
  }
  function toggleTheme(){
    var explicit = root.getAttribute('data-theme');
    var next;
    if (explicit === 'dark') next = 'light';
    else if (explicit === 'light') next = 'dark';
    else next = currentIsDark() ? 'light' : 'dark';
    root.setAttribute('data-theme', next);
    try { localStorage.setItem('theme', next); } catch (e) {}
    updateToggleAria();
  }
  applyStoredTheme();
  if (media && media.addEventListener) media.addEventListener('change', applyStoredTheme);
  var themeBtn = document.getElementById('theme-toggle');
  if (themeBtn) themeBtn.addEventListener('click', toggleTheme);

  var currentPath = {{printf "%q" .Path}};
  var parentPath = {{printf "%q" .ParentPath}};
  var list = document.getElementById('list');
  var dirsList = null; // deprecated separate folder list
  function esc(s){
    return String(s)
      .replace(/&/g,'&amp;')
      .replace(/</g,'&lt;')
      .replace(/>/g,'&gt;')
      .replace(/"/g,'&quot;')
      .replace(/'/g,'&#39;');
  }
  // Shared metadata helpers for details and overlay
  function getMeta(li){
    var title = li.getAttribute('data-title') || '';
    var id = li.getAttribute('data-id') || '';
    var typ = li.getAttribute('data-type') || 'youtube';
    var url = '';
    if (typ === 'youtube' && id) {
      url = 'https://www.youtube.com/watch?v=' + id;
    } else if (typ === 'svtplay' && id) {
      url = 'https://www.svtplay.se' + id + '?video=visa';
    }
    var thumb = li.getAttribute('data-thumb') || '';
    var tags = li.getAttribute('data-tags') || '';
    var plot = li.getAttribute('data-plot') || '';
    var date = li.getAttribute('data-date') || '';
    return { title: title, id: id, type: typ, url: url, thumb: thumb, tags: tags, plot: plot, date: date };
  }
  function buildMetaHTML(meta, opts){
    opts = opts || {};
    var includeTitle = !!opts.includeTitle;
    var includeActions = !!opts.includeActions;
    var includeCancel = !!opts.includeCancel;
    var includeNav = !!opts.includeNav;
    var playId = opts.playId || '';
    var cancelId = opts.cancelId || '';
    var prevId = opts.prevId || '';
    var nextId = opts.nextId || '';
    var html = '';
    if (includeTitle) {
      html += '<h2 style="margin-top:0">' + esc(meta.title || '') + '</h2>';
    }
    // Actions at the top
    if (includeActions) {
      var vals = esc(JSON.stringify({url: meta.url || '', type: meta.type || '', id: meta.id || ''}));
      html += '<div class="actions">';
      if (includeNav) {
        html += '<button ' + (prevId ? ('id="' + esc(prevId) + '" ') : '') + 'type="button" aria-label="Previous">⟵ Prev</button>';
        html += '<button ' + (nextId ? ('id="' + esc(nextId) + '" ') : '') + 'type="button" aria-label="Next">Next ⟶</button>';
      }
      html += '<button ' + (playId ? ('id="' + esc(playId) + '" ') : '') + 'type="button" hx-post="/play" hx-vals="' + vals + '" hx-trigger="click" hx-swap="none">Play</button>';
      if (includeCancel) {
        html += '<button ' + (cancelId ? ('id="' + esc(cancelId) + '" ') : '') + 'type="button">Cancel</button>';
      }
      html += '</div>';
    }
    if (meta.thumb) html += '<img src="' + esc(meta.thumb) + '" alt="thumb" style="max-width:100%;height:auto;border-radius:6px" />';
    if (meta.url) {
      var u = esc(meta.url);
      html += '<p style="margin-top:8px"><a href="' + u + '" target="_blank" rel="noopener noreferrer">' + u + '</a></p>';
    }
    if (meta.date) {
      html += '<p class="muted">' + esc(meta.date) + '</p>';
    }
    if (meta.plot) html += '<p style="white-space:pre-wrap">' + esc(meta.plot) + '</p>';
    if (meta.tags) html += '<div class="muted" style="margin-top:6px">Tags: ' + esc(meta.tags) + '</div>';
    return html;
  }
  function normalizePath(path){
    var p = String(path == null ? '' : path);
    if (p.length >= 2) {
      var a = p.charAt(0), b = p.charAt(p.length - 1);
      if ((a === '"' && b === '"') || (a === "'" && b === "'")) {
        p = p.slice(1, -1);
      }
    }
    return p;
  }
  // Ensure the selected list item stays vertically centered in the scrollable list
  function centerInList(el){
    if (!list || !el) return;
    var top = 0, n = el;
    while (n && n !== list) { top += n.offsetTop || 0; n = n.offsetParent; }
    var target = top - Math.max(0, (list.clientHeight - el.offsetHeight) / 2);
    var max = Math.max(0, list.scrollHeight - list.clientHeight);
    if (target < 0) target = 0;
    if (target > max) target = max;
    list.scrollTo({ top: target, behavior: 'auto' });
  }
  function navigateTo(path){
    var p = normalizePath(path);
    if (!p) { window.location.href = '/'; return; }
    var parts = p.split('/').filter(Boolean).map(encodeURIComponent);
    window.location.href = '/' + parts.join('/');
  }
  function navigateParent(){
    navigateTo(parentPath);
  }
  function show(li){
    if (!li) return;
    Array.prototype.forEach.call(list.querySelectorAll('.item'), function(n){ n.classList.remove('active'); });
    li.classList.add('active');
    // Update aria-selected for accessibility
    Array.prototype.forEach.call(list.querySelectorAll('.item'), function(n){ n.setAttribute('aria-selected', 'false'); });
    li.setAttribute('aria-selected', 'true');
    centerInList(li);
    // No left details pane anymore; overlay handles details.
  }
  // Overlay helpers
  var overlayBackdrop = document.getElementById('overlay-backdrop');
  var overlay = overlayBackdrop ? overlayBackdrop.querySelector('.overlay') : null;
  var overlayBody = document.getElementById('overlay-body');
  var overlayTitleEl = document.getElementById('overlay-title');
  var overlayClose = document.getElementById('overlay-close');
  var prevFocus = null;
  function openOverlayFor(li, preferred){
    if (!li) return;
    var meta = getMeta(li);
    if (overlayTitleEl) overlayTitleEl.textContent = meta.title || '';
    // Body content without actions; actions rendered in header
    var html = buildMetaHTML(meta, { includeTitle: false, includeActions: false, includeCancel: false, includeNav: false });
    if (overlayBody) {
      overlayBody.innerHTML = html;
      if (window.htmx) { try { htmx.process(overlayBody); } catch (e) {} }
    }
    if (overlayBackdrop) overlayBackdrop.setAttribute('aria-hidden', 'false');
    prevFocus = document.activeElement;
    if (overlay) overlay.focus();
    // Render actions in header
    var actions = document.getElementById('overlay-actions');
    if (actions) {
      var vals = JSON.stringify({url: meta.url || '', type: meta.type || '', id: meta.id || ''});
      var buf = '';
      buf += '<button id="overlay-prev" type="button" aria-label="Previous">⟵ Prev</button>';
      buf += '<button id="overlay-next" type="button" aria-label="Next">Next ⟶</button>';
      buf += '<button id="overlay-play" type="button" hx-post="/play" hx-vals="' + esc(vals) + '" hx-trigger="click" hx-swap="none">Play</button>';
      buf += '<button id="overlay-cancel" type="button">Cancel</button>';
      actions.innerHTML = buf;
      if (window.htmx) { try { htmx.process(actions); } catch (e) {} }
    }
    var overlayCancel = document.getElementById('overlay-cancel');
    if (overlayCancel) overlayCancel.addEventListener('click', closeOverlay);
    // Wire prev/next buttons
    var prevBtn = document.getElementById('overlay-prev');
    var nextBtn = document.getElementById('overlay-next');
    var items = list ? Array.prototype.slice.call(list.querySelectorAll('.item')) : [];
    var current = list ? (list.querySelector('.item.active') || li) : li;
    var idx = items.length ? items.indexOf(current) : -1;
    if (prevBtn) {
      prevBtn.disabled = (idx <= 0);
      prevBtn.addEventListener('click', function(){
        if (!items.length) return;
        var i = items.indexOf(list.querySelector('.item.active') || li);
        if (i > 0) {
          var target = items[i-1];
          show(target); centerInList(target); openOverlayFor(target, 'prev');
        }
      });
    }
    if (nextBtn) {
      nextBtn.disabled = (idx === -1 || idx >= items.length - 1);
      nextBtn.addEventListener('click', function(){
        if (!items.length) return;
        var i = items.indexOf(list.querySelector('.item.active') || li);
        if (i < items.length - 1) {
          var target = items[i+1];
          show(target); centerInList(target); openOverlayFor(target, 'next');
        }
      });
    }
    // Focus the preferred button after render
    var playBtn = document.getElementById('overlay-play');
    var cancelBtn = document.getElementById('overlay-cancel');
    var focusMap = { prev: prevBtn, next: nextBtn, play: playBtn, cancel: cancelBtn };
    var toFocus = preferred && focusMap[preferred] ? focusMap[preferred] : playBtn;
    if (toFocus && !toFocus.disabled) toFocus.focus();
  }
  function closeOverlay(){
    if (overlayBackdrop) overlayBackdrop.setAttribute('aria-hidden', 'true');
    var selected = list && list.querySelector('.item.active');
    if (selected) { centerInList(selected); selected.focus(); }
    else if (prevFocus && prevFocus.focus) { prevFocus.focus(); }
  }
  if (overlayClose) overlayClose.addEventListener('click', closeOverlay);
  if (overlayBackdrop) overlayBackdrop.addEventListener('click', function(e){
    if (e.target === overlayBackdrop) closeOverlay();
  });
  // Robust cancel via event delegation in case content re-renders
  if (overlay) overlay.addEventListener('click', function(e){
    var t = e.target;
    if (t && t.id === 'overlay-cancel') { e.preventDefault(); closeOverlay(); }
  });
  if (overlay) overlay.addEventListener('keydown', function(e){
    // Close on Escape
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); closeOverlay(); return; }
    // Activate Play on Enter/Space
    if (e.key === 'Enter' || e.key === ' ') {
      var active = document.activeElement;
      if (active && active.tagName === 'BUTTON') { e.preventDefault(); e.stopPropagation(); active.click(); return; }
    }
    // Move focus among all action buttons with Left/Right
    if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
      var prevBtn = document.getElementById('overlay-prev');
      var nextBtn = document.getElementById('overlay-next');
      var playBtn = document.getElementById('overlay-play');
      var cancelBtn = document.getElementById('overlay-cancel');
      var buttons = [prevBtn, nextBtn, playBtn, cancelBtn].filter(function(b){ return !!b && !b.disabled; });
      if (buttons.length) {
        e.preventDefault(); e.stopPropagation();
        var active = document.activeElement;
        var idx = Math.max(0, buttons.indexOf(active));
        if (e.key === 'ArrowRight') idx = Math.min(buttons.length - 1, idx + 1);
        else idx = Math.max(0, idx - 1);
        buttons[idx].focus();
        return;
      }
    }
    // Navigate items while overlay is open
    if (!list) return;
    var items = Array.prototype.slice.call(list.querySelectorAll('.item'));
    if (!items.length) return;
    var current = list.querySelector('.item.active') || items[0];
    var idx = items.indexOf(current);
    if (idx === -1) idx = 0;
    var next = null;
    if (e.key === 'ArrowDown') { e.preventDefault(); e.stopPropagation(); if (idx < items.length - 1) next = items[idx+1]; }
    else if (e.key === 'ArrowUp') { e.preventDefault(); e.stopPropagation(); if (idx > 0) next = items[idx-1]; }
    else if (e.key === 'PageDown') { e.preventDefault(); e.stopPropagation(); next = items[Math.min(items.length - 1, idx + 10)]; }
    else if (e.key === 'PageUp') { e.preventDefault(); e.stopPropagation(); next = items[Math.max(0, idx - 10)]; }
    if (next) {
      // Preserve which action button was focused
      var active = document.activeElement;
      var pref = null;
      if (active && active.id === 'overlay-prev') pref = 'prev';
      else if (active && active.id === 'overlay-next') pref = 'next';
      else if (active && active.id === 'overlay-play') pref = 'play';
      else if (active && active.id === 'overlay-cancel') pref = 'cancel';
      show(next); centerInList(next); openOverlayFor(next, pref || 'play');
    }
  });
  if (list) {
    list.addEventListener('click', function(e){
      var li = e.target.closest('.item');
      if (!li) return;
      var kind = li.getAttribute('data-kind') || 'video';
      if (kind === 'dir') {
        navigateTo(li.getAttribute('data-path') || '');
      } else if (kind === 'nav') {
        var href = li.getAttribute('data-href');
        if (href) { window.location.href = href; }
      } else {
        show(li); openOverlayFor(li, 'play');
      }
    });
    list.addEventListener('keydown', function(e){
      if (e.key === 'Enter' || e.key === ' ') {
        var li = e.target.closest('.item');
        if (li) {
          e.preventDefault();
          var kind = li.getAttribute('data-kind') || 'video';
          if (kind === 'dir') { navigateTo(li.getAttribute('data-path') || ''); }
          else if (kind === 'nav') { var href = li.getAttribute('data-href'); if (href) { window.location.href = href; } }
          else { show(li); openOverlayFor(li, 'play'); }
        }
      } else if (e.key === 'PageDown' || e.key === 'PageUp') {
        e.preventDefault();
        var delta = (e.key === 'PageDown' ? 1 : -1) * Math.max(0, list.clientHeight - 40);
        list.scrollBy(0, delta);
        // After scrolling, move selection to first visible item
        var items = Array.prototype.slice.call(list.querySelectorAll('.item'));
        if (!items.length) return;
        var listRect = list.getBoundingClientRect();
        var target = null;
        for (var i = 0; i < items.length; i++) {
          var r = items[i].getBoundingClientRect();
          if (r.bottom > listRect.top + 4) { target = items[i]; break; }
        }
        if (target) { show(target); centerInList(target); target.focus(); }
      } else if (e.key === 'ArrowLeft' || e.key === 'Backspace') {
        e.preventDefault();
        navigateParent();
      } else if (e.key === 'ArrowRight') {
        var li = e.target.closest('.item') || list.querySelector('.item.active');
        if (li) {
          var kind = li.getAttribute('data-kind') || 'video';
          if (kind === 'dir') { e.preventDefault(); navigateTo(li.getAttribute('data-path') || ''); }
        }
      } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        var items = Array.prototype.slice.call(list.querySelectorAll('.item'));
        if (!items.length) return;
        var current = list.querySelector('.item.active') || e.target.closest('.item') || items[0];
        var idx = items.indexOf(current);
        if (idx === -1) idx = 0;
        if (e.key === 'ArrowDown' && idx < items.length - 1) idx++;
        if (e.key === 'ArrowUp' && idx > 0) idx--;
        var next = items[idx];
        if (next) { show(next); centerInList(next); next.focus(); }
      }
    });
  }
  // Global keyboard navigation: when focus is outside the list and overlay is closed,
  // navigation keys operate on the current selection and restore focus.
  document.addEventListener('keydown', function(e){
    if (e.defaultPrevented) return; // handled elsewhere (e.g., overlay)
    var overlayOpen = overlayBackdrop && overlayBackdrop.getAttribute('aria-hidden') === 'false';
    if (overlayOpen) return; // overlay has its own key handling
    if (!list) return;
    if (list.contains(e.target)) return; // list handler will take over
    var keys = ['ArrowDown','ArrowUp','PageDown','PageUp','Home','End','Enter',' ','ArrowLeft','ArrowRight','Backspace'];
    if (keys.indexOf(e.key) === -1) return;
    var items = Array.prototype.slice.call(list.querySelectorAll('.item'));
    if (!items.length) return;
    var current = list.querySelector('.item.active') || items[0];
    var idx = items.indexOf(current);
    if (idx === -1) idx = 0;
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      show(current); centerInList(current); current.focus(); openOverlayFor(current, 'play');
      return;
    }
    if (e.key === 'ArrowLeft' || e.key === 'Backspace') {
      e.preventDefault(); navigateParent(); return;
    }
    if (e.key === 'ArrowRight') {
      var kind = current.getAttribute('data-kind') || 'video';
      if (kind === 'dir') { e.preventDefault(); navigateTo(current.getAttribute('data-path') || ''); }
      return;
    }
    var nextIdx = idx;
    if (e.key === 'ArrowDown') nextIdx = Math.min(items.length - 1, idx + 1);
    else if (e.key === 'ArrowUp') nextIdx = Math.max(0, idx - 1);
    else if (e.key === 'PageDown') nextIdx = Math.min(items.length - 1, idx + 10);
    else if (e.key === 'PageUp') nextIdx = Math.max(0, idx - 10);
    else if (e.key === 'Home') nextIdx = 0;
    else if (e.key === 'End') nextIdx = items.length - 1;
    if (nextIdx !== idx) {
      e.preventDefault();
      var next = items[nextIdx];
      show(next); centerInList(next); next.focus();
    }
  });
  // Auto-select first item
  if (list) {
    var first = list.querySelector('.item');
    if (first) { show(first); first.focus(); }
  }
  // htmx status handling for /play
  if (window.htmx) {
    document.body.addEventListener('htmx:beforeRequest', function(evt){
      var path = evt.detail && evt.detail.requestConfig && evt.detail.requestConfig.path;
      if (path === '/play') {
        var target = document.getElementById('overlay-body');
        if (target) {
          var n = document.createElement('div');
          n.className = 'muted';
          n.textContent = 'Casting…';
          target.appendChild(n);
        }
      }
    });
    document.body.addEventListener('htmx:afterRequest', function(evt){
      var path = evt.detail && evt.detail.requestConfig && evt.detail.requestConfig.path;
      if (path === '/play') {
        var xhr = evt.detail.xhr; var status = xhr ? xhr.status : 0;
        var target = document.getElementById('overlay-body');
        if (!target) return;
        var msg = document.createElement('div');
        msg.style.marginTop = '6px';
        if (status >= 200 && status < 300) {
          msg.textContent = 'Casting started.';
        } else {
          var text = xhr && xhr.responseText ? xhr.responseText : 'Failed to cast';
          msg.textContent = text;
          msg.className = 'muted';
        }
        target.appendChild(msg);
      }
    });
  }
})();
</script>
`
