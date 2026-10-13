package rvt

import (
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

// TestXNewSamples: test the parsing pipeline on additional RVT samples.
func TestXNewSamples(t *testing.T) {
	for _, path := range []string{
		"/tmp/Revit_Macro_Samples_2017.rvt",
		"/tmp/MyRevitProject.rvt",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Logf("%s: not found", path)
			continue
		}
		f, err := OpenCFB(data)
		if err != nil {
			t.Errorf("%s: not CFB: %v", path, err)
			continue
		}
		t.Logf("%s: CFB OK, %d streams", path, len(f.StreamPaths()))

		md, err := Open(data)
		if err != nil {
			t.Errorf("%s: Open failed: %v", path, err)
			continue
		}
		t.Logf("  version=%q revit=%q streams=%d", md.Format, md.RevitVersion, len(md.Streams))

		if md.RevitVersion != "" {
			t.Logf("  RevitVersion: %q", md.RevitVersion)
		}

		// Find biggest partition and scan
		var biggest string
		var sz uint64
		for _, sp := range f.StreamPaths() {
			if len(sp) > 11 && sp[:11] == "Partitions/" {
				s, _ := f.Size(sp)
				if s > sz {
					sz, biggest = s, sp
				}
			}
		}
		if biggest == "" {
			t.Logf("  no partitions")
			continue
		}
		raw, _ := f.Stream(biggest)
		year := "2023"
		if md.RevitVersion != "" {
			year = md.RevitVersion
		}
		lookup := func(cls int16) string {
			n, _ := classindex.Lookup(year, cls)
			return n
		}
		part, err := ReadPartitionWithLookup(biggest[11:], raw, lookup)
		if err != nil {
			t.Errorf("%s: partition read failed: %v", path, err)
			continue
		}
		m := part.Model(year)
		t.Logf("  partition v%d: elements=%d views=%d breps=%d trees=%d curves=%d arcs=%d boundC=%d boundA=%d",
			part.Info.Version, len(m.Elements), len(m.Views), len(m.BReps), len(m.Trees), len(m.Curves), len(m.Arcs), len(m.BoundCurves), len(m.BoundArcs))

		for _, s := range part.SubStreams {
			t.Logf("  sub-stream class=%d: records=%d logical=%d deposits=%d", s.Class, len(s.Records), s.LogicalBytes, len(s.Deposits))
		}
	}
}
