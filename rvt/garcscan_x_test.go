package rvt

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

// TestXGArcScan: for each [2040] marker in class-103, scan forward at
// multiple offsets for (unit XDir)(unit YDir)(positive R) patterns.
func TestXGArcScan(t *testing.T) {
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

	// find [2040] markers
	type marker struct {
		off int
		id  uint32
	}
	var markers []marker
	for off := 0; off+6 <= len(d); off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) == int16(ga) {
			id := binary.LittleEndian.Uint32(d[off+2:])
			markers = append(markers, marker{off, id})
		}
	}
	t.Logf("GArc markers: %d", len(markers))

	// for each marker, look forward up to 128B for XDir/YDir/R pattern
	patterns := []struct {
		epOff int // offset from marker to EndParams start
		desc  string
	}{
		{0, "no-prefix"},
		{16, "GInfo 16B"},
		{32, "GInfo+GCurve 32B"},
		{48, "GInfo+GCurve+XDir 48B"},
	}
	for _, pat := range patterns {
		total := 0
		for _, m := range markers {
			base := m.off + 2 + pat.epOff
			if base+88 > len(d) {
				continue
			}
			// XDir at +0 (unit-ish), YDir at +24 (unit-ish, orthogonal)
			x1 := math.Float64frombits(binary.LittleEndian.Uint64(d[base:]))
			x2 := math.Float64frombits(binary.LittleEndian.Uint64(d[base+8:]))
			x3 := math.Float64frombits(binary.LittleEndian.Uint64(d[base+16:]))
			xm := math.Sqrt(x1*x1 + x2*x2 + x3*x3)
			if math.IsNaN(xm) || xm < 0.9 || xm > 1.1 {
				continue
			}
			y1 := math.Float64frombits(binary.LittleEndian.Uint64(d[base+24:]))
			y2 := math.Float64frombits(binary.LittleEndian.Uint64(d[base+32:]))
			y3 := math.Float64frombits(binary.LittleEndian.Uint64(d[base+40:]))
			ym := math.Sqrt(y1*y1 + y2*y2 + y3*y3)
			if math.IsNaN(ym) || ym < 0.9 || ym > 1.1 {
				continue
			}
			dot := x1*y1 + x2*y2 + x3*y3
			if math.Abs(dot) > 0.05 {
				continue
			}
			// Radius at +48
			r := math.Float64frombits(binary.LittleEndian.Uint64(d[base+48:]))
			if math.IsNaN(r) || r <= 0 || r > 1e4 {
				continue
			}
			total++
		}
		t.Logf("offset %s: %d unit-orthogonal arcs", pat.desc, total)
	}

	// dump first GArc marker with 128B context
	if len(markers) > 0 {
		m := markers[0]
		lo := m.off - 16
		if lo < 0 {
			lo = 0
		}
		end := m.off + 2 + 128
		if end > len(d) {
			end = len(d)
		}
		t.Logf("first GArc marker @%d id=%d, context [%d:%d]:",
			m.off, m.id, lo, end)
		for k := lo; k+32 <= end; k += 32 {
			t.Logf("  +%3d: %x", k-lo, d[k:k+32])
		}
	}
}
