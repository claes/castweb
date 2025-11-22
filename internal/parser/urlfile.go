package parser

import (
    "bufio"
    "os"
    "strings"
)

// ParseURLFile reads a .url file and returns the first non-empty line trimmed.
// If the file is empty or only contains whitespace, it returns an empty string.
func ParseURLFile(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil {
        return "", err
    }
    defer f.Close()
    r := bufio.NewReader(f)
    for {
        line, err := r.ReadString('\n')
        line = strings.TrimSpace(line)
        if line != "" {
            return line, nil
        }
        if err != nil {
            // EOF or other error; return empty if nothing found yet
            if line == "" {
                return "", nil
            }
            return line, nil
        }
    }
}

