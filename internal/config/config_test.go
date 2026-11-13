package config

import (
	"path/filepath"
	"testing"
)

func TestClamp(t *testing.T) {
	c := Config{Interval: 1, Low: 150}
	c.Clamp()
	if c.Interval != 5 || c.Low != 100 {
		t.Fatalf("got %+v", c)
	}
}

func TestSaveLoad(t *testing.T) {
	store := &Store{Path: filepath.Join(t.TempDir(), "sub", "config.json")}
	want := Config{Interval: 120, Low: 15, Notify: false, Autostart: true, Disabled: []string{"upower"}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got := store.Load()
	if got.Interval != 120 || got.Low != 15 || got.Notify || !got.Autostart || len(got.Disabled) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadMissingUsesDefaults(t *testing.T) {
	store := &Store{Path: filepath.Join(t.TempDir(), "nope.json")}
	got := store.Load()
	if got.Interval != 60 || got.Low != 20 || !got.Notify {
		t.Fatalf("got %+v", got)
	}
}
