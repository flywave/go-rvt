package rvt

import (
	"math"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

// TestXGLineGInfoLayout (R102) validates the GInfo prefix model on both
// sample generations: [i16 GLine][GInfo 16][EndParams 16][Origin 24][Dir 24].
func TestXGLineGInfoLayout(t *testing.T) {
	for _, tc := range []struct {
		path, year, part string
	}{
		{samples[0], "2023", "Partitions/13"},
		{testdataDir() + "/rac_advanced_2017.rvt", "2017", "Partitions/3"},
	} {
		data, err := os.ReadFile(tc.path)
		if err != nil {
			t.Skip("no sample")
		}
		f, _ := OpenCFB(data)
		raw, _ := f.Stream(tc.part)
		lookup := func(cls int16) string {
			n, _ := classindex.Lookup(tc.year, cls)
			return n
		}
		part, err := ReadPartitionWithLookup(tc.part, raw, lookup)
		if err != nil {
			t.Fatal(err)
		}
		var s103 *SubStream
		for _, ss := range part.SubStreams {
			if ss.Class == 103 {
				s103 = ss
			}
		}
		gl, _ := classindex.Index(tc.year, "OdBmGLine")
		lines := ScanInlineGLines(s103.Data, int16(gl))
		t.Logf("%s: GInfo-gated inline GLine = %d", tc.year, len(lines))
		if len(lines) == 0 {
			t.Errorf("%s: no GInfo-verified lines", tc.year)
			continue
		}
		// every line must have finite, bounded coordinates
		for _, l := range lines {
			for _, v := range []float64{l.Origin.X, l.Origin.Y, l.Origin.Z,
				l.Direction.X, l.Direction.Y, l.Direction.Z} {
				if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e5 {
					t.Fatalf("%s: implausible line payload %+v", tc.year, l)
				}
			}
		}
		s := lines[0]
		t.Logf("  sample: (%.4f,%.4f,%.4f)->(%.4f,%.4f,%.4f)",
			s.Origin.X, s.Origin.Y, s.Origin.Z, s.Direction.X, s.Direction.Y, s.Direction.Z)
	}
}
