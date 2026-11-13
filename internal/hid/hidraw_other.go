//go:build !linux

package hid

import "time"

type unsupported struct{}

// Default returns a backend that reports ErrUnsupported.
func Default() Backend { return unsupported{} }

func (unsupported) Enumerate(uint16) ([]Info, error) { return nil, ErrUnsupported }
func (unsupported) Open(string) (Device, error)      { return nil, ErrUnsupported }

// compile-time check: the stubs keep the Device interface honest everywhere.
type stubDevice struct{}

func (stubDevice) SendFeature([]byte) error                       { return ErrUnsupported }
func (stubDevice) GetFeature(int) ([]byte, error)                 { return nil, ErrUnsupported }
func (stubDevice) Write([]byte) (int, error)                      { return 0, ErrUnsupported }
func (stubDevice) ReadTimeout([]byte, time.Duration) (int, error) { return 0, ErrUnsupported }
func (stubDevice) Close() error                                   { return nil }

var _ Device = stubDevice{}
