package rvt

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

// TestXVerifyR86 checks whether the R86 inline "lines" survive stricter
// gates: component magnitude >= 1e-6 (denormal catalog pairs excluded) and
// the u32-view interpretation test.
func TestXVerifyR86(t *testing.T) {
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

	total, denormOnly, mixed := 0, 0, 0
	for off := 0; off+2+48 <= len(d); off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(gl) {
			continue
		}
		var p [6]float64
		plain := true
		hasNormalMag := false
		allTiny := true
		for k := 0; k < 6; k++ {
			p[k] = math.Float64frombits(binary.LittleEndian.Uint64(d[off+2+k*8:]))
			if math.IsNaN(p[k]) || math.IsInf(p[k], 0) || math.Abs(p[k]) > 1e5 {
				plain = false
				break
			}
			if math.Abs(p[k]) > 1e-6 {
				hasNormalMag = true
				allTiny = false
			}
		}
		if !plain {
			continue
		}
		total++
		if allTiny {
			denormOnly++
		} else if hasNormalMag {
			mixed++
		}
	}
	t.Logf("R86 candidate lines: total=%d denormal-only=%d has-normal-magnitude=%d",
		total, denormOnly, mixed)
	// The 3,920 count: distribution check
	_ = mixed
}

func init() {
	_ = binary.LittleEndian
	_ = math.Abs
}
