//go:build linux

package hid

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ErrTimeout is returned by ReadTimeout when no report arrived in time.
var ErrTimeout = errors.New("hid: read timeout")

type linuxBackend struct{}

// Default returns the backend for this platform.
func Default() Backend { return linuxBackend{} }

func (linuxBackend) Enumerate(vid uint16) ([]Info, error) {
	dirs, err := filepath.Glob("/sys/class/hidraw/hidraw*")
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, dir := range dirs {
		info, ok := readUevent(filepath.Join(dir, "device", "uevent"))
		if !ok {
			continue
		}
		info.Path = "/dev/" + filepath.Base(dir)
		if vid != 0 && info.VID != vid {
			continue
		}
		out = append(out, info)
	}
	return out, nil
}

func readUevent(path string) (Info, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, false
	}
	defer f.Close()
	var info Info
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch k {
		case "HID_ID":
			// bus:vendor:product, vendor and product are 8 hex digits
			parts := strings.Split(v, ":")
			if len(parts) != 3 {
				continue
			}
			if n, err := strconv.ParseUint(parts[1], 16, 32); err == nil {
				info.VID = uint16(n)
			}
			if n, err := strconv.ParseUint(parts[2], 16, 32); err == nil {
				info.PID = uint16(n)
			}
		case "HID_NAME":
			info.Product = v
		case "HID_UNIQ":
			info.Serial = v
		}
	}
	return info, sc.Err() == nil
}

func (linuxBackend) Open(path string) (Device, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &linuxDevice{f: f}, nil
}

type linuxDevice struct {
	f *os.File
}

func featureIoctl(nr uintptr, size int) uintptr {
	// _IOWR('H', nr, size)
	return uintptr(0xC0000000) | uintptr(size)<<16 | uintptr('H')<<8 | nr
}

func (d *linuxDevice) SendFeature(report []byte) error {
	if len(report) == 0 {
		return errors.New("hid: empty feature report")
	}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(),
		featureIoctl(0x06, len(report)), uintptr(unsafe.Pointer(&report[0])))
	if errno != 0 {
		return fmt.Errorf("hid: set feature: %w", errno)
	}
	return nil
}

func (d *linuxDevice) GetFeature(size int) ([]byte, error) {
	if size <= 0 {
		return nil, errors.New("hid: bad feature size")
	}
	buf := make([]byte, size)
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(),
		featureIoctl(0x07, size), uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return nil, fmt.Errorf("hid: get feature: %w", errno)
	}
	return buf, nil
}

func (d *linuxDevice) Write(report []byte) (int, error) {
	return d.f.Write(report)
}

func (d *linuxDevice) ReadTimeout(buf []byte, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return 0, ErrTimeout
		}
		ms := int(left.Milliseconds())
		if ms < 1 {
			ms = 1
		}
		fds := []unix.PollFd{{Fd: int32(d.f.Fd()), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, ms)
		if err != nil && err != syscall.EINTR {
			return 0, err
		}
		if n > 0 {
			return d.f.Read(buf)
		}
	}
}

func (d *linuxDevice) Close() error { return d.f.Close() }
