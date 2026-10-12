package rvt

import (
	"encoding/binary"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXIdOverlap(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/13")
	part, _ := ReadPartitionWithLookup("13", raw, nil)
	var s101, s102 *SubStream
	for _, ss := range part.SubStreams {
		if ss.Class == 101 {
			s101 = ss
		}
		if ss.Class == 102 {
			s102 = ss
		}
	}
	gl, _ := classindex.Index("2023", "OdBmGLine")
	d := s102.Data

	// id-keyed entries: [i16 GLine][u32 id>=1][6xf64 finite]
	idSet := map[uint32]int{}
	for off := 0; off+6+48 <= len(d); off += 2 {
		if int16(binary.LittleEndian.Uint16(d[off:])) != int16(gl) {
			continue
		}
		id := binary.LittleEndian.Uint32(d[off+2:])
		if id == 0 || id > 1<<21 {
			continue
		}
		plain := true
		for k := 0; k < 6; k++ {
			v := math.Float64frombits(binary.LittleEndian.Uint64(d[off+6+k*8:]))
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e5 {
				plain = false
				break
			}
		}
		if plain {
			idSet[id]++
		}
	}
	recCounters := map[uint32]bool{}
	for i := range s101.Records {
		recCounters[s101.Records[i].Counter] = true
	}
	var ids []uint32
	for id := range idSet {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	inRec := 0
	for _, id := range ids {
		if recCounters[id] {
			inRec++
		}
	}
	t.Logf("id-keyed: %d distinct ids; overlapping class-101 record counters: %d", len(ids), inRec)
	if len(ids) > 0 {
		t.Logf("id range: %d..%d; lowest 12: %v", ids[0], ids[len(ids)-1], ids[:minI11(12, len(ids))])
	}
	// deposit counters overlap
	depCounters := map[uint32]bool{}
	for _, ss := range part.SubStreams {
		for _, dep := range ss.Deposits {
			depCounters[dep.Counter] = true
		}
	}
	inDep := 0
	for _, id := range ids {
		if depCounters[id] {
			inDep++
		}
	}
	t.Logf("overlapping deposit counters: %d", inDep)
}

func minI11(a, b int) int {
	if a < b {
		return a
	}
	return b
}
