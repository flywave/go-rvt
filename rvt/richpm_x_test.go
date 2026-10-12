package rvt

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXRichPM(t *testing.T) {
	data, err := os.ReadFile(richSample)
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/3")
	part, _ := ReadPartitionWithLookup("3", raw, nil)
	var s102 *SubStream
	for _, ss := range part.SubStreams {
		if ss.Class == 102 {
			s102 = ss
		}
	}
	d := s102.Data
	pm, _ := classindex.Index("2017", "OdBmGPolyMesh")
	shown := 0
	for off := 0; off+6+512 <= len(d) && shown < 2; off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(pm) ||
			binary.LittleEndian.Uint32(d[off+2:]) != 0 {
			continue
		}
		payload := d[off+6 : off+6+512]
		t.Logf("PolyMesh[0] @%d payload[0:128]:", off)
		for k := 0; k < 128; k += 32 {
			t.Logf("   +%3d: %x", k, payload[k:k+32])
		}
		// f64 coordinate scan: count plausible f64s in [0:512)
		f64ok, f64n := 0, 0
		var sample [3]float64
		haveSample := false
		for k := 0; k+8 <= 512; k += 8 {
			v := math.Float64frombits(binary.LittleEndian.Uint64(payload[k:]))
			f64n++
			if !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) < 1e4 && v != 0 {
				f64ok++
				if !haveSample && math.Abs(v) > 1e-3 {
					sample[0], sample[1], sample[2] = v, v, v
					haveSample = true
				}
			}
		}
		t.Logf("   f64 plausible/non-zero: %d/%d", f64ok, f64n)
		shown++
	}
}
