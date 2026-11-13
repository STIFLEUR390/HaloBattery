// Package providers defines the battery provider contract shared by the
// Razer, Logitech and UPower implementations.
package providers

import "halobattery/internal/device"

// Provider finds its devices and reads their battery level.
type Provider interface {
	Name() string
	Poll() []device.Status
}
