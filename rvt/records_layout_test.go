package rvt

// Synthesized-frame tests for the 2024+ element-record layout (64-bit
// ids and reference slots; id 80 bytes before the marker, category 62
// before). The layout is the reference implementation's measurement
// (RE-21/RE-32); no 2024+ file is in the regression set, so these tests
// pin the decoder against bytes built from the documented frame.

import (
	"encoding/binary"
	"math"
	"testing"
)

func buildSyntheticRecord2024(buf []byte, at int, id uint64, category int64, bbox [6]float64, refs []uint64) {
	binary.LittleEndian.PutUint64(buf[at:], id)                 // +0x00 id
	binary.LittleEndian.PutUint32(buf[at+0x08:], 0x0111)        // flags
	binary.LittleEndian.PutUint32(buf[at+0x0c:], 0x59f)         // constant
	binary.LittleEndian.PutUint16(buf[at+0x10:], 0)             // u16 0
	binary.LittleEndian.PutUint64(buf[at+0x12:], uint64(category))
	for i := 0x1a; i < 0x3a; i++ {
		buf[at+i] = 0xff
	}
	binary.LittleEndian.PutUint64(buf[at+0x2a:], 0xffffffffffffffff)
	binary.LittleEndian.PutUint32(buf[at+0x42:], 0xffffef7f) // placed
	binary.LittleEndian.PutUint32(buf[at+0x46:], 0)
	binary.LittleEndian.PutUint16(buf[at+0x4a:], 100) // class tag
	binary.LittleEndian.PutUint32(buf[at+0x4c:], 0xffffffff)
	copy(buf[at+0x50:], []byte{0x46, 0x01, 0xff, 0xff, 0xff, 0xff, 0xab, 0x05})
	for k := 0; k < 6; k++ {
		binary.LittleEndian.PutUint64(buf[at+0x58+k*8:], math.Float64bits(bbox[k]))
	}
	_ = refs // the reference list is written by the caller at marker+56
}

func TestSyntheticRecord2024(t *testing.T) {
	marker := [8]byte{0x46, 0x01, 0xff, 0xff, 0xff, 0xff, 0xab, 0x05}
	bbox := [6]float64{-10, -5, 0, 10, 5, 9}
	buf := make([]byte, 0x200)
	at := 0x100
	refs := []uint64{7, 99, 12345}
	buildSyntheticRecord2024(buf, at, 12345, -2_000_011, bbox, refs)
	declared := map[uint32]bool{12345: true}
	// reference-list position: the marker sits at at+0x50; the list at
	// marker+8+48 (recMinLen path)
	listAt := at + 0x50 + 8 + 48
	binary.LittleEndian.PutUint32(buf[listAt:], uint32(len(refs)))
	for i, r := range refs {
		binary.LittleEndian.PutUint64(buf[listAt+4+i*8:], r)
	}
	r, ok := decodeRecordAt(buf, at+0x50, marker, 1398, declared, true, 80, 62)
	if !ok {
		t.Fatal("synthetic 2024 record failed to decode")
	}
	if r.ElementID != 12345 || r.Category != -2_000_011 || !r.IsPlacedInstance() {
		t.Errorf("id=%d cat=%d placed=%v", r.ElementID, r.Category, r.IsPlacedInstance())
	}
	if r.PrecedingRef != 99 {
		t.Errorf("preceding ref %d, want 99 (slot before own id)", r.PrecedingRef)
	}
	for k := 0; k < 6; k++ {
		if r.BBox[k] != bbox[k] {
			t.Errorf("bbox[%d]=%v", k, r.BBox[k])
		}
	}
	// undeclared id rejected
	declared2 := map[uint32]bool{}
	if _, ok := decodeRecordAt(buf, at+0x50, marker, 1398, declared2, true, 80, 62); ok {
		t.Error("undeclared id accepted")
	}
	// category out of range rejected
	buildSyntheticRecord2024(buf, at, 12345, 5, bbox, nil)
	if _, ok := decodeRecordAt(buf, at+0x50, marker, 1398, declared, true, 80, 62); ok {
		t.Error("positive category accepted")
	}
}
