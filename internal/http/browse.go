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
              data-plot="{{.Video.Plot}}">
            {{if .Video.ThumbURL}}<img class="thumb" src="{{.Video.ThumbURL}}" alt="thumb">{{end}}
            <div class="title">{{if .Video.Title}}{{.Video.Title}}{{else}}{{.Video.Name}}{{end}}</div>
          </li>
        {{end}}
      {{end}}
      </ul>
    </div>
  </div>
  {{else}}
    <small>No items found in this folder</small>
  {{end}}
</section>

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
    } else {
      html += '<p class="muted">Folder. Press Enter or → to open.</p>';
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
          else { show(li); }
        }
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
})();
</script>
`
