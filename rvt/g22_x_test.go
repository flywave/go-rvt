package rvt

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestXG22(t *testing.T) {
	data, err := os.ReadFile(samples[1])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	b, _ := f.Stream("Partitions/8")
	p, _ := ReadPartition("8", b)
	for _, ss := range p.SubStreams {
		if ss.Class != 103 {
			continue
		}
		d := ss.Data
		t.Logf("2022 class103 head 64B: %x", d[:64])
		// i16 histogram of first 8 bytes at +4 pitch 18
		for off := 0; off < 3*18; off += 18 {
			t.Logf("  entry@%d: counter=%d i16@4=%d i16@6=%d u32@8=%d i16@12=%d u32@14=%d",
				off, binary.LittleEndian.Uint32(d[off:]),
				int16(binary.LittleEndian.Uint16(d[off+4:])),
				binary.LittleEndian.Uint16(d[off+6:]),
				binary.LittleEndian.Uint32(d[off+8:]),
				binary.LittleEndian.Uint16(d[off+12:]),
				binary.LittleEndian.Uint32(d[off+14:]))
		}
	}
}
