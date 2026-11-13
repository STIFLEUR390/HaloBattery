package logitech

import "testing"

func TestParseUnified(t *testing.T) {
	cases := []struct {
		data     []byte
		level    int
		charging bool
	}{
		{[]byte{85, 4, 0x00, 0}, 85, false},
		{[]byte{85, 4, 0x01, 0}, 85, true},
		{[]byte{0, 4, 0x00, 0}, 50, false}, // coarse "good"
		{[]byte{0, 8, 0x03, 0}, 100, true}, // coarse "full"
		{[]byte{0, 1, 0x00, 0}, 5, false},  // coarse "critical"
	}
	for _, c := range cases {
		level, charging, _, err := parseUnified(c.data)
		if err != nil || level == nil {
			t.Fatalf("% x: %v", c.data, err)
		}
		if *level != c.level || charging != c.charging {
			t.Fatalf("% x: level=%d charging=%v, want %d %v", c.data, *level, charging, c.level, c.charging)
		}
	}
}

func TestParseStatus(t *testing.T) {
	level, charging, _, err := parseStatus([]byte{42, 0, 0x01})
	if err != nil || level == nil || *level != 42 || !charging {
		t.Fatalf("got %v %v %v", level, charging, err)
	}
	if _, _, _, err := parseStatus([]byte{0, 0, 0}); err == nil {
		t.Fatal("zero discharge should be rejected")
	}
}

func TestParseVoltage(t *testing.T) {
	full := []byte{0x10, 0x68, 0x00} // 4200 mV
	if got := parseVoltage(full); got == nil || *got != 100 {
		t.Fatalf("4200 mV -> %d, want 100", *got)
	}
	empty := []byte{0x0D, 0xAC, 0x00} // 3500 mV
	if got := parseVoltage(empty); got == nil || *got != 0 {
		t.Fatalf("3500 mV -> %d, want 0", *got)
	}
}
