package rvt

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXGArcOffset(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/13")
	part, _ := ReadPartitionWithLookup("13", raw, nil)
	var s103 *SubStream
	for _, ss := range part.SubStreams {
		if ss.Class == 103 {
			s103 = ss
		}
	}
	d := s103.Data
	ga, _ := classindex.Index("2023", "OdBmGArc")

	// scan [i16 GArc] at various prefix offsets; gate: XDir+YDir unit-ish
	// or scaled-but-orthogonal, Radius > 0
	for _, prefix := range []int{0, 8, 16, 24, 32} {
		base := 2 + prefix // after class i16
		if base+88 > len(d) {
			continue
		}
		hits := 0
		for off := 0; off+base+88 <= len(d); off += 2 {
			if int16(binary.LittleEndian.Uint16(d[off:])) != int16(ga) {
				continue
			}
			p := d[off+base:]
			xm := math.Sqrt(math.Float64frombits(binary.LittleEndian.Uint64(p[0:]))*math.Float64frombits(binary.LittleEndian.Uint64(p[0:])) +
				math.Float64frombits(binary.LittleEndian.Uint64(p[8:]))*math.Float64frombits(binary.LittleEndian.Uint64(p[8:])) +
				math.Float64frombits(binary.LittleEndian.Uint64(p[16:]))*math.Float64frombits(binary.LittleEndian.Uint64(p[16:])))
			ym := math.Sqrt(math.Float64frombits(binary.LittleEndian.Uint64(p[24:]))*math.Float64frombits(binary.LittleEndian.Uint64(p[24:])) +
				math.Float64frombits(binary.LittleEndian.Uint64(p[32:]))*math.Float64frombits(binary.LittleEndian.Uint64(p[32:])) +
				math.Float64frombits(binary.LittleEndian.Uint64(p[40:]))*math.Float64frombits(binary.LittleEndian.Uint64(p[40:])))
			if math.IsNaN(xm) || math.IsNaN(ym) {
				continue
			}
			r := math.Float64frombits(binary.LittleEndian.Uint64(p[48:]))
			if r > 1e-6 && r < 1e4 && xm > 0.9 && ym > 0.9 {
				hits++
			}
		}
		t.Logf("prefix=%d: unit-arc hits=%d", prefix, hits)
	}
	// also: what does a known GArc look like at each offset?
	// print first 3 [2040] markers with 96B context
	shown := 0
	for off := 0; off+6+96 <= len(d) && shown < 3; off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(ga) ||
			binary.LittleEndian.Uint32(d[off+2:]) != 0 {
			continue
		}
		t.Logf("GArc[0] @%d payload 96B: %x", off, d[off+6:off+6+96])
		shown++
	}
}
