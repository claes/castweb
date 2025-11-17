package model

import "time"

// Video represents a single playable item with associated metadata.
type Video struct {
    Name     string // base filename without extension
    Type     string // source type: youtube, svtplay
    VideoID  string
    Title    string
    Plot     string
    ThumbURL string
    Tags     []string
}

// Listing represents the contents of a directory.
type Listing struct {
	Path       string   // relative path from root ("" for root)
	ParentPath string   // relative path to parent ("" if at root)
	Dirs       []string // child directory names
	Videos     []Video
	Entries    []Entry // combined list of dirs and videos, sorted by mod time
}

// Entry is a unified list item for UI navigation.
type Entry struct {
	Kind    string    // "dir" or "video"
	Name    string    // directory name or video base name/title for display
	Path    string    // for Kind=="dir": relative path to directory
	ModTime time.Time // source: .strm mod time (or best-effort)
	Video   *Video    // populated when Kind=="video"
}
