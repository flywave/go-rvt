package rvt

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

// TestXPrefixModel: GLine body = [EndParams 2xf64][Origin 3xf64][Dir 3xf64]
// (GNode.GInfo lazy → dynamic; GCurve.EndParams inline). Validate.
func TestXPrefixModel(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/13")
	part, _ := ReadPartitionWithLookup("13", raw, nil)
	var s103 *SubStream
	for _, ss := range part.SubStreams {
		if ss.Class == 103 {
			s103 = ss
		}
	}
	d := s103.Data
	gl, _ := classindex.Index("2023", "OdBmGLine")

	ok, bad := 0, 0
	var sample [8]float64
	have := false
	for off := 0; off+2+64 <= len(d); off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(gl) {
			continue
		}
		var f [8]float64
		plain := true
		for k := 0; k < 8; k++ {
			f[k] = math.Float64frombits(binary.LittleEndian.Uint64(d[off+2+k*8:]))
			if math.IsNaN(f[k]) || math.IsInf(f[k], 0) || math.Abs(f[k]) > 1e5 {
				plain = false
				break
			}
		}
		if !plain {
			bad++
			continue
		}
		// Origin = f[2:5], Dir = f[5:8] (EndParams = f[0:2])
		dx := f[5] - f[2]
		if f[0] == f[1] || math.Abs(dx) < 1e-9 {
			bad++
			continue
		}
		ok++
		if !have {
			sample = f
			have = true
		}
	}
	t.Logf("prefix-model GLine: ok=%d bad=%d", ok, bad)
	if have {
		t.Logf("sample: EP=(%.4f,%.4f) origin=(%.4f,%.4f,%.4f) dir=(%.4f,%.4f,%.4f)",
			sample[0], sample[1], sample[2], sample[3], sample[4], sample[5], sample[6], sample[7])
	}
	// compare: old model (no EP skip) plausibility
	old := 0
	for off := 0; off+2+48 <= len(d); off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(gl) {
			continue
		}
		plain := true
		for k := 0; k < 6; k++ {
			v := math.Float64frombits(binary.LittleEndian.Uint64(d[off+2+k*8:]))
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e5 {
				plain = false
				break
			}
		}
		if plain {
			old++
		}
	}
	t.Logf("old no-EP model plausible count (includes catalog denormals): %d", old)
}
