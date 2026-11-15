// Package history keeps a rolling 24 h record of battery levels per device,
// so the dashboard can draw a trend and estimate the remaining runtime.
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"halobattery/internal/config"
	"halobattery/internal/device"
)

const (
	retention   = 24 * time.Hour
	maxPoints   = 288              // 24 h at one sample every 5 min
	minGap      = 5 * time.Minute  // do not store more than one point per gap
	changeGap   = 60 * time.Second // a level change may land sooner than minGap
	trendWindow = 90 * time.Minute // window used by the runtime estimate
	trendPoints = 40               // points sent to the UI
	maxEstimate = 48 * time.Hour   // reject absurd extrapolations
)

// Point is one battery sample.
type Point struct {
	At    int64 `json:"t"` // unix seconds
	Level int   `json:"l"` // 0..100
}

type data struct {
	Devices map[string][]Point `json:"devices"`
}

// Store is the on-disk history. The zero value is not usable; use New.
type Store struct {
	Path string

	now func() time.Time

	mu     sync.Mutex
	loaded bool
	data   data
}

// New returns a store backed by path.
func New(path string) *Store {
	return &Store{Path: path, now: time.Now}
}

// DefaultStore returns $XDG_CONFIG_HOME/HaloBattery/history.json.
func DefaultStore() *Store {
	return New(filepath.Join(config.ConfigDir(), "history.json"))
}

func (s *Store) load() {
	if s.loaded {
		return
	}
	s.loaded = true
	s.data = data{Devices: map[string][]Point{}}
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		return
	}
	var d data
	if json.Unmarshal(raw, &d) == nil && d.Devices != nil {
		s.data = d
	}
}

// Add records every readable device and persists the file.
func (s *Store) Add(devs []device.Status) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()

	now := s.now().UTC()
	cutoff := now.Add(-retention).Unix()
	for _, d := range devs {
		if d.Level == nil || d.Key == "" {
			continue
		}
		series := s.data.Devices[d.Key]
		last := Point{}
		if len(series) > 0 {
			last = series[len(series)-1]
		}
		gap := now.Unix() - last.At
		changed := last.Level != *d.Level
		if len(series) > 0 && gap < int64(minGap.Seconds()) &&
			!(changed && gap >= int64(changeGap.Seconds())) {
			continue
		}
		series = append(series, Point{At: now.Unix(), Level: *d.Level})
		s.data.Devices[d.Key] = prune(series, cutoff)
	}

	for key, series := range s.data.Devices {
		kept := prune(series, cutoff)
		if len(kept) == 0 {
			delete(s.data.Devices, key)
			continue
		}
		s.data.Devices[key] = kept
	}
	s.save()
}

func prune(series []Point, cutoff int64) []Point {
	kept := series[:0]
	for _, p := range series {
		if p.At >= cutoff {
			kept = append(kept, p)
		}
	}
	if len(kept) > maxPoints {
		kept = kept[len(kept)-maxPoints:]
	}
	return kept
}

func (s *Store) save() {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return
	}
	raw, err := json.Marshal(s.data)
	if err != nil {
		return
	}
	tmp := s.Path + ".tmp"
	if os.WriteFile(tmp, raw, 0o644) == nil {
		_ = os.Rename(tmp, s.Path)
	}
}

// Trend returns a downsampled recent series and the estimated minutes left.
// minutes is -1 when there is not enough data to tell.
func (s *Store) Trend(key string) (levels []int, minutes int) {
	if s == nil {
		return nil, -1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()

	series := s.data.Devices[key]
	if len(series) == 0 {
		return nil, -1
	}
	levels = downsample(series, trendPoints)
	minutes = estimate(series, s.now())
	return levels, minutes
}

func downsample(series []Point, want int) []int {
	if len(series) <= want {
		out := make([]int, len(series))
		for i, p := range series {
			out[i] = p.Level
		}
		return out
	}
	out := make([]int, want)
	step := float64(len(series)-1) / float64(want-1)
	for i := range out {
		out[i] = series[int(float64(i)*step+0.5)].Level
	}
	return out
}

// estimate extrapolates the discharge slope of the last trendWindow.
func estimate(series []Point, now time.Time) int {
	if len(series) < 2 {
		return -1
	}
	last := series[len(series)-1]
	if last.Level >= 100 {
		return -1
	}
	cutoff := now.Add(-trendWindow).Unix()
	first := series[0]
	for _, p := range series {
		if p.At >= cutoff {
			first = p
			break
		}
	}
	dt := float64(last.At-first.At) / 60 // minutes
	drop := float64(first.Level - last.Level)
	if dt < 5 || drop <= 0 {
		return -1
	}
	perMinute := drop / dt
	if perMinute <= 0 {
		return -1
	}
	minutes := float64(last.Level) / perMinute
	if minutes > maxEstimate.Minutes() {
		return -1
	}
	return int(minutes)
}
