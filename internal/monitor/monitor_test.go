package monitor

import (
	"testing"

	"halobattery/internal/device"
)

func level(n int) *int { return &n }

func TestMergeDropsUPowerDuplicate(t *testing.T) {
	all := []device.Status{
		{Key: "razer:1", Name: "Razer Viper Ultimate", Source: "razer", Level: level(80), Online: true},
		{Key: "upower:1", Name: "Razer Viper Ultimate", Source: "upower", Level: level(80), Online: true},
		{Key: "upower:2", Name: "Sony WH-1000XM4", Source: "upower", Level: level(60), Online: true},
	}
	got := merge(all)
	if len(got) != 2 {
		t.Fatalf("got %d devices: %+v", len(got), got)
	}
	for _, d := range got {
		if d.Source == "upower" && d.Name == "Razer Viper Ultimate" {
			t.Fatal("upower duplicate kept")
		}
	}
}

func TestMergeKeepsUnknownBrand(t *testing.T) {
	all := []device.Status{
		{Key: "logitech:1", Name: "MX Master 3", Source: "logitech", Level: level(80)},
		{Key: "upower:1", Name: "Razer Viper", Source: "upower", Level: level(70)},
	}
	if got := merge(all); len(got) != 2 {
		t.Fatalf("got %d devices", len(got))
	}
}
