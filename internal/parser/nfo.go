package parser

import (
    "encoding/xml"
    "os"
    "strings"
)

type movie struct {
    Title string   `xml:"title"`
    Plot  string   `xml:"plot"`
    Thumb string   `xml:"thumb"`
    Tags  []string `xml:"tag"`
}

// ParseNFO parses a Kodi-compatible .nfo XML file and returns key fields.
func ParseNFO(path string) (title, plot, thumb string, tags []string, err error) {
    b, err := os.ReadFile(path)
    if err != nil {
        return "", "", "", nil, err
    }
    // Some .nfo files may have HTML entities; xml.Unmarshal handles them.
    var m movie
    if err := xml.Unmarshal(b, &m); err != nil {
        return "", "", "", nil, err
    }
    // Normalize whitespace lightly
    title = strings.TrimSpace(m.Title)
    plot = strings.TrimSpace(m.Plot)
    thumb = strings.TrimSpace(m.Thumb)
    tags = make([]string, 0, len(m.Tags))
    for _, t := range m.Tags {
        t = strings.TrimSpace(t)
        if t != "" {
            tags = append(tags, t)
        }
    }
    return
}

