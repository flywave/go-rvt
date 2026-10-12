package rvt

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func geomStepRec(counter uint32) []byte {
	b := make([]byte, 18)
	binary.LittleEndian.PutUint32(b[0:], counter)
	binary.LittleEndian.PutUint16(b[4:], 97)
	binary.LittleEndian.PutUint16(b[6:], 180)
	binary.LittleEndian.PutUint32(b[8:], 2)
	binary.LittleEndian.PutUint16(b[12:], 3282)
	binary.LittleEndian.PutUint32(b[14:], 2)
	return b
}

func TestParseGeometryIndexSynthetic(t *testing.T) {
	var data []byte
	for i := 0; i < 5; i++ {
		data = append(data, geomStepRec(uint32(100+i))...)
	}
	data = append(data, []byte{0xDE, 0xAD, 0xBE, 0xEF}...) // object region
	gi := ParseGeometryIndex(data)
	if len(gi.Entries) != 5 || gi.DataOffset != 5*18 {
		t.Fatalf("entries %d offset %d", len(gi.Entries), gi.DataOffset)
	}
	if !gi.HasCounter(102) || gi.HasCounter(200) {
		t.Error("HasCounter broken")
	}
	if gi.Entries[0].Counter != 100 || gi.Entries[0].Class != 97 {
		t.Errorf("entry0 %+v", gi.Entries[0])
	}
}

// TestGeometryIndexRealSamples validates the class-103 geometry sub-stream
// structure and the counter linkage to the class-101 deposit records.
func TestGeometryIndexRealSamples(t *testing.T) {
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
		f, err := OpenCFB(data)
		if err != nil {
			t.Fatal(err)
		}
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
		lookup := func(cls int16) string {
			name, _ := classindex.Lookup(version, cls)
			return name
		}
		part, err := ReadPartitionWithLookup(biggest[11:], raw, lookup)
		if err != nil {
			t.Fatal(err)
		}
		var hdr, geo *SubStream
		for _, ss := range part.SubStreams {
			if ss.Class == 101 && len(ss.Deposits) > 0 {
				hdr = ss
			}
			if ss.Class == 103 {
				geo = ss
			}
		}
		if hdr == nil || geo == nil {
			t.Fatalf("%s: missing sub-streams", path)
		}
		gi := ParseGeometryIndex(geo.Data)
		if len(gi.Entries) == 0 || gi.DataOffset == 0 {
			t.Fatalf("%s: no geomstep directory", path)
		}
		// counter linkage: geom directory counters live in the shared deposit
		// counter space — the class-101 deposit AND record counters (R53)
		recCounters := map[uint32]bool{}
		for _, d := range hdr.Deposits {
			recCounters[d.Counter] = true
		}
		for _, r := range hdr.Records {
			recCounters[r.Counter] = true
		}
		linked := 0
		for _, c := range gi.Counters {
			if recCounters[c] {
				linked++
			}
		}
		if linked*100 < len(gi.Counters)*90 {
			t.Errorf("%s: only %d/%d geom counters linked to class-101 records", path, linked, len(gi.Counters))
		}
		markers := GMarkerCensus(geo.Data[gi.DataOffset:], lookup)
		t.Logf("%s: geomstep dir %d entries (linked %d), data @%d; G markers: %v",
			path, len(gi.Entries), linked, gi.DataOffset, markers)
		// The marker census uses the 2023 index anchors; only assert on the
		// 2023 sample (2022 class indexes are renumbered — R33/R53).
		if version == "2023" && len(markers) < 3 {
			t.Errorf("%s: too few G markers in geometry region", path)
		}
	}
	if found == 0 {
		t.Skip("no .rvt samples")
	}
}

// TestGCatalogRunsReal detects the G-database catalog layer in the class-103
// geometry region (findings R56): [i16 class][u32 id] runs — including the
// Face/Edge sequences of BRep structures.
func TestGCatalogRunsReal(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	b, _ := f.Stream("Partitions/13")
	p, _ := ReadPartition("13", b)
	var geo *SubStream
	for _, ss := range p.SubStreams {
		if ss.Class == 103 {
			geo = ss
		}
	}
	lookup := func(cls int16) string {
		name, _ := classindex.Lookup("2023", cls)
		return name
	}
	gi := ParseGeometryIndex(geo.Data)
	runs := ScanGCatalogRuns(geo.Data, gi.DataOffset, lookup)
	if len(runs) < 10 {
		t.Fatalf("only %d catalog runs", len(runs))
	}
	// BRep topology catalogs use the non-G OdBmFace(1649)/OdBmEdge(1300)
	hasFace, hasEdge := false, false
	for _, r := range runs {
		if r.ClassName == "OdBmFace" {
			hasFace = true
		}
		if r.ClassName == "OdBmEdge" {
			hasEdge = true
		}
	}
	t.Logf("catalog runs: %d (face=%v edge=%v); first 6: %+v", len(runs), hasFace, hasEdge, runs[:6])
	if !hasFace || !hasEdge {
		t.Errorf("expected Face and Edge catalog sequences (BRep topology)")
	}
}
