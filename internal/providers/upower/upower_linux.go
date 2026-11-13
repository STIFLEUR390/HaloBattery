//go:build linux

// Package upower reads batteries UPower already knows about: Bluetooth mice,
// keyboards and headsets are the ones HID vendor protocols cannot reach.
package upower

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"

	"halobattery/internal/device"
	"halobattery/internal/providers"
)

const (
	upowerName  = "org.freedesktop.UPower"
	upowerPath  = dbus.ObjectPath("/org/freedesktop/UPower")
	deviceIface = "org.freedesktop.UPower.Device"
)

// kinds worth showing: mouse, keyboard, gaming input, headset, speakers,
// headphones, video, other audio, remote, camera, wearable, bluetooth generic.
var kinds = map[uint32]string{
	5: "mouse", 6: "keyboard", 12: "gamepad", 17: "headset", 18: "headset",
	19: "headset", 20: "headset", 21: "headset", 22: "", 25: "", 26: "watch",
	27: "",
}

// Provider implements providers.Provider.
type Provider struct {
	mu   sync.Mutex
	conn *dbus.Conn
}

// New returns a UPower-backed provider.
func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "upower" }

func (p *Provider) bus() (*dbus.Conn, error) {
	if p.conn != nil {
		return p.conn, nil
	}
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}
	p.conn = conn
	return conn, nil
}

func (p *Provider) reset() {
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

// Poll enumerates UPower devices and filters out the machine's own battery.
func (p *Provider) Poll() []device.Status {
	p.mu.Lock()
	defer p.mu.Unlock()

	conn, err := p.bus()
	if err != nil {
		return nil
	}
	obj := conn.Object(upowerName, upowerPath)
	var paths []dbus.ObjectPath
	if err := obj.Call("org.freedesktop.UPower.EnumerateDevices", 0).Store(&paths); err != nil {
		p.reset()
		return nil
	}

	var out []device.Status
	for _, path := range paths {
		st, ok := p.read(conn, path)
		if ok {
			out = append(out, st)
		}
	}
	return out
}

func (p *Provider) read(conn *dbus.Conn, path dbus.ObjectPath) (device.Status, bool) {
	obj := conn.Object(upowerName, path)
	var props map[string]dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, deviceIface).Store(&props); err != nil {
		return device.Status{}, false
	}
	get := func(name string) dbus.Variant { return props[name] }

	present, _ := get("IsPresent").Value().(bool)
	if !present {
		return device.Status{}, false
	}
	native, _ := get("NativePath").Value().(string)
	kindCode, _ := get("Kind").Value().(uint32)
	kind, known := kinds[kindCode]
	if !known {
		// Unknown kind: keep it only when it is clearly a Bluetooth peripheral.
		if !strings.HasPrefix(native, "/org/bluez") {
			return device.Status{}, false
		}
		kind = ""
	}

	pct, _ := get("Percentage").Value().(float64)
	var level *int
	if pct >= 0 && pct <= 100 {
		lv := int(math.Round(pct))
		level = &lv
	}

	state, _ := get("State").Value().(uint32)
	charging := state == 1 || state == 4

	name, _ := get("Model").Value().(string)
	if name == "" {
		name, _ = get("Serial").Value().(string)
	}
	if name == "" {
		base := strings.TrimPrefix(string(path), "/org/bluez/")
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		if i := strings.LastIndex(base, "_"); i >= 0 {
			base = base[i+1:]
		}
		name = "Bluetooth device " + base
	}

	return device.Status{
		Key:      "upower:" + strings.ReplaceAll(string(path), "/", ":"),
		Name:     name,
		Level:    level,
		Charging: charging,
		Online:   true,
		Source:   "upower",
		Kind:     kind,
	}, true
}

var _ providers.Provider = (*Provider)(nil)

// Diagnostics reports why UPower is unavailable, for the settings pane.
func (p *Provider) Diagnostics() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := p.bus(); err != nil {
		return fmt.Sprintf("UPower: %v", err)
	}
	return ""
}
