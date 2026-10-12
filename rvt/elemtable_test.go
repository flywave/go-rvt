package rvt

import (
	"encoding/binary"
	"testing"
)

func elemTableRec(id, depA, depB uint32) []byte {
	var b [28]byte
	binary.LittleEndian.PutUint32(b[0:], id)
	binary.LittleEndian.PutUint32(b[4:], id)
	binary.LittleEndian.PutUint32(b[12:], depA)
	binary.LittleEndian.PutUint32(b[16:], depB)
	binary.LittleEndian.PutUint32(b[24:], 0xffffffff)
	return b[:]
}

func TestParseElemTableSynthetic(t *testing.T) {
	var data []byte
	var magic [6]byte
	binary.LittleEndian.PutUint32(magic[:], 0x2054055a)
	data = append(data, magic[:]...)
	for i := 0; i < 7; i++ {
		data = append(data, elemTableRec(uint32(26+i*3), 767, 767)...)
	}
	footer := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	data = append(data, footer...)

	et, err := ParseElemTable(data)
	if err != nil {
		t.Fatal(err)
	}
	if et.Magic != 0x2054055a || len(et.Entries) != 7 || et.IDErrors != 0 {
		t.Errorf("magic %#x entries %d errs %d", et.Magic, len(et.Entries), et.IDErrors)
	}
	if et.Entries[2].ID != 32 || et.Entries[2].DepA != 767 {
		t.Errorf("entry2 %+v", et.Entries[2])
	}
	info := et.Info()
	if info.RecordCount != 7 || info.FirstID != 26 || info.LastID != 44 || info.FooterBytes != 19 {
		t.Errorf("info %+v", info)
	}
}

func TestParseElemTableBadMagic(t *testing.T) {
	if _, err := ParseElemTable([]byte{1, 2, 3, 4, 5, 6, 7, 8}); err == nil {
		t.Fatal("expected magic error")
	}
}
