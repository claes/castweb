package http

import (
	"html/template"
	nethttp "net/http"
	"net/url"
	stdpath "path"
	"strings"

	"github.com/claes/ytplv/internal/browse"
)

type server struct {
	root string
	tpl  *template.Template
}

// NewServer creates an HTTP handler for browsing video metadata rooted at dir.
func NewServer(root string) nethttp.Handler {
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
	s := &server{root: root, tpl: tpl}
	mux := nethttp.NewServeMux()
	mux.HandleFunc("/", s.handleBrowse)
	mux.HandleFunc("/health", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		HealthHandler().ServeHTTP(w, r)
	})
	return mux
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tpl.Execute(w, listing)
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
<title>ytplv</title>
<style>
body{font-family:system-ui,-apple-system,Segoe UI,Roboto;max-width:1100px;margin:0 auto;padding:1rem}
header{display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem}
ul{list-style:none;padding:0;margin:0}
.layout{display:grid;grid-template-columns:1.2fr 1fr;gap:16px;align-items:start}
.panel{border:1px solid #ddd;border-radius:8px;padding:12px}
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
</style>
<header>
  <div>
    <a href="/{{.ParentPath}}">⬅ Up</a>
    <strong style="margin-left:8px">/{{.Path}}</strong>
  </div>
  <a href="/">Root</a>
</header>

<section class="dirs">
  <h3>Folders</h3>
  {{if .Dirs}}
    <div class="panel" id="dirs">
      <ul id="dirsList" role="listbox" aria-label="Folders">
      {{range .Dirs}}
        <li class="dir-item" role="option" aria-selected="false" tabindex="0" data-name="{{.}}" data-path="{{pjoin $.Path .}}">📁 {{.}}</li>
      {{end}}
      </ul>
    </div>
  {{else}}
    <small>No subfolders</small>
  {{end}}
  <hr/>
</section>

<section>
  <h3>Videos</h3>
  {{if .Videos}}
  <div class="layout">
    <div class="panel details" id="details" aria-live="polite">
      <div class="muted">Select a video →</div>
    </div>
    <div class="panel list" id="list">
      <ul role="listbox" aria-label="Videos">
      {{range .Videos}}
        <li class="item" role="option" aria-selected="false" tabindex="0"
            data-title="{{if .Title}}{{.Title}}{{else}}{{.Name}}{{end}}"
            data-id="{{.VideoID}}"
            data-thumb="{{.ThumbURL}}"
            data-tags="{{join .Tags ", "}}"
            data-plot="{{.Plot}}">
          {{if .ThumbURL}}<img class="thumb" src="{{.ThumbURL}}" alt="thumb">{{end}}
          <div class="title">{{if .Title}}{{.Title}}{{else}}{{.Name}}{{end}}</div>
        </li>
      {{end}}
      </ul>
    </div>
  </div>
  {{else}}
    <small>No videos found in this folder</small>
  {{end}}
</section>

<script>
(function(){
  var currentPath = {{printf "%q" .Path}};
  var parentPath = {{printf "%q" .ParentPath}};
  var details = document.getElementById('details');
  var list = document.getElementById('list');
  var dirsList = document.getElementById('dirsList');
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
    if (thumb) html += '<img src="' + esc(thumb) + '" alt="thumb" />';
    if (tags) html += '<div class="muted" style="margin-top:6px">Tags: ' + esc(tags) + '</div>';
    if (plot) html += '<p style="white-space:pre-wrap">' + esc(plot) + '</p>';
    if (id) html += '<p><a target="_blank" href="https://www.youtube.com/watch?v=' + esc(id) + '">Open on YouTube</a></p>';
    details.innerHTML = html;
  }
  if (list) {
    list.addEventListener('click', function(e){
      var li = e.target.closest('.item');
      if (li) { show(li); li.focus(); }
    });
    list.addEventListener('keydown', function(e){
      if (e.key === 'Enter' || e.key === ' ') {
        var li = e.target.closest('.item');
        if (li) { e.preventDefault(); show(li); }
      } else if (e.key === 'ArrowLeft' || e.key === 'Backspace') {
        e.preventDefault();
        navigateParent();
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
  // Folder list interactions and keyboard navigation
  if (dirsList) {
    function selectDir(li){
      if (!li) return;
      Array.prototype.forEach.call(dirsList.querySelectorAll('.dir-item'), function(n){ n.classList.remove('active'); n.setAttribute('aria-selected', 'false'); });
      li.classList.add('active');
      li.setAttribute('aria-selected', 'true');
    }
    dirsList.addEventListener('click', function(e){
      var li = e.target.closest('.dir-item');
      if (li) { e.preventDefault(); navigateTo(li.getAttribute('data-path') || ''); }
    });
    dirsList.addEventListener('keydown', function(e){
      if (e.key === 'Enter' || e.key === 'ArrowRight') {
        var li = e.target.closest('.dir-item') || dirsList.querySelector('.dir-item.active');
        if (li) { e.preventDefault(); navigateTo(li.getAttribute('data-path') || ''); }
      } else if (e.key === 'ArrowLeft' || e.key === 'Backspace') {
        e.preventDefault();
        navigateParent();
      } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        var items = Array.prototype.slice.call(dirsList.querySelectorAll('.dir-item'));
        if (!items.length) return;
        var current = dirsList.querySelector('.dir-item.active') || e.target.closest('.dir-item') || items[0];
        var idx = items.indexOf(current);
        if (idx === -1) idx = 0;
        if (e.key === 'ArrowDown' && idx < items.length - 1) idx++;
        if (e.key === 'ArrowUp' && idx > 0) idx--;
        var next = items[idx];
        if (next) { selectDir(next); next.focus(); next.scrollIntoView({ block: 'nearest' }); }
      }
    });
    // Initial selection for directories
    var firstDir = dirsList.querySelector('.dir-item');
    if (firstDir) { selectDir(firstDir); }
  }
})();
</script>
`
