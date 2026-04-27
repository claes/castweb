package http

import (
	nethttp "net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/claes/ytplv/internal/browse"
	"github.com/claes/ytplv/internal/model"
)

const browsePageSize = 100

type breadcrumb struct {
	Name    string
	Href    string
	Current bool
}

type browsePageData struct {
	Page        int
	HasPrev     bool
	HasNext     bool
	PrevURL     string
	NextURL     string
	Path        string
	ParentPath  string
	Entries     []model.Entry
	Breadcrumbs []breadcrumb
}

type pairPageData struct {
	ActiveDevice string
}

func requestRelPath(urlPath string) string {
	p := filepath.Clean(urlPath)
	if p == "/" {
		return ""
	}
	return strings.TrimPrefix(p, "/")
}

func decodeRelPath(rel string) string {
	if rel == "" {
		return ""
	}
	segs := strings.Split(rel, "/")
	for i, s := range segs {
		if u, err := url.PathUnescape(s); err == nil {
			segs[i] = u
		}
	}
	return strings.Join(segs, "/")
}

func (s *server) serveImage(w nethttp.ResponseWriter, r *nethttp.Request, rel string) bool {
	if rel == "" {
		return false
	}
	lower := strings.ToLower(rel)
	if !strings.HasSuffix(lower, ".jpg") && !strings.HasSuffix(lower, ".jpeg") && !strings.HasSuffix(lower, ".png") {
		return false
	}
	full := filepath.Join(s.root, rel)
	if !browse.IsSubpath(s.root, full) {
		return false
	}
	fi, err := os.Stat(full)
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	switch {
	case strings.HasSuffix(lower, ".png"):
		w.Header().Set("Content-Type", "image/png")
	default:
		w.Header().Set("Content-Type", "image/jpeg")
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	nethttp.ServeFile(w, r, full)
	return true
}

func browsePageFromRequest(r *nethttp.Request, listing model.Listing, rel string) browsePageData {
	page := currentPage(r)
	start, end, hasPrev, hasNext := pageBounds(page, len(listing.Entries))
	prevURL, nextURL := pageLinks(r.URL.Query(), rel, page, hasPrev, hasNext)

	return browsePageData{
		Page:        page,
		HasPrev:     hasPrev,
		HasNext:     hasNext,
		PrevURL:     prevURL,
		NextURL:     nextURL,
		Path:        listing.Path,
		ParentPath:  listing.ParentPath,
		Entries:     listing.Entries[start:end],
		Breadcrumbs: breadcrumbsFor(listing.Path),
	}
}

func currentPage(r *nethttp.Request) int {
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	return page
}

func pageBounds(page, total int) (start, end int, hasPrev, hasNext bool) {
	start = (page - 1) * browsePageSize
	if start > total {
		start = total
	}
	end = start + browsePageSize
	if end > total {
		end = total
	}
	hasPrev = page > 1
	hasNext = end < total
	return start, end, hasPrev, hasNext
}

func pageLinks(query url.Values, rel string, page int, hasPrev, hasNext bool) (prevURL, nextURL string) {
	base := encodedBrowsePath(rel)
	q := cloneValues(query)
	if hasPrev {
		q.Set("page", strconv.Itoa(page-1))
		prevURL = base + "?" + q.Encode()
	}
	if hasNext {
		q.Set("page", strconv.Itoa(page+1))
		nextURL = base + "?" + q.Encode()
	}
	return prevURL, nextURL
}

func cloneValues(src url.Values) url.Values {
	dst := make(url.Values, len(src))
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}

func encodedBrowsePath(rel string) string {
	if rel == "" {
		return "/"
	}
	var parts []string
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" {
			continue
		}
		parts = append(parts, url.PathEscape(seg))
	}
	return "/" + strings.Join(parts, "/") + "/"
}

func breadcrumbsFor(path string) []breadcrumb {
	if path == "" {
		return []breadcrumb{{Name: "Root", Href: "/", Current: true}}
	}

	crumbs := []breadcrumb{{Name: "Root", Href: "/", Current: false}}
	parts := strings.Split(path, "/")
	var acc string
	for i, seg := range parts {
		if seg == "" {
			continue
		}
		if acc == "" {
			acc = "/" + url.PathEscape(seg)
		} else {
			acc += "/" + url.PathEscape(seg)
		}
		crumbs = append(crumbs, breadcrumb{
			Name:    seg,
			Href:    acc,
			Current: i == len(parts)-1,
		})
	}
	return crumbs
}
