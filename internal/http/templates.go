package http

import (
	"embed"
	"html/template"
	"net/url"
	"strings"
	"time"
)

//go:embed templates/browse.html templates/pair.html
var pageTemplates embed.FS

func newBrowseTemplate() *template.Template {
	return template.Must(template.New("browse.html").Funcs(pageTemplateFuncs()).ParseFS(pageTemplates, "templates/browse.html"))
}

func newPairTemplate() *template.Template {
	return template.Must(template.New("pair.html").ParseFS(pageTemplates, "templates/pair.html"))
}

func pageTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"join": strings.Join,
		"q":    url.QueryEscape,
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
		"urlfor": templateURLFor,
	}
}

func templateURLFor(base, name string) string {
	if name == "" {
		if base == "" {
			return "/"
		}
		var parts []string
		for _, s := range strings.Split(base, "/") {
			if s != "" {
				parts = append(parts, url.PathEscape(s))
			}
		}
		return "/" + strings.Join(parts, "/") + "/"
	}
	if u, err := url.Parse(name); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return name
	}
	var segs []string
	if base != "" {
		segs = append(segs, strings.Split(base, "/")...)
	}
	segs = append(segs, strings.Split(name, "/")...)
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		if s == "" {
			continue
		}
		out = append(out, url.PathEscape(s))
	}
	return "/" + strings.Join(out, "/")
}
