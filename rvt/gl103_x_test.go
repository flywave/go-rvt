package rvt

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXGLine103(t *testing.T) {
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

	// scan [i16 GLine][f64 x6 plausible] — inline line objects
	hits := 0
	var sample [6]float64
	for off := 0; off+2+48 <= len(d) && hits < 200000; off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(gl) {
			continue
		}
		var p [6]float64
		plain := true
		nonzero := false
		for k := 0; k < 6; k++ {
			p[k] = math.Float64frombits(binary.LittleEndian.Uint64(d[off+2+k*8:]))
			if math.IsNaN(p[k]) || math.IsInf(p[k], 0) || math.Abs(p[k]) > 1e5 {
				plain = false
				break
			}
			if p[k] != 0 {
				nonzero = true
			}
		}
		if !plain || !nonzero {
			continue
		}
		hits++
		if hits == 1 {
			sample = p
		}
	}
	t.Logf("class-103 inline GLine[f64x6] objects: %d", hits)
	if hits > 0 {
		t.Logf("sample: origin(%.4f,%.4f,%.4f) end(%.4f,%.4f,%.4f)",
			sample[0], sample[1], sample[2], sample[3], sample[4], sample[5])
	}
	// baseline: random-position false-positive estimate (rescan with wrong class)
	gb, _ := classindex.Index("2023", "OdBmGBrep")
	fp := 0
	for off := 0; off+2+48 <= len(d) && fp < 200000; off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(gb) {
			continue
		}
		plain, nonzero := true, false
		for k := 0; k < 6; k++ {
			v := math.Float64frombits(binary.LittleEndian.Uint64(d[off+2+k*8:]))
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e5 {
				plain = false
				break
			}
			if v != 0 {
				nonzero = true
			}
		}
		if plain && nonzero {
			fp++
		}
	}
	t.Logf("baseline GBrep[f64x6] false-positive rate: %d", fp)
}
