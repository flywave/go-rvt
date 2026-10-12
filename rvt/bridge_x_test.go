package rvt

import (
	"encoding/binary"
	"os"
	"sort"
	"testing"

	"github.com/flywave/go-rvt/rvt/classindex"
)

func TestXBridge(t *testing.T) {
	data, err := os.ReadFile(samples[0])
	if err != nil {
		t.Skip("no sample")
	}
	f, _ := OpenCFB(data)
	raw, _ := f.Stream("Partitions/13")
	lookup := func(cls int16) string {
		n, _ := classindex.Lookup("2023", cls)
		return n
	}
	part, _ := ReadPartitionWithLookup("13", raw, lookup)
	var s101, s102, s103 *SubStream
	for _, ss := range part.SubStreams {
		if ss.Class == 101 {
			s101 = ss
		}
		if ss.Class == 102 {
			s102 = ss
		}
		if ss.Class == 103 {
			s103 = ss
		}
	}
	gl, _ := classindex.Index("2023", "OdBmGLine")

	// 1. trees: element counter -> GLine ref ids
	type tree struct {
		ctr    uint32
		glines []uint32
	}
	var trees []tree
	gi := ParseGeometryIndex(s103.Data)
	ge, _ := classindex.Index("2023", "OdBmGElement")
	// rebuild trees with refs by walking groups
	pos := gi.DataOffset
	for pos+18 <= len(s103.Data) {
		ctr := binary.LittleEndian.Uint32(s103.Data[pos:])
		if int16(binary.LittleEndian.Uint16(s103.Data[pos+12:])) != int16(ge) ||
			binary.LittleEndian.Uint32(s103.Data[pos+14:]) != ctr {
			pos++
			continue
		}
		next := len(s103.Data)
		for k := pos + 18; k+18 <= len(s103.Data); k++ {
			c2 := binary.LittleEndian.Uint32(s103.Data[k:])
			if c2 != 0 && int16(binary.LittleEndian.Uint16(s103.Data[k+12:])) == int16(ge) &&
				binary.LittleEndian.Uint32(s103.Data[k+14:]) == c2 {
				next = k
				break
			}
		}
		tr := tree{ctr: ctr}
		for k := pos + 18; k+6 <= next; k += 2 {
			if int16(binary.LittleEndian.Uint16(s103.Data[k:])) == int16(gl) {
				id := binary.LittleEndian.Uint32(s103.Data[k+2:])
				if id >= 1 && id <= 1<<20 {
					tr.glines = append(tr.glines, id)
				}
			}
		}
		if len(tr.glines) > 0 {
			trees = append(trees, tr)
		}
		pos = next
	}
	t.Logf("trees with GLine refs: %d", len(trees))

	// 2. class-102: element-counter key positions (from class-101 records)
	keys := map[uint32]bool{}
	for i := range s101.Records {
		keys[s101.Records[i].Counter] = true
	}
	var keyOffs []struct {
		off int
		id  uint32
	}
	for off := 0; off+4 <= len(s102.Data); off += 4 {
		c := binary.LittleEndian.Uint32(s102.Data[off:])
		if keys[c] {
			keyOffs = append(keyOffs, struct {
				off int
				id  uint32
			}{off, c})
		}
	}
	sort.Slice(keyOffs, func(i, j int) bool { return keyOffs[i].off < keyOffs[j].off })
	t.Logf("class-102 key positions: %d", len(keyOffs))

	// 3. for the payload at @71242 (after [GLine][15]): nearest preceding key
	//    and which tree refs GLine id 15?
	nearest := keyOffs[len(keyOffs)-1]
	for _, k := range keyOffs {
		if k.off > 71242 {
			continue
		}
		nearest = k
	}
	t.Logf("payload @71242 (GLine id 15): nearest preceding class-102 key = element %d @%d", nearest.id, nearest.off)
	for _, tr := range trees {
		for _, g := range tr.glines {
			if g == 15 {
				t.Logf("tree counter=%d refs GLine id 15 (members %v)", tr.ctr, tr.glines)
			}
		}
	}
	// 4. also check element 1049's tree (refs GLine 10-13): where are the
	//    payloads? find [GLine][10..13] catalog ids in class-102
	for _, idv := range []uint32{10, 11, 12, 13} {
		n := 0
		var first int
		for off := 0; off+6 <= len(s102.Data); off += 2 {
			if int16(binary.LittleEndian.Uint16(s102.Data[off:])) == int16(gl) &&
				binary.LittleEndian.Uint32(s102.Data[off+2:]) == idv {
				n++
				if n == 1 {
					first = off
				}
			}
		}
		t.Logf("GLine catalog id %d: %d occurrences in class-102 (first @%d)", idv, n, first)
	}
	// which element owns the tree refs 10-13?
	for _, tr := range trees {
		has1012 := false
		for _, g := range tr.glines {
			if g >= 10 && g <= 13 {
				has1012 = true
			}
		}
		if has1012 {
			t.Logf("tree ctr=%d refs GLine 10-13; class-102 keys for ctr: %d",
				tr.ctr, func() int {
					n := 0
					for _, k := range keyOffs {
						if k.id == tr.ctr {
							n++
						}
					}
					return n
				}())
		}
	}
}
