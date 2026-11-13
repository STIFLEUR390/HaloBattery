package razer

import "testing"

func TestBuildRequest(t *testing.T) {
	req := buildRequest(0x1F, 0x07, 0x80)
	if len(req) != 90 {
		t.Fatalf("len = %d, want 90", len(req))
	}
	if req[1] != 0x1F || req[5] != 0x02 || req[6] != 0x07 || req[7] != 0x80 {
		t.Fatalf("header = % x", req[:8])
	}
	var crc byte
	for _, b := range req[2:88] {
		crc ^= b
	}
	if req[88] != crc {
		t.Fatalf("crc = %02x, want %02x", req[88], crc)
	}
}

func TestNormalizeReply(t *testing.T) {
	msg := buildRequest(0x1F, 0x07, 0x80)
	// hidapi returns the report id in front of the payload.
	got := normalizeReply(append([]byte{0x00}, msg...))
	if got == nil || len(got) != 90 || got[6] != 0x07 {
		t.Fatalf("normalizeReply = % x", got)
	}
	if normalizeReply(msg[:40]) != nil {
		t.Fatal("short reply should be dropped")
	}
}
