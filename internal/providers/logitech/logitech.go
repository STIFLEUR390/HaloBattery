// Package logitech reads Logitech wireless devices through HID++ 2.0 (and
// falls back to HID++ 1.0 registers), following Solaar's protocol notes.
//
//	short report: 0x10, 7 bytes   -> receiver slots and HID++ 1.0
//	long report:  0x11, 20 bytes  -> HID++ 2.0 devices and Bluetooth
//	request:  [report, slot, featureIndex, function<<4|swid, params...]
//	reply:    same header, then params
//	error:    feature 0xFF, then [feature, function, errorCode]
package logitech

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"halobattery/internal/device"
	"halobattery/internal/hid"
	"halobattery/internal/providers"
)

const logitechVID = 0x046D

const (
	swid       = 0x08
	reqTimeout = 300 * time.Millisecond
	pingTimout = 250 * time.Millisecond

	featRoot       = 0x0000
	featDeviceName = 0x0005
	featBattery    = 0x1000
	featVoltage    = 0x1001
	featUnified    = 0x1004

	regBatteryCharge = 0x0D
)

var errNoReply = errors.New("logitech: no reply")

type replyError struct {
	feature byte
	code    byte
}

func (e replyError) Error() string {
	return fmt.Sprintf("logitech: feature %02x error %d", e.feature, e.code)
}

type route struct {
	path     string
	long     bool
	features map[uint16]byte // feature id -> index
	names    map[byte]string
}

type lastValue struct {
	level    int
	charging bool
	at       time.Time
}

// Provider implements providers.Provider.
type Provider struct {
	backend hid.Backend
	routes  map[string]*route
	last    map[string]lastValue
}

// New returns a Logitech provider.
func New(backend hid.Backend) *Provider {
	return &Provider{backend: backend, routes: map[string]*route{}, last: map[string]lastValue{}}
}

func (p *Provider) Name() string { return "logitech" }

// call opens the interface, sends one request and returns the reply params.
func (p *Provider) call(path string, long bool, slot, feature, fn byte, params []byte) ([]byte, error) {
	dev, err := p.backend.Open(path)
	if err != nil {
		return nil, err
	}
	defer dev.Close()

	size := 7
	reportID := byte(0x10)
	if long {
		size = 20
		reportID = 0x11
	}
	frame := make([]byte, size)
	frame[0] = reportID
	frame[1] = slot
	frame[2] = feature
	fnByte := (fn & 0xF0) | swid
	if feature == 0x81 {
		fnByte = fn // HID++ 1.0 register traffic keeps its own sub-id
	}
	frame[3] = fnByte
	copy(frame[4:], params)
	if _, err := dev.Write(frame); err != nil {
		return nil, err
	}

	buf := make([]byte, size)
	deadline := time.Now().Add(reqTimeout)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return nil, errNoReply
		}
		n, err := dev.ReadTimeout(buf, left)
		if err != nil {
			if errors.Is(err, hid.ErrTimeout) {
				return nil, errNoReply
			}
			return nil, err
		}
		r := buf[:n]
		if len(r) < 4 || r[1] != slot {
			continue
		}
		if r[2] == 0xFF || r[2] == 0x8F {
			if len(r) >= 6 && r[3] == feature && r[4] == fnByte {
				return nil, replyError{feature: feature, code: r[5]}
			}
			continue
		}
		if r[2] != feature || r[3] != fnByte {
			continue
		}
		return r[4:], nil
	}
}

func ping(p *Provider, r *route, slot byte) (float64, error) {
	params := []byte{0x00, 0x00, 0xAA}
	data, err := p.call(r.path, r.long, slot, featRoot, 0x10, params)
	if err != nil {
		return 0, err
	}
	if len(data) < 4 {
		return 0, errNoReply
	}
	return float64(data[2]) + float64(data[3])/10, nil
}

func featureIndex(p *Provider, r *route, slot byte, id uint16) (byte, error) {
	if idx, ok := r.features[id]; ok {
		return idx, nil
	}
	data, err := p.call(r.path, r.long, slot, featRoot, 0x00, []byte{byte(id >> 8), byte(id)})
	if err != nil {
		return 0, err
	}
	if len(data) == 0 || data[0] == 0 {
		return 0, errors.New("logitech: feature unsupported")
	}
	r.features[id] = data[0]
	return data[0], nil
}

// battery returns level (nil when unknown), charging and a coarse-level hint.
func (p *Provider) battery(r *route, slot byte, protocol float64) (*int, bool, string, error) {
	if protocol < 2.0 {
		return p.legacyBattery(r, slot)
	}
	if idx, err := featureIndex(p, r, slot, featUnified); err == nil {
		if data, err := p.call(r.path, r.long, slot, idx, 0x10, nil); err == nil && len(data) >= 3 {
			return parseUnified(data)
		}
	}
	if idx, err := featureIndex(p, r, slot, featBattery); err == nil {
		if data, err := p.call(r.path, r.long, slot, idx, 0x00, nil); err == nil && len(data) >= 3 {
			return parseStatus(data)
		}
	}
	if idx, err := featureIndex(p, r, slot, featVoltage); err == nil {
		if data, err := p.call(r.path, r.long, slot, idx, 0x00, nil); err == nil && len(data) >= 3 {
			return parseVoltage(data), false, "", nil
		}
	}
	return nil, false, "", errNoReply
}

func (p *Provider) legacyBattery(r *route, slot byte) (*int, bool, string, error) {
	data, err := p.call(r.path, false, slot, 0x81, regBatteryCharge, nil)
	if err != nil || len(data) < 3 {
		return nil, false, "", errNoReply
	}
	level := int(data[0])
	status := data[2] & 0xF0
	charging := status == 0x50 || status == 0x90
	return &level, charging, "", nil
}

func parseUnified(data []byte) (*int, bool, string, error) {
	discharge, level, status := int(data[0]), int(data[1]), int(data[2])
	charging := status > 0 && status <= 4
	approx := ""
	switch {
	case discharge > 0 && discharge <= 100:
		return &discharge, charging, "", nil
	case level == 8:
		discharge, approx = 100, "approximate"
	case level == 4:
		discharge, approx = 50, "approximate"
	case level == 2:
		discharge, approx = 20, "approximate"
	case level == 1:
		discharge, approx = 5, "approximate"
	default:
		discharge, approx = 0, "approximate"
	}
	return &discharge, charging, approx, nil
}

func parseStatus(data []byte) (*int, bool, string, error) {
	discharge, status := int(data[0]), int(data[2])
	if discharge <= 0 || discharge > 100 {
		return nil, false, "", errNoReply
	}
	return &discharge, status > 0 && status <= 4, "", nil
}

// voltageToPercent uses Logitech's own discharge curve, as Solaar does.
var voltageToPercent = [][2]int{
	{4186, 100}, {4067, 90}, {3989, 80}, {3922, 70}, {3859, 60}, {3811, 50},
	{3778, 40}, {3751, 30}, {3717, 20}, {3671, 10}, {3646, 5}, {3579, 2}, {3500, 0},
}

func parseVoltage(data []byte) *int {
	mv := int(data[0])<<8 | int(data[1])
	flags := data[2]
	level := 0 // below the first table entry
	if flags&0x80 != 0 && flags&0x03 == 0x01 {
		level = 100
	}
	for _, point := range voltageToPercent {
		if point[0] < mv {
			level = point[1]
			break
		}
	}
	return &level
}

func (p *Provider) deviceName(r *route, slot byte) string {
	if name, ok := r.names[slot]; ok {
		return name
	}
	idx, err := featureIndex(p, r, slot, featDeviceName)
	if err != nil {
		return ""
	}
	data, err := p.call(r.path, r.long, slot, idx, 0x00, nil)
	if err != nil || len(data) == 0 {
		return ""
	}
	length := int(data[0])
	var name []byte
	for len(name) < length && len(name) < 64 {
		chunk, err := p.call(r.path, r.long, slot, idx, 0x10, []byte{byte(len(name))})
		if err != nil || len(chunk) == 0 {
			break
		}
		n := chunk
		if n[0] == 0 {
			n = n[1:]
		}
		name = append(name, n...)
	}
	if len(name) > length {
		name = name[:length]
	}
	result := strings.TrimSpace(string(name))
	r.names[slot] = result
	return result
}

func isReceiver(pid uint16) bool { return pid >= 0xC500 && pid < 0xC600 }

func nameKind(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "keyboard"):
		return "keyboard"
	case strings.Contains(n, "headset"), strings.Contains(n, "headphone"):
		return "headset"
	case strings.Contains(n, "gamepad"):
		return "gamepad"
	case strings.Contains(n, "mouse"), strings.Contains(n, "mx "):
		return "mouse"
	}
	return ""
}

// probe finds a path and report length that answers on this receiver/device.
func (p *Provider) probe(infos []hid.Info, receiver bool) *route {
	sorted := append([]hid.Info(nil), infos...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	slot := byte(0xFF)
	if receiver {
		slot = 1
	}
	for _, info := range sorted {
		for _, long := range []bool{true, false} {
			r := &route{path: info.Path, long: long, features: map[uint16]byte{}, names: map[byte]string{}}
			if _, err := ping(p, r, slot); err == nil {
				return r
			}
		}
	}
	return nil
}

// Poll enumerates Logitech interfaces, finds receivers and reads every slot.
func (p *Provider) Poll() []device.Status {
	infos, err := p.backend.Enumerate(logitechVID)
	if err != nil {
		return nil
	}

	type groupKey struct {
		pid    uint16
		serial string
	}
	groups := map[groupKey][]hid.Info{}
	for _, info := range infos {
		groups[groupKey{info.PID, info.Serial}] = append(groups[groupKey{info.PID, info.Serial}], info)
	}

	var out []device.Status
	for key, ifaces := range groups {
		keyStr := fmt.Sprintf("logitech:%04x:%s", key.pid, key.serial)
		r := p.routes[keyStr]
		if r == nil {
			r = p.probe(ifaces, isReceiver(key.pid))
			if r == nil {
				continue
			}
			p.routes[keyStr] = r
		}

		var slots []byte
		if isReceiver(key.pid) {
			slots = []byte{1, 2, 3, 4, 5, 6}
		} else {
			slots = []byte{0xFF}
		}

		for _, slot := range slots {
			protocol, err := ping(p, r, slot)
			if err != nil {
				continue
			}
			level, charging, approx, err := p.battery(r, slot, protocol)
			devKey := fmt.Sprintf("%s:%d", keyStr, slot)
			if err != nil {
				if last, ok := p.last[devKey]; ok && time.Since(last.at) < 3*time.Minute {
					lv := last.level
					out = append(out, device.Status{
						Key: devKey, Name: p.nameFor(r, slot, key.pid, ifaces),
						Level: &lv, Charging: last.charging, Online: false,
						Source: "logitech", Kind: "", Approx: "last seen",
					})
				}
				continue
			}
			if level != nil {
				p.last[devKey] = lastValue{*level, charging, time.Now()}
			}
			name := p.nameFor(r, slot, key.pid, ifaces)
			out = append(out, device.Status{
				Key: devKey, Name: name, Level: level, Charging: charging, Online: true,
				Source: "logitech", Kind: nameKind(name), Approx: approx,
			})
		}
	}
	return out
}

func (p *Provider) nameFor(r *route, slot byte, pid uint16, infos []hid.Info) string {
	if name := p.deviceName(r, slot); name != "" {
		return name
	}
	if len(infos) > 0 && infos[0].Product != "" {
		return infos[0].Product
	}
	return fmt.Sprintf("Logitech %04x", pid)
}

var _ providers.Provider = (*Provider)(nil)
