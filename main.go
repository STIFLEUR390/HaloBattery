package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"halobattery/internal/config"
	"halobattery/internal/device"
	"halobattery/internal/icons"
	"halobattery/internal/monitor"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/logo.png
var appLogo []byte

func init() {
	application.RegisterEvent[[]device.Status]("devices")
}

func main() {
	probe := flag.Bool("probe", false, "print detected devices as JSON and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("HaloBattery", version)
		return
	}
	if *probe {
		runProbe()
		return
	}

	store := config.DefaultStore()
	cfg := store.Load()
	ns := notifications.New()

	svc := &Service{
		store:    store,
		notifier: ns,
		cfg:      cfg,
		lowSent:  map[string]bool{},
		fullSent: map[string]bool{},
		wasBelow: map[string]bool{},
	}

	app := application.New(application.Options{
		Name:        "HaloBattery",
		Description: "Battery levels for wireless mice, keyboards and headsets",
		Icon:        appLogo,
		Services: []application.Service{
			application.NewService(ns),
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})
	svc.app = app

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "HaloBattery",
		Width:            460,
		Height:           660,
		MinWidth:         380,
		MinHeight:        520,
		BackgroundColour: application.NewRGB(18, 20, 26),
		URL:              "/",
	})
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if svc.isQuitting() {
			return
		}
		e.Cancel()
		window.Hide()
	})

	tray := app.SystemTray.New()
	tray.SetTooltip("HaloBattery")
	tray.OnClick(func() { svc.ShowWindow() })
	tray.SetMenu(buildTrayMenu(app, svc))
	tray.SetIcon(trayIcon(svc))

	svc.setUITrayHook(func() {
		application.InvokeAsync(func() {
			tray.SetMenu(buildTrayMenu(app, svc))
			tray.SetIcon(trayIcon(svc))
			tray.SetTooltip(trayTooltip(svc))
		})
	})

	if cfg.Autostart {
		if err := app.Autostart.Enable(); err != nil {
			app.Logger.Warn("autostart", "err", err)
		}
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func runProbe() {
	ps := monitor.DefaultProviders()
	m := monitor.New(ps, time.Minute, nil)
	m.PollNow()
	raw, err := json.MarshalIndent(struct {
		Devices []device.Status `json:"devices"`
		Notes   []string        `json:"notes"`
	}{m.Devices(), m.Notes()}, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(raw))
}

func buildTrayMenu(app *application.App, svc *Service) *application.Menu {
	menu := app.NewMenu()
	devs := svc.devices()
	if len(devs) == 0 {
		item := menu.Add("No device detected")
		item.SetEnabled(false)
	}
	for i, d := range devs {
		if i >= 8 {
			break
		}
		menu.Add(deviceLabel(d)).OnClick(func(*application.Context) { svc.ShowWindow() })
	}
	menu.AddSeparator()
	menu.Add("Open dashboard").OnClick(func(*application.Context) { svc.ShowWindow() })
	menu.Add("Refresh now").OnClick(func(*application.Context) { svc.RefreshNow() })
	menu.AddSeparator()
	menu.Add("Quit HaloBattery").OnClick(func(*application.Context) { svc.Quit() })
	return menu
}

func deviceLabel(d device.Status) string {
	level := "--"
	if d.Level != nil {
		level = fmt.Sprintf("%d%%", *d.Level)
	}
	state := ""
	if !d.Online {
		state = " (asleep)"
	} else if d.Charging {
		state = " (charging)"
	}
	return fmt.Sprintf("%s  %s%s", level, d.Name, state)
}

// trayIcon shows the lowest battery among online devices.
func trayIcon(svc *Service) []byte {
	var lowest *device.Status
	for _, d := range svc.devices() {
		if !d.Online || d.Level == nil {
			continue
		}
		if lowest == nil || *d.Level < *lowest.Level {
			d := d
			lowest = &d
		}
	}
	if lowest == nil {
		return icons.PNG(nil, false, svc.low())
	}
	return icons.PNG(lowest.Level, lowest.Charging, svc.low())
}

func trayTooltip(svc *Service) string {
	n := 0
	for _, d := range svc.devices() {
		if d.Online {
			n++
		}
	}
	return fmt.Sprintf("HaloBattery — %d device(s)", n)
}

func platform() string { return runtime.GOOS }
