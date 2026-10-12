package rvt

import (
	"encoding/binary"
	"sort"
)

// Partition sub-stream record framing (findings R42/R44 + dual-sample probe):
//
//	record = [u32 counter][u32 size][body: size bytes][u32 size echo]
//	stride = 12 + size; the echo repeats size as the record's last word.
//
// On the 2023/2022 samples this framing is verified for the class-101
// sub-stream (72,045 records / 100% echo coverage on 2023, every body a
// 1398=OdBmElementHeader object; counters step +1 within contiguous page
// runs, jumping over gaps left by excluded corrupt pages). class-102/103
// carry different framing (counter-stamped head / id directory — R28) and
// are NOT walked.
//
// body = [i16 class index][object envelope fields…]; the class index is
// resolved through rvt/classindex (per-version tables).

// ObjectRecord is one decoded record of a partition sub-stream.
type ObjectRecord struct {
	Offset     int // offset within the sub-stream data
	Counter    uint32
	ClassIndex int16
	ClassName  string // resolved via classindex; "" when unresolved
	Body       []byte // size bytes incl. class index, excl. counter/size/echo
}

// WalkStats summarises a record walk.
type WalkStats struct {
	Records     int    `json:"records"`
	BytesWalked int    `json:"bytes_walked"`
	Resyncs     int    `json:"resyncs"`           // gaps jumped (excluded pages / regions)
	Stopped     string `json:"stopped,omitempty"` // why the walk ended
}

// minRecordSize gates degenerate false chains (probe: spurious [4][4]…
// echo coincidences in regular binary data); real records observed ≥ 91B
// on both samples.
const minRecordSize = 16

// minChain is how many consecutive records must validate before a resync
// offset is accepted.
const minChain = 4

// walkRecords walks the [counter][size][body][echo] record loop from from.
// It resyncs across gaps by scanning for the next offset that starts a
// minChain-record run whose bodies share one class index that matches the
// first record's class (the probe showed uniform-class chaining holds on
// real sub-streams and rejects regular-data false chains).
func walkRecords(data []byte, from int, lookup func(int16) string) ([]ObjectRecord, WalkStats) {
	var recs []ObjectRecord
	st := WalkStats{}
	pos := from
	for pos+12 <= len(data) {
		size, ok := recordAt(data, pos)
		if !ok {
			next, chainCls := findChain(data, pos+1)
			if next < 0 {
				st.Stopped = "no further record chain"
				break
			}
			st.Resyncs++
			pos = next
			_ = chainCls
			continue
		}
		body := data[pos+8 : pos+8+size]
		cls := int16(binary.LittleEndian.Uint16(body))
		name := ""
		if lookup != nil {
			name = lookup(cls)
		}
		recs = append(recs, ObjectRecord{
			Offset:     pos,
			Counter:    binary.LittleEndian.Uint32(data[pos:]),
			ClassIndex: cls,
			ClassName:  name,
			Body:       body,
		})
		pos += 12 + size
	}
	st.Records = len(recs)
	st.BytesWalked = pos - from
	return recs, st
}

// recordAt validates one record at pos and returns its size.
func recordAt(data []byte, pos int) (int, bool) {
	if pos+12 > len(data) {
		return 0, false
	}
	size := int(binary.LittleEndian.Uint32(data[pos+4:]))
	if size < minRecordSize || pos+12+size > len(data) {
		return 0, false
	}
	if binary.LittleEndian.Uint32(data[pos+8+size:]) != uint32(size) {
		return 0, false
	}
	return size, true
}

// findChain scans forward for the first offset starting minChain consecutive
// valid records whose bodies carry one uniform class index.
func findChain(data []byte, from int) (int, int16) {
	for scan := from; scan+12*minChain <= len(data); scan++ {
		pos := scan
		var cls int16
		ok := true
		for k := 0; k < minChain; k++ {
			size, valid := recordAt(data, pos)
			if !valid {
				ok = false
				break
			}
			c := int16(binary.LittleEndian.Uint16(data[pos+8:]))
			if k == 0 {
				cls = c
			} else if c != cls {
				ok = false
				break
			}
			pos += 12 + size
		}
		if ok {
			return scan, cls
		}
	}
	return -1, 0
}

// CensusClasses converts records into a class histogram sorted by count.
func CensusClasses(recs []ObjectRecord) []ClassCount {
	counts := map[string]int{}
	unresolved := 0
	for _, r := range recs {
		if r.ClassName == "" {
			unresolved++
			continue
		}
		counts[r.ClassName]++
	}
	out := make([]ClassCount, 0, len(counts)+1)
	for name, n := range counts {
		out = append(out, ClassCount{Name: name, Count: n})
	}
	if unresolved > 0 {
		out = append(out, ClassCount{Name: "(unresolved)", Count: unresolved})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// ClassCount is one histogram entry.
type ClassCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
