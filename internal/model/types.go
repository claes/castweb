package model

// Video represents a single playable item with associated metadata.
type Video struct {
	Name     string // base filename without extension
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
}
