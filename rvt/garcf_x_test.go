package rvt

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXGArcFrame(t *testing.T) {
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
	ga, _ := classindex.Index("2023", "OdBmGArc")

	// collect [2040] markers with 128B payloads, group by content hash
	type cand struct {
		off int
		b   [128]byte
	}
	var cands []cand
	seen := map[[128]byte]bool{}
	for off := 0; off+6+128 <= len(d); off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(ga) {
			continue
		}
		var q cand
		q.off = off
		copy(q.b[:], d[off:off+128])
		if seen[q.b] {
			continue
		}
		seen[q.b] = true
		cands = append(cands, q)
	}
	t.Logf("distinct GArc-marked 128B blocks: %d", len(cands))

	// find near-identical pairs: differ in <= 24 bytes (coords differ, structure same)
	type pair struct {
		a, b int
		diff int
	}
	var pairs []pair
	for i := 0; i < len(cands) && len(pairs) < 5; i++ {
		for j := i + 1; j < len(cands); j++ {
			diff := 0
			for k := 0; k < 128; k++ {
				if cands[i].b[k] != cands[j].b[k] {
					diff++
					if diff > 24 {
						break
					}
				}
			}
			if diff <= 24 && diff > 0 {
				pairs = append(pairs, pair{cands[i].off, cands[j].off, diff})
				if len(pairs) >= 3 {
					break
				}
			}
		}
	}
	for _, pp := range pairs {
		var a, b cand
		for _, e := range cands {
			if e.off == pp.a {
				a = e
			}
			if e.off == pp.b {
				b = e
			}
		}
		t.Logf("near-identical pair @%d vs @%d (diff %d B):", pp.a, pp.b, pp.diff)
		for k := 0; k < 128; k += 16 {
			m := ""
			for n := 0; n < 16; n++ {
				if a.b[k+n] != b.b[k+n] {
					m += " ^"
				}
			}
			if m != "" {
				t.Logf("   diff at [%3d:%3d]%s", k, k+16, m)
			}
		}
		t.Logf("   A: %x", a.b[:64])
		t.Logf("   B: %x", b.b[:64])
	}
}
