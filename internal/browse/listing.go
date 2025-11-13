package browse

import (
    "os"
    "path/filepath"
    "sort"
    "strings"

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

    // Collect .strm base names and their paths; only include if matching .nfo exists.
    type pair struct{
        base string
        strm string
        nfo  string
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
            if p == nil { p = &pair{base: base}; pairs[base] = p }
            p.strm = filepath.Join(dir, name)
        case ".nfo":
            p := pairs[base]
            if p == nil { p = &pair{base: base}; pairs[base] = p }
            p.nfo = filepath.Join(dir, name)
        }
    }

    for _, p := range pairs {
        if p.strm == "" || p.nfo == "" {
            continue // only include pairs
        }
        vid, err := parser.ParseSTRM(p.strm)
        if err != nil || vid == "" {
            continue
        }
        title, plot, thumb, tags, err := parser.ParseNFO(p.nfo)
        if err != nil {
            continue
        }
        listing.Videos = append(listing.Videos, model.Video{
            Name:     p.base,
            VideoID:  vid,
            Title:    title,
            Plot:     plot,
            ThumbURL: thumb,
            Tags:     tags,
        })
    }

    sort.Strings(listing.Dirs)
    sort.Slice(listing.Videos, func(i, j int) bool {
        // Prefer sort by Title; fallback to Name
        ti := listing.Videos[i].Title
        tj := listing.Videos[j].Title
        if ti == tj { return listing.Videos[i].Name < listing.Videos[j].Name }
        if ti == "" { return false }
        if tj == "" { return true }
        return strings.ToLower(ti) < strings.ToLower(tj)
    })
    return listing, nil
}

func cleanRel(rel string) string {
    rel = filepath.Clean(rel)
    rel = strings.TrimPrefix(rel, string(filepath.Separator))
    if rel == "." { return "" }
    return rel
}

func parentOf(rel string) string {
    p := filepath.Dir(rel)
    if p == "." { return "" }
    // prevent going above root
    if strings.HasPrefix(p, "..") { return "" }
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
    if err != nil { return false }
    return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
