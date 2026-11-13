//go:build linux

// Package bluez reads Bluetooth battery levels from BlueZ's D-Bus API.
// Any device exposing org.bluez.Battery1 (BLE Battery Service) shows up here:
// most BLE mice, keyboards and headsets report their level that way.
package bluez

import (
	"sort"
	"strings"

	"github.com/godbus/dbus/v5"

	"halobattery/internal/device"
	"halobattery/internal/providers"
)

const (
	busName      = "org.bluez"
	objectMgr    = "org.freedesktop.DBus.ObjectManager"
	deviceIface  = "org.bluez.Device1"
	batteryIface = "org.bluez.Battery1"
)

// Provider polls BlueZ for connected devices with a battery interface.
type Provider struct {
	conn *dbus.Conn
	note string
}

// New returns a BlueZ provider.
func New() *Provider { return &Provider{} }

// Name implements providers.Provider.
func (p *Provider) Name() string { return "bluetooth" }

// Poll implements providers.Provider.
func (p *Provider) Poll() []device.Status {
	if p.conn == nil {
		conn, err := dbus.SystemBus()
		if err != nil {
			p.note = "BlueZ unavailable: " + err.Error()
			return nil
		}
		p.conn = conn
	}
	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := p.conn.Object(busName, "/").Call(objectMgr+".GetManagedObjects", 0).Store(&objects); err != nil {
		p.note = "BlueZ unavailable: " + err.Error()
		return nil
	}
	statuses, missing := parseObjects(objects)
	p.note = ""
	if len(missing) > 0 {
		p.note = "Bluetooth, no battery service: " + strings.Join(missing, ", ")
	}
	return statuses
}

// Diagnostics implements the optional monitor interface.
func (p *Provider) Diagnostics() string { return p.note }

func parseObjects(objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant) ([]device.Status, []string) {
	var out []device.Status
	var missing []string
	for path, ifaces := range objects {
		dev, ok := ifaces[deviceIface]
		if !ok || !boolProp(dev, "Connected") {
			continue
		}
		name := stringProp(dev, "Alias")
		if name == "" {
			name = stringProp(dev, "Name")
		}
		if name == "" {
			name = "Bluetooth device"
		}
		var level *int
		if pct, ok := byteProp(ifaces[batteryIface], "Percentage"); ok && pct <= 100 {
			lv := int(pct)
			level = &lv
		} else {
			// connected but no Battery Service: still list it, level unknown
			missing = append(missing, name)
		}
		mac := macOf(string(path))
		out = append(out, device.Status{
			Key:    "bluez:" + mac,
			Name:   name,
			Level:  level,
			Online: true,
			Source: "bluetooth",
			Kind:   kindOf(stringProp(dev, "Icon")),
			Addr:   mac,
		})
	}
	sort.Strings(missing)
	return out, missing
}

// macOf turns /org/bluez/hci0/dev_B9_18_08_9B_97_23 into b9:18:08:9b:97:23.
func macOf(path string) string {
	i := strings.LastIndex(path, "dev_")
	if i < 0 {
		return path
	}
	return strings.ToLower(strings.ReplaceAll(path[i+4:], "_", ":"))
}

func kindOf(icon string) string {
	switch icon {
	case "audio-headset":
		return "headset"
	case "audio-headphones":
		return "headphones"
	case "input-mouse":
		return "mouse"
	case "input-keyboard":
		return "keyboard"
	case "input-gaming":
		return "gamepad"
	case "phone":
		return "phone"
	default:
		return "bluetooth"
	}
}

func boolProp(props map[string]dbus.Variant, key string) bool {
	v, ok := props[key]
	if !ok {
		return false
	}
	b, ok := v.Value().(bool)
	return ok && b
}

func stringProp(props map[string]dbus.Variant, key string) string {
	v, ok := props[key]
	if !ok {
		return ""
	}
	s, _ := v.Value().(string)
	return s
}

func byteProp(props map[string]dbus.Variant, key string) (byte, bool) {
	v, ok := props[key]
	if !ok {
		return 0, false
	}
	b, ok := v.Value().(byte)
	return b, ok
}

var _ providers.Provider = (*Provider)(nil)
