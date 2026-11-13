//go:build linux

package bluez

import (
	"testing"

	"github.com/godbus/dbus/v5"

	"halobattery/internal/device"
)

func dev(props map[string]dbus.Variant) map[string]map[string]dbus.Variant {
	return map[string]map[string]dbus.Variant{"org.bluez.Device1": props}
}

func TestParseObjects(t *testing.T) {
	objects := map[dbus.ObjectPath]map[string]map[string]dbus.Variant{
		"/org/bluez/hci0/dev_B9_18_08_9B_97_23": {
			"org.bluez.Device1": {
				"Alias":     dbus.MakeVariant("oraimo BoomPop Lite"),
				"Connected": dbus.MakeVariant(true),
				"Icon":      dbus.MakeVariant("audio-headset"),
			},
			"org.bluez.Battery1": {"Percentage": dbus.MakeVariant(byte(64))},
		},
		"/org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF": dev(map[string]dbus.Variant{
			"Alias":     dbus.MakeVariant("MX Anywhere 3"),
			"Connected": dbus.MakeVariant(true),
			"Icon":      dbus.MakeVariant("input-mouse"),
		}),
		"/org/bluez/hci0/dev_11_22_33_44_55_66": dev(map[string]dbus.Variant{
			"Alias":     dbus.MakeVariant("Old speaker"),
			"Connected": dbus.MakeVariant(false),
		}),
	}
	got, missing := parseObjects(objects)
	if len(got) != 2 {
		t.Fatalf("got %d statuses: %+v", len(got), got)
	}
	byName := map[string]device.Status{}
	for _, d := range got {
		byName[d.Name] = d
	}
	s, ok := byName["oraimo BoomPop Lite"]
	if !ok {
		t.Fatalf("missing headset: %+v", got)
	}
	if mx := byName["MX Anywhere 3"]; mx.Level != nil {
		t.Fatalf("battery-less device should have no level: %+v", mx)
	}
	if s.Name != "oraimo BoomPop Lite" || s.Level == nil || *s.Level != 64 {
		t.Fatalf("bad status: %+v", s)
	}
	if s.Key != "bluez:b9:18:08:9b:97:23" || s.Addr != "b9:18:08:9b:97:23" || s.Kind != "headset" || s.Source != "bluetooth" {
		t.Fatalf("bad metadata: %+v", s)
	}
	if len(missing) != 1 || missing[0] != "MX Anywhere 3" {
		t.Fatalf("missing = %v", missing)
	}
}
