package store

import (
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
)

// State represents the persisted application state.
// For now, it only stores the ytcast code/device.
type State struct {
    YtcastCode string `json:"ytcast_code"`
}

// LoadState reads state from path.
// If the file does not exist, it returns a zero-value State and nil error.
// If the file exists but cannot be parsed, it returns an error so callers can log it.
func LoadState(path string) (State, error) {
    var s State
    f, err := os.Open(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return s, nil
        }
        return s, fmt.Errorf("open state: %w", err)
    }
    defer f.Close()
    data, err := io.ReadAll(f)
    if err != nil {
        return s, fmt.Errorf("read state: %w", err)
    }
    if len(data) == 0 {
        return s, nil
    }
    if err := json.Unmarshal(data, &s); err != nil {
        return s, fmt.Errorf("decode state: %w", err)
    }
    return s, nil
}

// SaveState writes state to path atomically.
func SaveState(path string, s State) error {
    // Tighten directory permissions: 0750 by default.
    if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
        return fmt.Errorf("mkdir: %w", err)
    }
    tmp := path + ".tmp"
    f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
    if err != nil {
        return fmt.Errorf("open tmp: %w", err)
    }
    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    if err := enc.Encode(&s); err != nil {
        f.Close()
        _ = os.Remove(tmp)
        return fmt.Errorf("encode state: %w", err)
    }
    if err := f.Close(); err != nil {
        _ = os.Remove(tmp)
        return fmt.Errorf("close tmp: %w", err)
    }
    if err := os.Rename(tmp, path); err != nil {
        _ = os.Remove(tmp)
        return fmt.Errorf("rename tmp: %w", err)
    }
    return nil
}
