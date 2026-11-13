// Package hid exposes the small slice of HID access the providers need.
//
// Only Linux is implemented (hidraw). Other platforms compile and run, but
// Enumerate returns ErrUnsupported: the UI still works, it just finds nothing.
package hid

import (
	"errors"
	"time"
)

var ErrUnsupported = errors.New("hid: not supported on this platform")

// Info describes one HID interface node.
type Info struct {
	Path    string
	VID     uint16
	PID     uint16
	Product string
	Serial  string
}

// Device is one open HID interface.
type Device interface {
	// SendFeature writes a feature report. The slice includes the report id.
	SendFeature(report []byte) error
	// GetFeature reads a feature report of size bytes (report id included).
	GetFeature(size int) ([]byte, error)
	// Write sends an output report (report id included).
	Write(report []byte) (int, error)
	// ReadTimeout waits up to timeout for one input report.
	ReadTimeout(buf []byte, timeout time.Duration) (int, error)
	Close() error
}

// Backend lists and opens HID interfaces.
type Backend interface {
	Enumerate(vid uint16) ([]Info, error)
	Open(path string) (Device, error)
}
