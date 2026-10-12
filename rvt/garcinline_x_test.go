package rvt

import (
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
	"github.com/flywave/go-rvt/rvt/geom"
)

func TestXInlineGArc(t *testing.T) {
	for _, tc := range []struct {
		path string
		year string
		part string
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
		gArc, _ := classindex.Index(tc.year, "OdBmGArc")
		var geo *SubStream
		for _, ss := range part.SubStreams {
			if ss.Class == 103 {
				geo = ss
			}
		}
		if geo == nil {
			continue
		}
		gi := ParseGeometryIndex(geo.Data)
		gEl, _ := classindex.Index(tc.year, "OdBmGElement")
		trees := ScanGTrees(geo.Data, gi.DataOffset, int16(gEl), lookup)
		arcs := ScanInlineGArcs(geo.Data, int16(gArc), trees, lookup)
		t.Logf("%s: inline GArc objects: %d", tc.year, len(arcs))
		for i, a := range arcs {
			if i >= 3 {
				break
			}
			t.Logf("   R=%.4f C=(%.4f,%.4f,%.4f) X=(%.3f,%.3f,%.3f)",
				a.Radius, a.Center.X, a.Center.Y, a.Center.Z, a.XDir.X, a.XDir.Y, a.XDir.Z)
		}
		_ = geom.Point3{}
	}
}
