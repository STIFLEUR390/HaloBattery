// Package razer reads Razer wireless mice, keyboards and headsets.
//
// Protocol: a 90-byte feature report (report id 0), the same one Synapse and
// OpenRazer use. See providers/razer.py in the Python project.
//
//	[0]  status        (0x02 ok, 0x01 busy, 0x04 timeout/asleep, 0x05 unsupported)
//	[1]  transaction id
//	[5]  data size
//	[6]  command class (0x07 = power)
//	[7]  command id    (0x80 = level, 0x84 = charging)
//	[9]  value         (level 0..255, charging 0/1)
//	[88] crc = XOR of bytes 2..87
package razer

import (
	"fmt"
	"math"
	"sort"
	"time"

	"halobattery/internal/device"
	"halobattery/internal/hid"
	"halobattery/internal/providers"
)

const razerVID = 0x1532

const (
	statusOK           = 0x02
	statusBusy         = 0x01
	statusTimeout      = 0x04
	statusNotSupported = 0x05
	// statusForeign is not a device status: every packet read back belonged to
	// another command, i.e. to another app talking to the same collection.
	statusForeign = 0x100
)

const (
	asleepKeep = 5 * time.Minute
	sends      = 3
)

type deviceInfo struct {
	name string
	tid  byte
}

type candidate struct {
	path string
	tid  byte
}

type lastValue struct {
	level    int
	charging bool
	at       time.Time
}

// Provider implements providers.Provider.
type Provider struct {
	backend hid.Backend
	cache   map[string]*candidate
	good    map[string]*candidate
	dead    map[string]time.Time
	last    map[string]lastValue
}

// New returns a Razer provider using the given HID backend (hid.Default() in production).
func New(backend hid.Backend) *Provider {
	return &Provider{
		backend: backend,
		cache:   map[string]*candidate{},
		good:    map[string]*candidate{},
		dead:    map[string]time.Time{},
		last:    map[string]lastValue{},
	}
}

func (p *Provider) Name() string { return "razer" }

func buildRequest(tid, class, id byte) []byte {
	msg := make([]byte, 90)
	msg[1] = tid
	msg[5] = 0x02
	msg[6] = class
	msg[7] = id
	var crc byte
	for _, b := range msg[2:88] {
		crc ^= b
	}
	msg[88] = crc
	return msg
}

func normalizeReply(resp []byte) []byte {
	if len(resp) >= 91 {
		resp = resp[1:91]
	}
	if len(resp) < 90 {
		return nil
	}
	return resp
}

// query returns (status, value). value is nil when the reply is for another command.
func query(dev hid.Device, tid, class, id byte) (int, *int) {
	req := buildRequest(tid, class, id)
	send := func() bool {
		report := append([]byte{0x00}, req...)
		return dev.SendFeature(report) == nil
	}
	if !send() {
		return 0, nil
	}
	time.Sleep(60 * time.Millisecond)
	deadline := time.Now().Add(time.Second)
	sent := 1
	foreign := 0
	for {
		resp, err := dev.GetFeature(91)
		if err != nil {
			return 0, nil
		}
		data := normalizeReply(resp)
		if data == nil {
			return 0, nil
		}
		status := int(data[0])
		var value *int
		if data[6] == class && data[7] == id {
			v := int(data[9])
			value = &v
		}
		isForeign := status == statusOK && value == nil
		if time.Now().Before(deadline) && (status == statusBusy || isForeign) {
			if isForeign {
				foreign++
				if foreign%4 == 0 && sent < sends {
					if !send() {
						return 0, nil
					}
					sent++
				}
			}
			time.Sleep(80 * time.Millisecond)
			continue
		}
		if isForeign {
			return statusForeign, nil
		}
		return status, value
	}
}

// read returns (status, level, charging). A zero status means the interface
// could not be used at all (open or feature report failure).
func (p *Provider) read(path string, tid byte) (int, *int, bool) {
	dev, err := p.backend.Open(path)
	if err != nil {
		return 0, nil, false
	}
	defer dev.Close()

	status, raw := query(dev, tid, 0x07, 0x80)
	if status != statusOK || raw == nil {
		return status, nil, false
	}
	if *raw > 255 {
		return statusNotSupported, nil, false
	}
	level := int(math.Round(float64(*raw) / 255 * 100))
	charging := false
	if st, v := query(dev, tid, 0x07, 0x84); st == statusOK && v != nil {
		charging = *v != 0
	}
	return statusOK, &level, charging
}

func maybeWireless(pid uint16, name string) bool {
	if _, ok := known[pid]; ok {
		return true
	}
	if paPIDs[pid] {
		return true
	}
	for _, w := range []string{"hyperspeed", "wireless", "receiver", "dongle", "dock", "blackshark", "barracuda", "nari"} {
		if containsFold(name, w) {
			return true
		}
	}
	return false
}

func containsFold(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexFold(s, sub) >= 0)
}

func indexFold(s, sub string) int {
	ls, lsub := lower(s), lower(sub)
	for i := 0; i+len(lsub) <= len(ls); i++ {
		if ls[i:i+len(lsub)] == lsub {
			return i
		}
	}
	return -1
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

type groupKey struct {
	pid    uint16
	serial string
}

func (p *Provider) pollGroup(key groupKey, ifaces []hid.Info, prefTid byte) (int, *int, bool) {
	keyStr := fmt.Sprintf("%04x:%s", key.pid, key.serial)
	if c := p.cache[keyStr]; c != nil {
		status, level, charging := p.read(c.path, c.tid)
		if status == statusOK {
			p.good[keyStr] = c
		}
		if status == statusOK || status == statusTimeout || status == statusForeign {
			return status, level, charging
		}
		delete(p.cache, keyStr)
	}

	sorted := append([]hid.Info(nil), ifaces...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	tids := []byte{}
	if prefTid != 0 {
		tids = append(tids, prefTid)
	}
	for _, t := range transactionIDs {
		dup := false
		for _, have := range tids {
			if have == t {
				dup = true
				break
			}
		}
		if !dup {
			tids = append(tids, t)
		}
	}

	now := time.Now()
	good := p.good[keyStr]
	var timeout *struct {
		status   int
		level    *int
		charging bool
	}
	for _, info := range sorted {
		proven := good != nil && info.Path == good.path
		if !proven && p.dead[info.Path].After(now) {
			continue
		}
		answered := false
		for _, tid := range tids {
			status, level, charging := p.read(info.Path, tid)
			if status == 0 {
				break // this interface rejects feature reports
			}
			answered = true
			if status == statusOK {
				p.good[keyStr] = &candidate{path: info.Path, tid: tid}
				p.cache[keyStr] = &candidate{path: info.Path, tid: tid}
				return status, level, charging
			}
			if status == statusTimeout {
				timeout = &struct {
					status   int
					level    *int
					charging bool
				}{status, nil, false}
				if proven {
					p.cache[keyStr] = &candidate{path: good.path, tid: good.tid}
					return status, nil, false
				}
				continue
			}
			if status == statusForeign {
				p.cache[keyStr] = &candidate{path: info.Path, tid: tid}
				return status, nil, false
			}
		}
		if !answered && !proven {
			p.dead[info.Path] = now.Add(5 * time.Minute)
		}
	}
	if timeout != nil {
		return timeout.status, timeout.level, timeout.charging
	}
	return statusTimeout, nil, false
}

// Poll reads every Razer interface present now.
func (p *Provider) Poll() []device.Status {
	infos, err := p.backend.Enumerate(razerVID)
	if err != nil {
		return nil
	}

	groups := map[groupKey][]hid.Info{}
	for _, info := range infos {
		groups[groupKey{info.PID, info.Serial}] = append(groups[groupKey{info.PID, info.Serial}], info)
	}

	// Windows re-parents some collections with an empty serial (RZVIRTUAL).
	// Attach them to the real device of the same PID when there is exactly one.
	named := map[uint16][]string{}
	for k := range groups {
		if k.serial != "" {
			named[k.pid] = append(named[k.pid], k.serial)
		}
	}
	for k := range groups {
		if k.serial == "" && len(named[k.pid]) == 1 {
			target := groupKey{k.pid, named[k.pid][0]}
			groups[target] = append(groups[target], groups[k]...)
			delete(groups, k)
		}
	}

	var out []device.Status
	for key, ifaces := range groups {
		info, isKnown := known[key.pid]
		name := info.name
		if !isKnown {
			name = ifaces[0].Product
			if name == "" {
				name = fmt.Sprintf("Razer %04x", key.pid)
			}
		}
		keyStr := fmt.Sprintf("razer:%04x:%s", key.pid, key.serial)
		kind := ""
		if _, ok := keyboardInterface[key.pid]; ok {
			kind = "keyboard"
		}
		if key.pid == 0x0552 || key.pid == 0x053A {
			continue // Barracuda X / Pro: own protocols, not implemented here
		}
		if !maybeWireless(key.pid, name) {
			continue
		}
		status, level, charging := p.pollGroup(key, ifaces, info.tid)
		if status != statusOK {
			if last, ok := p.last[keyStr]; ok && time.Since(last.at) < asleepKeep {
				lv := last.level
				out = append(out, device.Status{
					Key: keyStr, Name: name, Level: &lv, Charging: last.charging,
					Online: false, Source: "razer", Kind: kind,
				})
			}
			continue
		}
		if level != nil {
			p.last[keyStr] = lastValue{*level, charging, time.Now()}
		}
		out = append(out, device.Status{
			Key: keyStr, Name: name, Level: level, Charging: charging,
			Online: true, Source: "razer", Kind: kind,
		})
	}
	return out
}

var _ providers.Provider = (*Provider)(nil)
