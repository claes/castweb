package http

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	nethttp "net/http"
	"net/url"
	"os/exec"
	stdpath "path"
	"strconv"
	"strings"
	"time"

	"github.com/claes/ytplv/internal/browse"
)

type server struct {
	root         string
	tpl          *template.Template
	ytcastDevice string
}

// NewServer creates an HTTP handler for browsing video metadata rooted at dir.
func NewServer(root string, ytcastDevice string) nethttp.Handler {
	// simple HTML template without external assets
	tpl := template.Must(template.New("page").Funcs(template.FuncMap{
		"join": strings.Join,
		"q":    url.QueryEscape,
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
	s := &server{root: root, tpl: tpl, ytcastDevice: ytcastDevice}
	mux := nethttp.NewServeMux()
	mux.HandleFunc("/", s.handleBrowse)
	mux.HandleFunc("/health", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		HealthHandler().ServeHTTP(w, r)
	})
	mux.HandleFunc("/play", s.handlePlay)
	return mux
}

func (s *server) handlePlay(w nethttp.ResponseWriter, r *nethttp.Request) {
	// Accept POST (htmx) or GET. Expect parameter "id" (YouTube video id).
	if err := r.ParseForm(); err != nil {
		log.Printf("/play: parse error: %v", err)
		httpError(w, nethttp.StatusBadRequest, "invalid form")
		return
	}
	id := r.FormValue("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	if id == "" {
		log.Printf("/play: missing id")
		httpError(w, nethttp.StatusBadRequest, "missing id")
		return
	}
	if s.ytcastDevice == "" {
		log.Printf("/play: device not configured; set -ytcast or YTCAST_DEVICE")
		httpError(w, nethttp.StatusBadRequest, "ytcast device not configured")
		return
	}
	// Build URL and execute ytcast
	ytURL := "https://www.youtube.com/watch?v=" + id
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	bin, _ := exec.LookPath("ytcast")
	args := []string{"-d", s.ytcastDevice, ytURL}
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
	log.Printf("/play: casting id=%s device=%s", id, s.ytcastDevice)
	log.Printf("/play: exec %s %s", prog, strings.Join(q, " "))
	if err := cmd.Run(); err != nil {
		exitCode := 0
		if ee, ok := err.(*exec.ExitError); ok && ee.ProcessState != nil {
			exitCode = ee.ProcessState.ExitCode()
		}
		outStr := strings.TrimSpace(stdout.String())
		errStr := strings.TrimSpace(stderr.String())
		log.Printf("/play: ytcast failed: err=%v exit=%d\nstdout: %s\nstderr: %s", err, exitCode, outStr, errStr)
		httpError(w, nethttp.StatusInternalServerError, "failed to cast")
		return
	}
	w.WriteHeader(nethttp.StatusNoContent)
}

func (s *server) handleBrowse(w nethttp.ResponseWriter, r *nethttp.Request) {
	// Derive relative path from URL path ("/" => "")
	p := stdpath.Clean(r.URL.Path)
	if p == "/" {
		p = ""
	} else {
		p = strings.TrimPrefix(p, "/")
	}
	rel := p
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tpl.Execute(w, data)
}

func httpError(w nethttp.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(msg))
}

const pageTpl = `<!doctype html>
<html lang="en">
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>castweb</title>
<style>
/* Ensure padding/borders are included in element width to prevent overflow */
*, *::before, *::after { box-sizing: border-box }
body{font-family:system-ui,-apple-system,Segoe UI,Roboto;margin:0;padding:16px 24px}
header{display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem}
ul{list-style:none;padding:0;margin:0}
.layout{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:24px;align-items:start}
.panel{border:1px solid #ddd;border-radius:8px;padding:12px}
.details{position:sticky;top:12px;align-self:start}
.list{max-height:calc(100vh - 180px);overflow:auto}
.dirs .dir-item{padding:6px;border-radius:6px;cursor:pointer;margin-bottom:4px}
.dirs .dir-item:hover{background:#f6f6f6}
.dirs .dir-item.active{background:#eef5ff}
.list .item{display:flex;gap:10px;align-items:center;padding:8px;border-radius:6px;cursor:pointer}
.list .item:hover{background:#f6f6f6}
.list .item.active{background:#eef5ff}
.thumb{width:96px;height:72px;object-fit:cover;border-radius:4px;flex:0 0 auto}
.title{font-weight:600}
.details img{max-width:100%;height:auto;border-radius:6px}
.muted, small{color:#666}
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
  <div>
    {{if ne .Path ""}}<a href="/{{.ParentPath}}">⬅ Up</a>{{end}}
    <strong style="margin-left:8px">/{{.Path}}</strong>
  </div>
  <a href="/">Root</a>
</header>

<section>
  {{if .Entries}}
  <div class="layout">
    <div class="panel details" id="details" aria-live="polite">
      <div class="muted">Select an item →</div>
    </div>
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
              data-id="{{.Video.VideoID}}"
              data-thumb="{{.Video.ThumbURL}}"
              data-tags="{{join .Video.Tags ", "}}"
              data-plot="{{.Video.Plot}}"
              hx-post="/play"
              hx-vals='{"id":"{{.Video.VideoID}}"}'
              hx-trigger="click, keyup[key=='Enter']"
              hx-swap="none">
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

<script src="https://unpkg.com/htmx.org@1.9.12"></script>
<script>
(function(){
  var currentPath = {{printf "%q" .Path}};
  var parentPath = {{printf "%q" .ParentPath}};
  var details = document.getElementById('details');
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
    var title = li.getAttribute('data-title') || '';
    var id = li.getAttribute('data-id') || '';
    var thumb = li.getAttribute('data-thumb') || '';
    var tags = li.getAttribute('data-tags') || '';
    var plot = li.getAttribute('data-plot') || '';
    var html = '';
    html += '<h2 style="margin-top:0">' + esc(title) + '</h2>';
    var kind = li.getAttribute('data-kind') || 'video';
    if (kind === 'video') {
      if (thumb) html += '<img src="' + esc(thumb) + '" alt="thumb" />';
      if (tags) html += '<div class="muted" style="margin-top:6px">Tags: ' + esc(tags) + '</div>';
      if (plot) html += '<p style="white-space:pre-wrap">' + esc(plot) + '</p>';
      if (id) html += '<p><a target="_blank" href="https://www.youtube.com/watch?v=' + esc(id) + '">Open on YouTube</a></p>';
    } else if (kind === 'dir') {
      html += '<p class="muted">Folder. Press Enter or → to open.</p>';
    } else if (kind === 'nav') {
      html += '<p class="muted">Navigation. Press Enter to follow.</p>';
    }
    details.innerHTML = html;
  }
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
        show(li); li.focus();
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
          else { show(li); }
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
        if (target) { show(target); target.focus(); }
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
        if (next) { show(next); next.focus(); next.scrollIntoView({ block: 'nearest' }); }
      }
    });
  }
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
        var detailsEl = document.getElementById('details');
        if (detailsEl) {
          var n = document.createElement('div');
          n.className = 'muted';
          n.textContent = 'Casting…';
          detailsEl.appendChild(n);
        }
      }
    });
    document.body.addEventListener('htmx:afterRequest', function(evt){
      var path = evt.detail && evt.detail.requestConfig && evt.detail.requestConfig.path;
      if (path === '/play') {
        var xhr = evt.detail.xhr; var status = xhr ? xhr.status : 0;
        var detailsEl = document.getElementById('details');
        if (!detailsEl) return;
        var msg = document.createElement('div');
        msg.style.marginTop = '6px';
        if (status >= 200 && status < 300) {
          msg.textContent = 'Casting started.';
        } else {
          var text = xhr && xhr.responseText ? xhr.responseText : 'Failed to cast';
          msg.textContent = text;
          msg.className = 'muted';
        }
        detailsEl.appendChild(msg);
      }
    });
  }
})();
</script>
`
