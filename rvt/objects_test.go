package rvt

import (
	"encoding/binary"
	"testing"
)

// objRec builds one [counter][size][body][echo] record.
func objRec(counter uint32, cls int16, payload []byte) []byte {
	body := make([]byte, 2+len(payload))
	binary.LittleEndian.PutUint16(body, uint16(cls))
	copy(body[2:], payload)
	size := len(body)
	out := make([]byte, 0, 12+size)
	var hdr, echo [4]byte
	binary.LittleEndian.PutUint32(hdr[:], counter)
	binary.LittleEndian.PutUint32(echo[:], uint32(size))
	out = append(out, hdr[:]...)
	var sz [4]byte
	binary.LittleEndian.PutUint32(sz[:], uint32(size))
	out = append(out, sz[:]...)
	out = append(out, body...)
	out = append(out, echo[:]...)
	return out
}

func lookup1398(int16) string { return "OdBmElementHeader" }

func TestWalkRecordsBasic(t *testing.T) {
	var data []byte
	data = append(data, objRec(100, 1398, make([]byte, 50))...)
	data = append(data, objRec(101, 1398, make([]byte, 30))...)
	data = append(data, objRec(102, 1398, make([]byte, 200))...)
	recs, st := walkRecords(data, 0, lookup1398)
	if st.Resyncs != 0 || st.Stopped != "" {
		t.Fatalf("stats %+v", st)
	}
	if len(recs) != 3 {
		t.Fatalf("records %d", len(recs))
	}
	if recs[0].Counter != 100 || recs[0].ClassIndex != 1398 || recs[0].ClassName != "OdBmElementHeader" {
		t.Errorf("rec0 %+v", recs[0])
	}
	if len(recs[2].Body) != 202 { // i16 + 200
		t.Errorf("body %d", len(recs[2].Body))
	}
}

func TestWalkRecordsResync(t *testing.T) {
	var data []byte
	for i := 0; i < 5; i++ {
		data = append(data, objRec(uint32(i), 1398, make([]byte, 40))...)
	}
	gap := make([]byte, 64) // excluded-page gap with garbage
	for i := range gap {
		gap[i] = byte(i * 31)
	}
	data = append(data, gap...)
	for i := 5; i < 10; i++ {
		data = append(data, objRec(uint32(i+100), 1398, make([]byte, 40))...)
	}
	recs, st := walkRecords(data, 0, lookup1398)
	if len(recs) != 10 {
		t.Fatalf("records %d (want 10)", len(recs))
	}
	if st.Resyncs != 1 {
		t.Errorf("resyncs %d", st.Resyncs)
	}
}

func TestWalkRecordsRejectsFalseChains(t *testing.T) {
	// Regular binary data full of small size-like words must not produce
	// records (min size gate + uniform-class chain gate).
	data := make([]byte, 4096)
	for i := 0; i < len(data); i += 4 {
		binary.LittleEndian.PutUint32(data[i:], 4)
	}
	recs, _ := walkRecords(data, 0, lookup1398)
	if len(recs) != 0 {
		t.Fatalf("false records %d", len(recs))
	}
}
