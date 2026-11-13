// Package device holds the types shared by every battery provider.
package device

// Status is one device reading, mirroring DeviceStatus in the Python app.
type Status struct {
	Key      string `json:"key"`            // stable identifier, one row per key
	Name     string `json:"name"`           // human-readable name
	Level    *int   `json:"level"`          // 0..100, nil = unknown
	Charging bool   `json:"charging"`       // charger detected
	Online   bool   `json:"online"`         // false = receiver present, device asleep/off
	Source   string `json:"source"`         // razer / logitech / upower
	Kind     string `json:"kind"`           // mouse / keyboard / headset / gamepad / ""
	Approx   string `json:"approx"`         // set when only a coarse level is known
	Addr     string `json:"addr,omitempty"` // hardware address when the source has one
}
