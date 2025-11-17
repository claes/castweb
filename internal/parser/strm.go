package parser

import (
    "bufio"
    "net/url"
    "os"
    "strings"
)

// ParseStream reads a .strm file and returns the stream type and id.
// Supported types:
// - youtube: plugin://plugin.video.youtube with query param video_id
// - svtplay: plugin://plugin.video.svtplay with query param id (URL-encoded path)
// For unsupported or empty lines, returns ("", "").
func ParseStream(path string) (streamType, id string, err error) {
    f, err := os.Open(path)
    if err != nil {
        return "", "", err
    }
    defer f.Close()

    r := bufio.NewReader(f)
    line, _ := r.ReadString('\n')
    line = strings.TrimSpace(line)
    if line == "" {
        return "", "", nil
    }
    lower := strings.ToLower(line)
    // Extract raw query string (part after '?'), or whole line as fallback
    var rawQ string
    if i := strings.IndexByte(line, '?'); i >= 0 {
        rawQ = line[i+1:]
    } else {
        rawQ = line
    }
    q, _ := url.ParseQuery(rawQ)
    switch {
    case strings.HasPrefix(lower, "plugin://plugin.video.youtube"):
        return "youtube", q.Get("video_id"), nil
    case strings.HasPrefix(lower, "plugin://plugin.video.svtplay"):
        return "svtplay", q.Get("id"), nil
    default:
        return "", "", nil
    }
}
