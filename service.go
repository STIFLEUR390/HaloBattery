package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"halobattery/internal/config"
	"halobattery/internal/device"
	"halobattery/internal/history"
	"halobattery/internal/i18n"
	"halobattery/internal/monitor"
)

const version = "1.1.0-go"

// State is everything the UI needs in one snapshot.
type State struct {
	Devices       []device.Status  `json:"devices"`
	HiddenDevices []device.Status  `json:"hidden_devices"`
	Config        config.Config    `json:"config"`
	Providers     []ProviderInfo   `json:"providers"`
	History       map[string]Trend `json:"history"`
	Version       string           `json:"version"`
	Platform      string           `json:"platform"`
	Updated       time.Time        `json:"updated"`
	Notes         []string         `json:"notes"`
}

// Trend is the recorded battery curve of one device.
type Trend struct {
	Levels  []int `json:"levels"`
	Minutes int   `json:"minutes"` // estimated runtime left, -1 when unknown
}

// ProviderInfo describes one battery source for the settings pane.
type ProviderInfo struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// Service is the Wails-bound app service: the methods below are callable
// from the frontend as HaloService.*.
type Service struct {
	app      *application.App
	store    *config.Store
	notifier *notifications.NotificationService

	mon  *monitor.Monitor
	hist *history.Store

	mu        sync.Mutex
	cfg       config.Config
	updated   time.Time
	lowSent   map[string]bool
	fullSent  map[string]bool
	wasBelow  map[string]bool
	refreshUI func()
	quit      func()
	quitting  bool

	started bool
}

// NewService builds the service; call Start or let ServiceStartup do it.
func NewService(app *application.App, store *config.Store, notifier *notifications.NotificationService) *Service {
	return &Service{
		app:      app,
		store:    store,
		notifier: notifier,
		cfg:      store.Load(),
		lowSent:  map[string]bool{},
		fullSent: map[string]bool{},
		wasBelow: map[string]bool{},
	}
}

// ServiceStartup is called by Wails when the app starts.
func (s *Service) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	s.start()
	return nil
}

// Start launches the polling loop once.
func (s *Service) start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	cfg := s.cfg
	if s.hist == nil {
		s.hist = history.DefaultStore()
	}
	s.mu.Unlock()

	s.mon = monitor.New(monitor.DefaultProviders(), time.Duration(cfg.Interval)*time.Second, s.onDevices)
	s.mon.SetDisabled(cfg.Disabled)
	go s.mon.Start()
}

// setUITrayHook lets main rebuild the tray after each poll.
func (s *Service) setUITrayHook(fn func()) { s.refreshUI = fn }

// isQuitting reports whether the user chose Quit (window close only hides).
func (s *Service) isQuitting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.quitting
}

// devices returns the latest snapshot for the tray.
func (s *Service) devices() []device.Status {
	if s.mon == nil {
		return nil
	}
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	return visible(cfg, s.mon.Devices())
}

// visible applies the custom names and drops the devices the user hid.
func visible(cfg config.Config, devs []device.Status) []device.Status {
	hidden := map[string]bool{}
	for _, key := range cfg.Hidden {
		hidden[key] = true
	}
	out := make([]device.Status, 0, len(devs))
	for _, d := range devs {
		if hidden[d.Key] {
			continue
		}
		if name := cfg.Names[d.Key]; name != "" {
			d.Name = name
		}
		out = append(out, d)
	}
	return out
}

// hiddenOnly returns the devices the user hid, for the settings pane.
func hiddenOnly(cfg config.Config, devs []device.Status) []device.Status {
	hidden := map[string]bool{}
	for _, key := range cfg.Hidden {
		hidden[key] = true
	}
	out := []device.Status{}
	for _, d := range devs {
		if !hidden[d.Key] {
			continue
		}
		if name := cfg.Names[d.Key]; name != "" {
			d.Name = name
		}
		out = append(out, d)
	}
	return out
}

// lang is the concrete interface language.
func (s *Service) lang() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return i18n.Resolve(s.cfg.Language)
}

// low returns the low-battery threshold.
func (s *Service) low() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Low
}

func (s *Service) notes() []string {
	if s.mon == nil {
		return nil
	}
	return s.mon.Notes()
}

func (s *Service) onDevices(devs []device.Status) {
	s.mu.Lock()
	s.updated = time.Now()
	cfg := s.cfg
	s.mu.Unlock()

	if s.hist != nil {
		s.hist.Add(devs)
	}
	shown := visible(cfg, devs)
	s.app.Event.Emit("devices", shown)
	s.alert(shown, cfg)
	if s.refreshUI != nil {
		s.refreshUI()
	}
}

// alert notifies once per crossing: low while discharging, full while charging.
func (s *Service) alert(devs []device.Status, cfg config.Config) {
	if !cfg.Notify || s.notifier == nil {
		return
	}
	lang := i18n.Resolve(cfg.Language)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range devs {
		if d.Level == nil {
			continue
		}
		low := cfg.Low
		if v, ok := cfg.Alerts[d.Key]; ok {
			low = v
		}
		level := *d.Level
		if level <= low && !d.Charging {
			if !s.lowSent[d.Key] {
				s.lowSent[d.Key] = true
				s.notify(d.Key+"-low",
					i18n.T(lang, "notify.low_title"),
					i18n.Tf(lang, "notify.low_body", d.Name, level))
			}
		} else if level > low+5 || d.Charging {
			s.lowSent[d.Key] = false
		}

		if d.Charging && level < 100 {
			s.wasBelow[d.Key] = true
		}
		if d.Charging && level >= 100 && s.wasBelow[d.Key] {
			s.wasBelow[d.Key] = false
			if !s.fullSent[d.Key] {
				s.fullSent[d.Key] = true
				s.notify(d.Key+"-full",
					i18n.T(lang, "notify.full_title"),
					i18n.Tf(lang, "notify.full_body", d.Name))
			}
		}
		if !d.Charging {
			s.fullSent[d.Key] = false
		}
	}
}

func (s *Service) notify(id, title, body string) {
	if err := s.notifier.SendNotification(notifications.NotificationOptions{
		ID: id, Title: title, Body: body,
	}); err != nil && s.app != nil && s.app.Logger != nil {
		s.app.Logger.Warn("notification failed", "err", err)
	}
}

// GetState returns the current snapshot for the UI.
func (s *Service) GetState() State {
	s.mu.Lock()
	cfg, updated, histStore := s.cfg, s.updated, s.hist
	s.mu.Unlock()

	all := []device.Status{}
	if s.mon != nil {
		all = s.mon.Devices()
	}
	providers := []ProviderInfo{}
	if s.mon != nil {
		for _, name := range s.mon.ProviderNames() {
			providers = append(providers, ProviderInfo{Name: name, Enabled: !contains(cfg.Disabled, name)})
		}
	}
	devs := []device.Status{}
	if s.mon != nil {
		devs = visible(cfg, all)
	}
	trends := map[string]Trend{}
	for _, d := range devs {
		levels, minutes := histStore.Trend(d.Key)
		if len(levels) > 0 {
			trends[d.Key] = Trend{Levels: levels, Minutes: minutes}
		}
	}
	return State{
		Devices: devs, HiddenDevices: hiddenOnly(cfg, all), Config: cfg,
		Providers: providers, History: trends,
		Version: version, Platform: platform(), Updated: updated,
		Notes: s.notes(),
	}
}

// RefreshNow forces an immediate poll (the button in the UI).
func (s *Service) RefreshNow() {
	if s.mon != nil {
		go s.mon.PollNow()
	}
}

// SaveConfig validates, persists and applies settings.
func (s *Service) SaveConfig(cfg config.Config) error {
	cfg.Clamp()
	if err := s.store.Save(cfg); err != nil {
		return err
	}
	s.mu.Lock()
	old := s.cfg
	s.cfg = cfg
	s.mu.Unlock()

	if s.mon != nil {
		s.mon.SetInterval(time.Duration(cfg.Interval) * time.Second)
		s.mon.SetDisabled(cfg.Disabled)
	}
	if cfg.Autostart != old.Autostart {
		if cfg.Autostart {
			if err := s.app.Autostart.Enable(); err != nil {
				return fmt.Errorf("autostart: %w", err)
			}
		} else if err := s.app.Autostart.Disable(); err != nil {
			return fmt.Errorf("autostart: %w", err)
		}
	}
	return nil
}

// Quit exits the app (window close only hides it).
func (s *Service) Quit() {
	s.mu.Lock()
	s.quitting = true
	s.mu.Unlock()
	if s.quit != nil {
		s.quit()
	}
	s.app.Quit()
}

// ShowWindow brings the dashboard back from the tray.
func (s *Service) ShowWindow() {
	if w, ok := s.app.Window.GetByName("main"); ok {
		w.Show()
		w.Focus()
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
