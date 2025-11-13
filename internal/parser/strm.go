package parser

import (
	"bufio"
	"net/url"
	"os"
	"strings"
)

// ParseSTRM reads a .strm file and extracts the YouTube video_id query parameter.
func ParseSTRM(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	line, err := r.ReadString('\n')
	if err != nil && !strings.HasSuffix(line, "\n") {
		// allow EOF without newline
		if err != nil {
			// keep line as-is; err is likely EOF; ignore
		}
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	// The line may not be a standard URL but should contain query params.
	// Try to split on '?' and parse the query part.
	var rawQ string
	if i := strings.IndexByte(line, '?'); i >= 0 {
		rawQ = line[i+1:]
	} else {
		// attempt to parse entire line as URL for robustness
		rawQ = line
	}
	values, _ := url.ParseQuery(rawQ)
	return values.Get("video_id"), nil
}
