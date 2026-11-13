//go:build linux

package monitor

import (
	"halobattery/internal/hid"
	"halobattery/internal/providers"
	"halobattery/internal/providers/bluez"
	"halobattery/internal/providers/logitech"
	"halobattery/internal/providers/razer"
	"halobattery/internal/providers/upower"
)

// DefaultProviders is the provider set used by the app on Linux.
func DefaultProviders() []providers.Provider {
	backend := hid.Default()
	return []providers.Provider{
		razer.New(backend),
		logitech.New(backend),
		bluez.New(),
		upower.New(),
	}
}
