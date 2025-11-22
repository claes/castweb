package browse

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/claes/ytplv/internal/model"
	"github.com/claes/ytplv/internal/parser"
)

// BuildListing scans a directory under root and returns directories and paired videos.
// rel must be a clean, relative path ("" or "." means root).
func BuildListing(root, rel string) (model.Listing, error) {
	listing := model.Listing{Path: cleanRel(rel)}
	if listing.Path != "" {
		listing.ParentPath = parentOf(listing.Path)
	}

	dir := filepath.Join(root, listing.Path)
	if !IsSubpath(root, dir) {
		return listing, os.ErrPermission
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return listing, err
	}

    // Collect .strm/.url base names and their paths; only include if matching .nfo exists.
    type pair struct {
        base  string
        strm  string
        url   string
        nfo   string
        mtime time.Time
    }
	pairs := make(map[string]*pair)

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			listing.Dirs = append(listing.Dirs, name)
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		base := strings.TrimSuffix(name, ext)
        switch ext {
        case ".strm":
            p := pairs[base]
            if p == nil {
                p = &pair{base: base}
                pairs[base] = p
            }
            p.strm = filepath.Join(dir, name)
            if fi, err := os.Stat(filepath.Join(dir, name)); err == nil {
                // only set mtime from .strm if no .url has been seen
                if p.url == "" {
                    p.mtime = fi.ModTime()
                }
            }
        case ".url":
            p := pairs[base]
            if p == nil {
                p = &pair{base: base}
                pairs[base] = p
            }
            p.url = filepath.Join(dir, name)
            if fi, err := os.Stat(filepath.Join(dir, name)); err == nil {
                // .url takes precedence
                p.mtime = fi.ModTime()
            }
        case ".nfo":
            p := pairs[base]
            if p == nil {
                p = &pair{base: base}
                pairs[base] = p
            }
            p.nfo = filepath.Join(dir, name)
        }
    }

    for _, p := range pairs {
        // Require metadata, and at least one of .url or .strm
        if p.nfo == "" || (p.strm == "" && p.url == "") {
            continue // only include pairs
        }
        var typ, vid, rawURL string
        if p.url != "" {
            if u, err := parser.ParseURLFile(p.url); err == nil {
                rawURL = u
            } else {
                continue
            }
        } else {
            t, v, err := parser.ParseStream(p.strm)
            if err != nil || v == "" {
                continue
            }
            typ, vid = t, v
        }
        title, plot, thumb, tags, err := parser.ParseNFO(p.nfo)
        if err != nil {
            continue
        }
        listing.Videos = append(listing.Videos, model.Video{
            Name:     p.base,
            Type:     typ,
            VideoID:  vid,
            URL:      rawURL,
            Title:    title,
            Plot:     plot,
            ThumbURL: thumb,
            Tags:     tags,
        })
		// add to combined entries with mod time
        listing.Entries = append(listing.Entries, model.Entry{
            Kind:    "video",
            Name:    titleOr(p.base, title),
            ModTime: p.mtime,
            Video:   &model.Video{Name: p.base, Type: typ, VideoID: vid, URL: rawURL, Title: title, Plot: plot, ThumbURL: thumb, Tags: tags},
        })
    }

	sort.Strings(listing.Dirs)
	sort.Slice(listing.Videos, func(i, j int) bool {
		// Prefer sort by Title; fallback to Name
		ti := listing.Videos[i].Title
		tj := listing.Videos[j].Title
		if ti == tj {
			return listing.Videos[i].Name < listing.Videos[j].Name
		}
		if ti == "" {
			return false
		}
		if tj == "" {
			return true
		}
		return strings.ToLower(ti) < strings.ToLower(tj)
	})
	// For each immediate subdirectory, compute newest .strm mtime among valid pairs to sort
	for _, d := range listing.Dirs {
		sub := filepath.Join(dir, d)
		var latest time.Time
        if des, err := os.ReadDir(sub); err == nil {
            // build local map to require pairs within subdir
            mp := map[string]struct {
                hasMedia bool // .url or .strm
                hasNfo  bool
                m       time.Time
            }{}
            for _, de := range des {
                if de.IsDir() {
                    continue
                }
                ext := strings.ToLower(filepath.Ext(de.Name()))
                base := strings.TrimSuffix(de.Name(), ext)
                switch ext {
                case ".strm":
                    fi, _ := os.Stat(filepath.Join(sub, de.Name()))
                    m := time.Time{}
                    if fi != nil {
                        m = fi.ModTime()
                    }
                    v := mp[base]
                    v.hasMedia = true
                    if v.m.IsZero() { // don't override .url mtime if already set
                        v.m = m
                    }
                    mp[base] = v
                case ".url":
                    fi, _ := os.Stat(filepath.Join(sub, de.Name()))
                    m := time.Time{}
                    if fi != nil {
                        m = fi.ModTime()
                    }
                    v := mp[base]
                    // .url takes precedence for mtime as well
                    v.hasMedia = true
                    v.m = m
                    mp[base] = v
                case ".nfo":
                    v := mp[base]
                    v.hasNfo = true
                    mp[base] = v
                }
            }
            for _, v := range mp {
                if v.hasMedia && v.hasNfo {
                    if v.m.After(latest) {
                        latest = v.m
                    }
                }
            }
        }
		// Fallback: if no valid pairs were found, use directory's own mtime
		if latest.IsZero() {
			if fi, err := os.Stat(sub); err == nil {
				latest = fi.ModTime()
			}
		}
		listing.Entries = append(listing.Entries, model.Entry{
			Kind:    "dir",
			Name:    d,
			Path:    cleanRel(filepath.Join(listing.Path, d)),
			ModTime: latest,
		})
	}
	// Sort combined entries by modtime desc; if equal then by name
	sort.SliceStable(listing.Entries, func(i, j int) bool {
		mi, mj := listing.Entries[i].ModTime, listing.Entries[j].ModTime
		if mi.Equal(mj) {
			return strings.ToLower(listing.Entries[i].Name) < strings.ToLower(listing.Entries[j].Name)
		}
		return mi.After(mj)
	})
	return listing, nil
}

func titleOr(name, title string) string {
	if strings.TrimSpace(title) != "" {
		return title
	}
	return name
}

func cleanRel(rel string) string {
	rel = filepath.Clean(rel)
	rel = strings.TrimPrefix(rel, string(filepath.Separator))
	if rel == "." {
		return ""
	}
	return rel
}

func parentOf(rel string) string {
	p := filepath.Dir(rel)
	if p == "." {
		return ""
	}
	// prevent going above root
	if strings.HasPrefix(p, "..") {
		return ""
	}
	return p
}

// Exists reports whether a path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

// IsSubpath ensures child is within root, preventing path traversal.
func IsSubpath(root, child string) bool {
	absRoot, _ := filepath.Abs(root)
	absChild, _ := filepath.Abs(child)
	rel, err := filepath.Rel(absRoot, absChild)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
