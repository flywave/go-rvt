package rvt

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXV7Geo(t *testing.T) {
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
	lookup := func(cls int16) string {
		n, _ := classindex.Lookup("2017", cls)
		return n
	}
	// 1. inline class census: [i16 cls][48B plausible f64×6] per G class
	gLine, _ := classindex.Index("2017", "OdBmGLine")
	gArc, _ := classindex.Index("2017", "OdBmGArc")
	gEl, _ := classindex.Index("2017", "OdBmGElement")
	gRep, _ := classindex.Index("2017", "OdBmGRep")
	gGeo, _ := classindex.Index("2017", "OdBmGeometry")
	gBrep, _ := classindex.Index("2017", "OdBmGBrep")
	census := map[int16]int{}
	for off := 0; off+2+48 <= len(d); off += 2 {
		cls := int16(binary.LittleEndian.Uint16(d[off:]))
		if cls < 16 {
			continue
		}
		plain, nonzero := true, false
		for k := 0; k < 6; k++ {
			v := math.Float64frombits(binary.LittleEndian.Uint64(d[off+2+k*8:]))
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e5 {
				plain = false
				break
			}
			if v != 0 {
				nonzero = true
			}
		}
		if plain && nonzero {
			census[cls]++
		}
	}
	names := []struct {
		i int16
		n string
	}{{gLine, "GLine"}, {gArc, "GArc"}, {gEl, "GElement"}, {gRep, "GRep"}, {gGeo, "Geometry"}, {gBrep, "GBrep"}}
	for _, e := range names {
		t.Logf("v7 class-103 [cls][f64x6] census: %s(%d)=%d", e.n, e.i, census[e.i])
	}

	// 2. twin-bbox anywhere scan (96B byte-equal double-bbox, no class gate)
	twin := 0
	var firstTwin int
	for off := 0; off+2+96 <= len(d); off += 2 {
		a := d[off+2 : off+50]
		bb := d[off+50 : off+98]
		if !bytes.Equal(a, bb) {
			continue
		}
		// plausible bbox: finite, min<=max, non-zero
		plausible := true
		var prev [6]float64
		for k := 0; k < 6; k++ {
			v := math.Float64frombits(binary.LittleEndian.Uint64(a[k*8:]))
			prev[k] = v
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e7 {
				plausible = false
				break
			}
		}
		if !plausible || prev[0] > prev[3] || prev[1] > prev[4] || prev[2] > prev[5] {
			continue
		}
		nz := false
		for _, v := range prev {
			if v != 0 {
				nz = true
			}
		}
		if !nz {
			continue
		}
		twin++
		if twin == 1 {
			firstTwin = off
			cls := int16(binary.LittleEndian.Uint16(d[off:]))
			t.Logf("first twin-bbox @%d: cls=%d(%s) bbox=(%.4f,%.4f,%.4f)..(%.4f,%.4f,%.4f)",
				off, cls, lookup(cls), prev[0], prev[1], prev[2], prev[3], prev[4], prev[5])
			t.Logf("   context: %x", d[off-16:off+8])
		}
	}
	t.Logf("twin-bbox blocks (any class): %d (first @%d)", twin, firstTwin)
}
