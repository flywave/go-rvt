package rvt

import (
	"os"
	"strings"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
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

// TestGTreesRealSamples validates the M7 G-tree group parse (findings R67):
// group counts, counter linkage to class-101 records, and bbox sanity —
// with bboxes attached to model elements.
func TestGTreesRealSamples(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/13")
	lookup := func(cls int16) string {
		name, _ := classindex.Lookup("2023", cls)
		return name
	}
	part, err := ReadPartitionWithLookup("13", raw, lookup)
	if err != nil {
		t.Fatal(err)
	}
	m := part.Model("2023")
	if len(m.Trees) < 9000 {
		t.Errorf("%d G trees (want >= 9000)", len(m.Trees))
	}
	// counter linkage
	recCounters := map[uint32]bool{}
	var hdr *SubStream
	for _, ss := range part.SubStreams {
		if len(ss.Deposits) > 0 {
			hdr = ss
		}
	}
	for i := range hdr.Records {
		recCounters[hdr.Records[i].Counter] = true
	}
	linked := 0
	bboxOK := 0
	for _, tr := range m.Trees {
		if recCounters[tr.Counter] {
			linked++
		}
		for _, bb := range tr.BBoxes {
			if bb.Min.X <= bb.Max.X && bb.Min.Y <= bb.Max.Y && bb.Min.Z <= bb.Max.Z {
				bboxOK++
			}
		}
	}
	if linked*100 < len(m.Trees)*80 {
		t.Errorf("only %d/%d tree counters linked", linked, len(m.Trees))
	}
	withBBox := 0
	for _, e := range m.Elements {
		if e.BBox != nil {
			withBBox++
		}
	}
	t.Logf("G trees %d (linked %d, %.0f%%), bboxes %d (sane %d), elements with bbox %d",
		len(m.Trees), linked, 100*float64(linked)/float64(len(m.Trees)),
		func() int {
			n := 0
			for _, tr := range m.Trees {
				n += len(tr.BBoxes)
			}
			return n
		}(), bboxOK, withBBox)
	if withBBox == 0 {
		t.Error("no elements carry a bbox")
	}
	// sample one bbox for plausibility logging
	for _, e := range m.Elements {
		if e.BBox != nil {
			t.Logf("sample bbox: (%.4f, %.4f, %.4f)..(%.4f, %.4f, %.4f)",
				e.BBox.Min.X, e.BBox.Min.Y, e.BBox.Min.Z, e.BBox.Max.X, e.BBox.Max.Y, e.BBox.Max.Z)
			break
		}
	}
	var _ = geom.BBox{}
}

// TestCurvesRealSamples validates the class-102 GLine extraction (R68).
func TestCurvesRealSamples(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/13")
	lookup := func(cls int16) string {
		name, _ := classindex.Lookup("2023", cls)
		return name
	}
	part, err := ReadPartitionWithLookup("13", raw, lookup)
	if err != nil {
		t.Fatal(err)
	}
	m := part.Model("2023")
	t.Logf("curves extracted: %d", len(m.Curves))
	if len(m.Curves) < 50 {
		t.Errorf("only %d curves", len(m.Curves))
	}
	// sanity: finite endpoints, non-degenerate
	for i, c := range m.Curves {
		dx := c.Direction.X - c.Origin.X
		dy := c.Direction.Y - c.Origin.Y
		dz := c.Direction.Z - c.Origin.Z
		if dx*dx+dy*dy+dz*dz == 0 {
			t.Fatalf("curve %d degenerate", i)
		}
	}
	if len(m.Curves) > 0 {
		c := m.Curves[0]
		t.Logf("sample curve: (%.5f,%.5f,%.5f)->(%.5f,%.5f,%.5f)",
			c.Origin.X, c.Origin.Y, c.Origin.Z, c.Direction.X, c.Direction.Y, c.Direction.Z)
	}
}

// TestArcsRealSamples validates content-anchored arc extraction (R68/R69).
func TestArcsRealSamples(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/13")
	lookup := func(cls int16) string {
		name, _ := classindex.Lookup("2023", cls)
		return name
	}
	part, err := ReadPartitionWithLookup("13", raw, lookup)
	if err != nil {
		t.Fatal(err)
	}
	m := part.Model("2023")
	t.Logf("arcs: %d", len(m.Arcs))
	if len(m.Arcs) == 0 {
		t.Fatal("no arcs extracted")
	}
	for i, a := range m.Arcs {
		if a.Radius <= 0 || a.Radius > 1e4 {
			t.Fatalf("arc %d radius %v", i, a.Radius)
		}
	}
	a := m.Arcs[0]
	t.Logf("sample arc: R=%.4f C=(%.4f,%.4f,%.4f)", a.Radius, a.Center.X, a.Center.Y, a.Center.Z)
}

// TestKeyedBinding validates element attribution of curves/arcs (R77).
func TestKeyedBinding(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/13")
	lookup := func(cls int16) string {
		name, _ := classindex.Lookup("2023", cls)
		return name
	}
	part, err := ReadPartitionWithLookup("13", raw, lookup)
	if err != nil {
		t.Fatal(err)
	}
	m := part.Model("2023")
	t.Logf("bound curves: %d, bound arcs: %d", len(m.BoundCurves), len(m.BoundArcs))
	if len(m.BoundCurves) == 0 {
		t.Fatal("no bound curves")
	}
	// element ids must exist in the element set
	elSet := map[uint32]bool{}
	for _, e := range m.Elements {
		elSet[e.ID] = true
	}
	boundToKnown := 0
	for _, b := range m.BoundCurves {
		if elSet[b.ElementID] {
			boundToKnown++
		}
	}
	t.Logf("curves bound to known elements: %d/%d", boundToKnown, len(m.BoundCurves))
	if boundToKnown == 0 {
		t.Error("no curves bound to known elements")
	}
	// distinct element count
	distinct := map[uint32]bool{}
	for _, b := range m.BoundCurves {
		distinct[b.ElementID] = true
	}
	t.Logf("distinct elements with curves: %d", len(distinct))
	// sample
	b := m.BoundCurves[0]
	t.Logf("sample: element %d line (%.5f,%.5f,%.5f)->(%.5f,%.5f,%.5f)",
		b.ElementID, b.Origin.X, b.Origin.Y, b.Origin.Z, b.Direction.X, b.Direction.Y, b.Direction.Z)
}

// TestBindingPrecision quantifies binding accuracy via bbox containment
// (R78): a curve attributed to element X should have both endpoints inside
// X's world bbox (with tolerance). Compares against the random-assignment
// baseline implied by the element count.
func TestBindingPrecision(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/13")
	lookup := func(cls int16) string {
		name, _ := classindex.Lookup("2023", cls)
		return name
	}
	part, err := ReadPartitionWithLookup("13", raw, lookup)
	if err != nil {
		t.Fatal(err)
	}
	m := part.Model("2023")
	bboxOf := map[uint32]geom.BBox{}
	for _, e := range m.Elements {
		if e.BBox != nil {
			bboxOf[e.ID] = *e.BBox
		}
	}
	const tol = 1.0 // internal feet; generous for stub-bbox under-coverage
	in, out, noBBox := 0, 0, 0
	for _, b := range m.BoundCurves {
		bb, ok := bboxOf[b.ElementID]
		if !ok {
			noBBox++
			continue
		}
		inside := func(p geom.Point3) bool {
			return p.X >= bb.Min.X-tol && p.X <= bb.Max.X+tol &&
				p.Y >= bb.Min.Y-tol && p.Y <= bb.Max.Y+tol &&
				p.Z >= bb.Min.Z-tol && p.Z <= bb.Max.Z+tol
		}
		if inside(b.Origin) && inside(b.Direction) {
			in++
		} else {
			out++
		}
	}
	tested := in + out
	t.Logf("bbox containment: in=%d out=%d (no-bbox %d untested)", in, out, noBBox)
	if tested > 0 {
		t.Logf("containment rate: %.1f%% of %d tested", 100*float64(in)/float64(tested), tested)
	}
	// baseline: random element of ~8k elements would rarely contain the line
	if tested > 20 && float64(in)/float64(tested) < 0.5 {
		t.Errorf("containment %.1f%% too low for correct attribution", 100*float64(in)/float64(tested))
	}

	// arcs: center within bbox expanded by radius
	aIn, aOut := 0, 0
	for _, b := range m.BoundArcs {
		bb, ok := bboxOf[b.ElementID]
		if !ok {
			continue
		}
		if b.Center.X >= bb.Min.X-b.Radius-tol && b.Center.X <= bb.Max.X+b.Radius+tol &&
			b.Center.Y >= bb.Min.Y-b.Radius-tol && b.Center.Y <= bb.Max.Y+b.Radius+tol &&
			b.Center.Z >= bb.Min.Z-b.Radius-tol && b.Center.Z <= bb.Max.Z+b.Radius+tol {
			aIn++
		} else {
			aOut++
		}
	}
	if aIn+aOut > 0 {
		t.Logf("arc center containment: %d/%d (%.1f%%)", aIn, aIn+aOut, 100*float64(aIn)/float64(aIn+aOut))
	}
}
