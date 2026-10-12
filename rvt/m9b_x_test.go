package rvt

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXM9B(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	b, _ := f.Stream("Partitions/13")
	p, _ := ReadPartition("13", b)
	var s102 *SubStream
	for _, ss := range p.SubStreams {
		if ss.Class == 102 {
			s102 = ss
		}
	}
	d := s102.Data
	ns, _ := classindex.Index("2023", "OdBmGNurbSpline")
	pm, _ := classindex.Index("2023", "OdBmGPolyMesh")

	// NurbSpline [cls][0] blocks: dump payloads
	shown := 0
	for off := 0; off+6+96 <= len(d) && shown < 3; off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(ns) ||
			binary.LittleEndian.Uint32(d[off+2:]) != 0 {
			continue
		}
		t.Logf("NurbSpline[0] @%d payload 96B: %x", off, d[off+6:off+6+96])
		shown++
		off += 90
	}
	// PolyMesh [cls][0] blocks
	shown = 0
	for off := 0; off+6+64 <= len(d) && shown < 2; off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(pm) ||
			binary.LittleEndian.Uint32(d[off+2:]) != 0 {
			continue
		}
		t.Logf("PolyMesh[0] @%d payload 64B: %x", off, d[off+6:off+6+64])
		shown++
		off += 58
	}
	_ = math.Sqrt
}
