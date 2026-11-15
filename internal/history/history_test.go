package history

import (
	"path/filepath"
	"testing"
	"time"

	"halobattery/internal/device"
)

func level(n int) *int { return &n }

func TestAddAndTrend(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "history.json"))
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return base }

	devs := []device.Status{{Key: "razer:1", Name: "Mouse", Level: level(80)}}
	s.Add(devs)

	base = base.Add(6 * time.Minute)
	devs[0].Level = level(78)
	s.Add(devs)

	base = base.Add(6 * time.Minute)
	devs[0].Level = level(76)
	s.Add(devs)

	levels, minutes := s.Trend("razer:1")
	if len(levels) != 3 || levels[0] != 80 || levels[2] != 76 {
		t.Fatalf("levels = %v", levels)
	}
	// 4 points lost over 12 min => 3 min per point => 76 % left is 228 min.
	if minutes != 228 {
		t.Fatalf("minutes = %d", minutes)
	}

	// A reload must read the same file back.
	reloaded := New(s.Path)
	levels, _ = reloaded.Trend("razer:1")
	if len(levels) != 3 {
		t.Fatalf("reloaded levels = %v", levels)
	}
}

func TestAddThrottlesSamples(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "history.json"))
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return base }
	s.Add([]device.Status{{Key: "k", Level: level(50)}})

	s.now = func() time.Time { return base.Add(30 * time.Second) }
	s.Add([]device.Status{{Key: "k", Level: level(49)}}) // same level gap, dropped
	levels, _ := s.Trend("k")
	if len(levels) != 1 {
		t.Fatalf("levels = %v", levels)
	}
}

func TestTrendUnknownAndNil(t *testing.T) {
	var nilStore *Store
	if levels, minutes := nilStore.Trend("x"); levels != nil || minutes != -1 {
		t.Fatal("nil store should be a no-op")
	}
	s := New(filepath.Join(t.TempDir(), "history.json"))
	if levels, minutes := s.Trend("missing"); levels != nil || minutes != -1 {
		t.Fatal("missing key should have no trend")
	}
}
