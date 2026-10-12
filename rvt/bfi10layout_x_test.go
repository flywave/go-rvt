package rvt

import (
	"encoding/binary"
	"os"
	"testing"
	"unicode/utf16"
)

// TestXBFI10Layout: walk the rev-10 BFI layout to map fields (R58 accept table).
func TestXBFI10Layout(t *testing.T) {
	data, err := os.ReadFile(richSample)
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	b, ok := func() ([]byte, bool) {
		v, err := f.Stream("BasicFileInfo")
		return v, err == nil
	}()
	if !ok {
		t.Skip("no BFI")
	}
	rev := binary.LittleEndian.Uint32(b)
	t.Logf("revision=%d total=%d", rev, len(b))
	off := 4 + 10 // rev + reserved
	for i := 0; i < 10 && off+4 <= len(b); i++ {
		n := int(binary.LittleEndian.Uint32(b[off:]))
		if n < 0 || n > 4096 || off+4+n*2 > len(b) {
			t.Logf("[%2d] @%3d: non-string u32=%d", i, off, n)
			off += 4
			continue
		}
		units := make([]uint16, n)
		for k := 0; k < n; k++ {
			units[k] = binary.LittleEndian.Uint16(b[off+4+k*2:])
		}
		s := string(utf16.Decode(units))
		if len(s) > 60 {
			s = s[:60] + "..."
		}
		off += 4 + n*2
		t.Logf("[%2d] @%3d len=%4d %q", i, off-4-n*2, n, s)
	}
	// dump remaining tail
	if off < len(b) {
		t.Logf("tail @%d: %x", off, b[off:minT2(off+64, len(b))])
	}
}

func minT2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
