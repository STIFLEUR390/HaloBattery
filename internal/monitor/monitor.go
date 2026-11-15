// Package monitor polls every provider on a timer and merges the results.
package monitor

import (
	"sort"
	"strings"
	"sync"
	"time"

	"halobattery/internal/device"
	"halobattery/internal/providers"
)

// Monitor owns the polling loop and the latest snapshot.
type Monitor struct {
	providers []providers.Provider
	onUpdate  func([]device.Status)

	mu       sync.Mutex
	interval time.Duration
	devices  []device.Status
	busy     bool
	disabled map[string]bool
}

// New returns a monitor; onUpdate may be nil.
func New(ps []providers.Provider, interval time.Duration, onUpdate func([]device.Status)) *Monitor {
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	return &Monitor{providers: ps, interval: interval, onUpdate: onUpdate, disabled: map[string]bool{}}
}

// Start runs the polling loop until Stop is called.
func (m *Monitor) Start() {
	go func() {
		for {
			m.PollNow()
			time.Sleep(m.Interval())
		}
	}()
}

// Interval returns the current poll period.
func (m *Monitor) Interval() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.interval
}

// SetInterval changes the poll period; the next sleep uses it.
func (m *Monitor) SetInterval(d time.Duration) {
	if d < 5*time.Second {
		d = 5 * time.Second
	}
	m.mu.Lock()
	m.interval = d
	m.mu.Unlock()
}

// ProviderNames lists the available providers.
func (m *Monitor) ProviderNames() []string {
	names := make([]string, 0, len(m.providers))
	for _, p := range m.providers {
		names = append(names, p.Name())
	}
	return names
}

// SetDisabled switches providers off without recreating the monitor.
func (m *Monitor) SetDisabled(names []string) {
	m.mu.Lock()
	m.disabled = map[string]bool{}
	for _, n := range names {
		m.disabled[n] = true
	}
	m.mu.Unlock()
}

// Devices returns the last snapshot.
func (m *Monitor) Devices() []device.Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]device.Status(nil), m.devices...)
}

// PollNow refreshes all providers. Concurrent calls collapse into one.
func (m *Monitor) PollNow() {
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return
	}
	m.busy = true
	m.mu.Unlock()

	m.mu.Lock()
	disabled := make(map[string]bool, len(m.disabled))
	for k, v := range m.disabled {
		disabled[k] = v
	}
	m.mu.Unlock()

	var all []device.Status
	for _, p := range m.providers {
		if disabled[p.Name()] {
			continue
		}
		all = append(all, safePoll(p)...)
	}
	all = merge(all)
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Online != all[j].Online {
			return all[i].Online
		}
		if all[i].Source != all[j].Source {
			return all[i].Source < all[j].Source
		}
		return strings.ToLower(all[i].Name) < strings.ToLower(all[j].Name)
	})

	m.mu.Lock()
	m.devices = all
	m.busy = false
	m.mu.Unlock()

	if m.onUpdate != nil {
		m.onUpdate(all)
	}
}

// safePoll keeps one broken protocol from taking down the poll loop.
func safePoll(p providers.Provider) (out []device.Status) {
	defer func() {
		if recover() != nil {
			out = nil
		}
	}()
	return p.Poll()
}

// merge drops UPower copies of devices a vendor protocol already reported.
// A Razer mouse often shows up twice: once over its dongle, once over Bluetooth.
func merge(all []device.Status) []device.Status {
	// UPower reports Bluetooth peripherals too, with a coarser name; drop its
	// copy when a vendor or BlueZ provider already knows the same device.
	vendors := map[string]bool{}
	for _, d := range all {
		if d.Source != "upower" {
			vendors[strings.ToLower(d.Name)] = true
		}
	}
	var out []device.Status
	for _, d := range all {
		if d.Source == "upower" && vendors[strings.ToLower(d.Name)] {
			continue
		}
		out = append(out, d)
	}
	return out
}

// Notes collects provider diagnostics worth showing in the settings pane.
func (m *Monitor) Notes() []string {
	notes := []string{}
	for _, p := range m.providers {
		if d, ok := p.(interface{ Diagnostics() string }); ok {
			if text := d.Diagnostics(); text != "" {
				notes = append(notes, text)
			}
		}
	}
	return notes
}
