package rvt

import (
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXNurbScan(t *testing.T) {
	for _, tc := range []struct {
		path, year, part string
	}{
		{samples[0], "2023", "Partitions/13"},
		{richSample, "2017", "Partitions/3"},
	} {
		data, err := os.ReadFile(tc.path)
		if err != nil {
			continue
		}
		f, _ := OpenCFB(data)
		raw, _ := f.Stream(tc.part)
		lookup := func(cls int16) string {
			n, _ := classindex.Lookup(tc.year, cls)
			return n
		}
		part, _ := ReadPartitionWithLookup(tc.part[11:], raw, lookup)
		var s103 *SubStream
		for _, ss := range part.SubStreams {
			if ss.Class == 103 {
				s103 = ss
			}
		}
		gNurb, _ := classindex.Index(tc.year, "OdBmGNurbSpline")
		gi := ParseGeometryIndex(s103.Data)
		gEl, _ := classindex.Index(tc.year, "OdBmGElement")
		trees := ScanGTrees(s103.Data, gi.DataOffset, int16(gEl), lookup)
		splines := ScanNurbSplines(s103.Data, int16(gNurb), trees)
		t.Logf("%s: NurbSpline objects: %d", tc.year, len(splines))
		for i, s := range splines {
			if i >= 3 {
				break
			}
			t.Logf("  spline %d: elem=%d degree=%d closed=%v rational=%v CP=%d knots=%d",
				i, s.ElementID, s.Degree, s.Closed, s.Rational, len(s.ControlPoints), len(s.Knots))
		}
	}
}
