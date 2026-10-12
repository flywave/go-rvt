package rvt

import (
	"os"
	"strings"
	"testing"

	"github.com/flywave/go-rvt/rvt/geom"
)

// TestModelRealSamples assembles the object model on the real samples:
// elements with class identities, view memberships, BRep topology skeletons.
func TestModelRealSamples(t *testing.T) {
	found := 0
	for _, path := range samples {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		found++
		version := "2023"
		if strings.Contains(path, "2022") {
			version = "2022"
		}
		f, _ := OpenCFB(data)
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
		raw, _ := f.Stream(biggest)
		part, err := ReadPartitionWithLookup(biggest[11:], raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		m := part.Model(version)
		if len(m.Elements) < 50000 {
			t.Errorf("%s: elements %d", path, len(m.Elements))
		}
		named := 0
		for _, e := range m.Elements {
			if e.ClassName != "" {
				named++
			}
		}
		if named*100 < len(m.Elements)*99 {
			t.Errorf("%s: only %d/%d elements named", path, named, len(m.Elements))
		}
		t.Logf("%s: model elements=%d (%.1f%% named) views=%d breps=%d",
			path, len(m.Elements), 100*float64(named)/float64(len(m.Elements)), len(m.Views), len(m.BReps))
		for i, v := range m.Views {
			if i >= 3 {
				break
			}
			t.Logf("   view %d: %d members", v.ID, len(v.Members))
		}
		for i, br := range m.BReps {
			if i >= 3 {
				break
			}
			t.Logf("   brep: %d faces / %d edges", len(br.FaceIDs), len(br.EdgeIDs))
		}
		if len(m.BReps) == 0 {
			t.Errorf("%s: no BRep skeletons", path)
		}
		// spot-check a BRep: faces and edges both present
		withBoth := 0
		for _, br := range m.BReps {
			if len(br.FaceIDs) > 0 && len(br.EdgeIDs) > 0 {
				withBoth++
			}
		}
		if withBoth == 0 {
			t.Errorf("%s: no BRep with both faces and edges", path)
		}
	}
	if found == 0 {
		t.Skip("no .rvt samples")
	}
}

var _ = geom.Point3{}
