//go:build !linux

package monitor

import (
	"halobattery/internal/hid"
	"halobattery/internal/providers"
	"halobattery/internal/providers/logitech"
	"halobattery/internal/providers/razer"
)

// DefaultProviders is the provider set used outside Linux. HID access is not
// implemented yet there, so the app runs but discovers nothing.
func DefaultProviders() []providers.Provider {
	backend := hid.Default()
	return []providers.Provider{
		razer.New(backend),
		logitech.New(backend),
	}
}
