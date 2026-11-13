// Package config persists user settings as JSON in the XDG config dir.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config mirrors the settings the Python app exposes in its flyout.
type Config struct {
	Interval  int      `json:"interval"` // seconds between polls
	Low       int      `json:"low"`      // low battery threshold, %
	Notify    bool     `json:"notify"`   // desktop notifications
	Autostart bool     `json:"autostart"`
	Disabled  []string `json:"disabled_providers"` // provider names switched off
}

// Default returns the shipped defaults.
func Default() Config {
	return Config{Interval: 60, Low: 20, Notify: true}
}

// Clamp keeps hand-edited or hostile values usable.
func (c *Config) Clamp() {
	if c.Interval < 5 {
		c.Interval = 5
	}
	if c.Interval > 3600 {
		c.Interval = 3600
	}
	if c.Low < 0 {
		c.Low = 0
	}
	if c.Low > 100 {
		c.Low = 100
	}
}

// Store reads and writes the config file.
type Store struct {
	Path string
}

// DefaultStore returns the per-user config location.
func DefaultStore() *Store {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return &Store{Path: filepath.Join(dir, "HaloBattery", "config.json")}
}

// Load returns the stored config, or the defaults when the file is missing.
func (s *Store) Load() Config {
	cfg := Default()
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		return cfg
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return Default()
	}
	cfg.Clamp()
	return cfg
}

// Save writes the config atomically so a crash cannot truncate it.
func (s *Store) Save(cfg Config) error {
	cfg.Clamp()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}
