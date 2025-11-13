package http

import (
    "html/template"
    nethttp "net/http"
    "net/url"
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
        "join":  strings.Join,
        "q":     url.QueryEscape,
        "pjoin": func(a, b string) string {
            if a == "" { return b }
            if b == "" { return a }
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
    rel := r.URL.Query().Get("path")
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
.dirs a{display:inline-block;margin:0 8px 8px 0}
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
    <a href="/?path={{.ParentPath}}">⬅ Up</a>
    <strong style="margin-left:8px">/{{.Path}}</strong>
  </div>
  <a href="/">Root</a>
</header>

<section class="dirs">
  <h3>Folders</h3>
  {{if .Dirs}}
    {{range .Dirs}}
      <a href="/?path={{pjoin $.Path .}}">📁 {{.}}</a>
    {{end}}
  {{else}}
    <small>No subfolders</small>
  {{end}}
  <hr/>
</section>

<section>
  <h3>Videos</h3>
  {{if .Videos}}
  <div class="layout">
    <div class="panel details" id="details">
      <div class="muted">Select a video →</div>
    </div>
    <div class="panel list" id="list">
      <ul>
      {{range .Videos}}
        <li class="item" tabindex="0"
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
  var details = document.getElementById('details');
  var list = document.getElementById('list');
  if (!list) return;
  function esc(s){
    return String(s)
      .replace(/&/g,'&amp;')
      .replace(/</g,'&lt;')
      .replace(/>/g,'&gt;')
      .replace(/"/g,'&quot;')
      .replace(/'/g,'&#39;');
  }
  function show(li){
    if (!li) return;
    Array.prototype.forEach.call(list.querySelectorAll('.item'), function(n){ n.classList.remove('active'); });
    li.classList.add('active');
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
  list.addEventListener('click', function(e){
    var li = e.target.closest('.item');
    if (li) show(li);
  });
  list.addEventListener('keydown', function(e){
    if (e.key === 'Enter' || e.key === ' ') {
      var li = e.target.closest('.item');
      if (li) { e.preventDefault(); show(li); }
    }
  });
  // Auto-select first item
  var first = list.querySelector('.item');
  if (first) show(first);
})();
</script>
`
