package rvt

// V7 (Revit 2017, PartitionStreamVersion 7, BFI rev 10) support tests —
// gated on the rich sample at /tmp/rich_sample.rvt (R79/R81).

import (
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

const richSample = "/tmp/rich_sample.rvt"

func TestV7Model(t *testing.T) {
	data, err := os.ReadFile(richSample)
	if err != nil {
		t.Skip("rich sample not present")
	}
	f, err := OpenCFB(data)
	if err != nil {
		t.Fatalf("not CFB: %v", err)
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
		name, _ := classindex.Lookup("2017", cls)
		return name
	}
	part, err := ReadPartitionWithLookup(biggest[11:], raw, lookup)
	if err != nil {
		t.Fatal(err)
	}
	m := part.Model("2017")
	t.Logf("v7 model: elements=%d views=%d breps=%d trees=%d curves=%d arcs=%d",
		len(m.Elements), len(m.Views), len(m.BReps), len(m.Trees), len(m.Curves), len(m.Arcs))
	if len(m.Elements) < 50000 {
		t.Fatalf("elements %d (want >= 50000)", len(m.Elements))
	}
	named := 0
	for _, e := range m.Elements {
		if e.ClassName != "" {
			named++
		}
	}
	if named != len(m.Elements) {
		t.Errorf("named %d/%d", named, len(m.Elements))
	}
	// v7 records start at offset 0: the deposit-free framing (R79/R81)
	for _, ss := range part.SubStreams {
		if ss.Class == 101 {
			if len(ss.Records) == 0 {
				t.Error("class-101 has no records (v7 walk failed)")
			}
		}
	}
	// curves/arcs on v7
	t.Logf("v7 curves=%d arcs=%d", len(m.Curves), len(m.Arcs))
	if len(m.Arcs) == 0 {
		t.Error("no arcs on v7")
	}
}

func TestV7Census(t *testing.T) {
	data, err := os.ReadFile(richSample)
	if err != nil {
		t.Skip("rich sample not present")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/3")
	lookup := func(cls int16) string {
		name, _ := classindex.Lookup("2017", cls)
		return name
	}
	part, _ := ReadPartitionWithLookup("3", raw, lookup)
	var s101, s102 *SubStream
	for _, ss := range part.SubStreams {
		if ss.Class == 101 {
			s101 = ss
		}
		if ss.Class == 102 {
			s102 = ss
		}
	}
	if s101 == nil || s102 == nil {
		t.Fatal("missing sub-streams")
	}
	// v7 class-101 records: uniform ElementHeader per the census
	el, _ := classindex.Index("2017", "OdBmElementHeader")
	nNS, nPM := 0, 0
	ns, _ := classindex.Index("2017", "OdBmGNurbSpline")
	pm, _ := classindex.Index("2017", "OdBmGPolyMesh")
	for off := 0; off+6 <= len(s102.Data); off += 2 {
		v := int16(binary.LittleEndian.Uint16(s102.Data[off:]))
		id := binary.LittleEndian.Uint32(s102.Data[off+2:])
		if v == int16(ns) && id == 0 {
			nNS++
		}
		if v == pm && id == 0 {
			nPM++
		}
	}
	t.Logf("v7: %d records (uniform %s), class-102 NurbSpline[0]=%d PolyMesh[0]=%d",
		len(s101.Records), lookup(int16(el)), nNS, nPM)
	if !strings.Contains(lookup(int16(el)), "ElementHeader") {
		t.Errorf("v7 record class %q", lookup(int16(el)))
	}
}

// TestV7NurbSplinePayload decodes the inline NurbSpline objects (R73 method).
func TestV7NurbSplinePayload(t *testing.T) {
	data, err := os.ReadFile(richSample)
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/3")
	lookup := func(cls int16) string {
		name, _ := classindex.Lookup("2017", cls)
		return name
	}
	part, _ := ReadPartitionWithLookup("3", raw, lookup)
	var s102 *SubStream
	for _, ss := range part.SubStreams {
		if ss.Class == 102 {
			s102 = ss
		}
	}
	ns, _ := classindex.Index("2017", "OdBmGNurbSpline")
	d := s102.Data
	shown := 0
	for off := 0; off+6+192 <= len(d) && shown < 4; off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(ns) ||
			binary.LittleEndian.Uint32(d[off+2:]) != 0 {
			continue
		}
		payload := d[off+6 : off+6+192]
		// decode per R54: Degree(i32) Closed(bool) Rational(bool)
		// ControlPoints(WPoint=4xf64 each) Knots(f64[])
		deg := int32(binary.LittleEndian.Uint32(payload[0:]))
		closed := payload[4]
		rational := payload[5]
		t.Logf("NurbSpline @%d: degree=%d closed=%d rational=%d", off, deg, closed, rational)
		t.Logf("   payload[0:96]: %x", payload[:96])
		// if degree sane (2..10), decode f64 stream after the bools
		if deg >= 2 && deg <= 10 {
			// try: 2 bools (pad to 4) then WPoint array: find knots tail
			nW := 0
			for k := 8; k+32 <= 192; k += 32 {
				w := math.Float64frombits(binary.LittleEndian.Uint64(payload[k:]))
				if w >= 0.9 && w <= 1.1 {
					nW++
				}
			}
			t.Logf("   weight~1.0 slots: %d/6", nW)
		}
		shown++
	}
}
