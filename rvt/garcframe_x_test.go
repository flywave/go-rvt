package rvt

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXGArcFrame(t *testing.T) {
	data, err := os.ReadFile(richSample)
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/3")
	part, _ := ReadPartitionWithLookup("3", raw, nil)
	var s103 *SubStream
	for _, ss := range part.SubStreams {
		if ss.Class == 103 {
			s103 = ss
		}
	}
	d := s103.Data
	ga, _ := classindex.Index("2017", "OdBmGArc")

	f64at := func(base int) float64 {
		return math.Float64frombits(binary.LittleEndian.Uint64(d[base:]))
	}
	fin := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

	ok, badEP, badAxes, badR := 0, 0, 0, 0
	var samples [][11]float64
	for off := 0; off+2+99 <= len(d); off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(ga) {
			continue
		}
		base := off + 2
		// EndParams 2×f64
		ep0, ep1 := f64at(base), f64at(base+8)
		if !fin(ep0) || !fin(ep1) || math.Abs(ep0) > 1e4 || math.Abs(ep1) > 1e4 {
			badEP++
			continue
		}
		// XDir 3×f64 unit
		x1, x2, x3 := f64at(base+16), f64at(base+24), f64at(base+32)
		if !fin(x1) || !fin(x2) || !fin(x3) {
			badAxes++
			continue
		}
		if math.Abs(x1) < 1e-9 && math.Abs(x2) < 1e-9 && math.Abs(x3) < 1e-9 {
			badAxes++
			continue
		}
		// YDir 3×f64, orthogonal to XDir (axes may be scaled — ellipse-style)
		y1, y2, y3 := f64at(base+40), f64at(base+48), f64at(base+56)
		if !fin(y1) || !fin(y2) || !fin(y3) {
			badAxes++
			continue
		}
		if math.Abs(x1*y1+x2*y2+x3*y3) > 0.05*1e4 {
			badAxes++
			continue
		}
		// Radius f64 > 0
		r := f64at(base + 64)
		if !fin(r) || r <= 1e-6 || r > 1e4 {
			badR++
			continue
		}
		// Center 3×f64 finite
		c1, c2, c3 := f64at(base+72), f64at(base+80), f64at(base+88)
		if !fin(c1) || !fin(c2) || !fin(c3) || math.Abs(c1) > 1e5 || math.Abs(c2) > 1e5 || math.Abs(c3) > 1e5 {
			badR++
			continue
		}
		ok++
		if len(samples) < 3 {
			samples = append(samples, [11]float64{ep0, ep1, x1, x2, x3, y1, y2, y3, r, c1, c2})
			_ = samples[len(samples)-1]
			t.Logf("GArc @%d: EP=(%.4f,%.4f) X=(%.4f,%.4f,%.4f) Y=(%.4f,%.4f,%.4f) R=%.4f C=(%.4f,%.4f)",
				off, ep0, ep1, x1, x2, x3, y1, y2, y3, r, c1, c2)
		}
	}
	t.Logf("GArc full-frame: ok=%d badEP=%d badAxes=%d badR=%d", ok, badEP, badAxes, badR)
}
