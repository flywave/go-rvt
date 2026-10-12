package rvt

import (
	"encoding/binary"
	"fmt"
)

// Global/ElemTable layout (findings R35, 2023 sample byte-walk):
//
//	header 6B   = u32 magic 0x2054055a + u16 0        [magic semantics ?]
//	records     = 28B each, 7×u32 LE:
//	              [0] id      element id (gaps = deleted elements)
//	              [1] id2     == id (double-write)
//	              [2] 0
//	              [3] depA    dependency/group slot [?] — NOT a class index
//	              [4] depB    mostly == depA                  [?]
//	              [5] 0
//	              [6] 0xffffffff sentinel
//	footer ~19B [semantic ?]; observed trailing u32s include the
//	            partition's last deposit counter (2023: 1098316) —
//	            the ElemTable↔partition linkage key (02-elemtable.md).

// ElemTableEntry is one 28-byte ElemTable record.
type ElemTableEntry struct {
	ID       uint32 `json:"id"`
	ID2      uint32 `json:"id2"`
	Field2   uint32 `json:"field2"`
	DepA     uint32 `json:"dep_a"`
	DepB     uint32 `json:"dep_b"`
	Field5   uint32 `json:"field5"`
	Sentinel uint32 `json:"sentinel"`
}

// ElemTable is the parsed Global/ElemTable stream.
type ElemTable struct {
	Magic    uint32
	Pad      uint16
	Entries  []ElemTableEntry
	Footer   []byte
	IDErrors int // entries with ID2 != ID
}

// ElemTableInfo is the JSON summary.
type ElemTableInfo struct {
	RecordCount int    `json:"record_count"`
	FirstID     uint32 `json:"first_id,omitempty"`
	LastID      uint32 `json:"last_id,omitempty"`
	IDErrors    int    `json:"id_errors"`
	MagicHex    string `json:"magic_hex"`
	FooterBytes int    `json:"footer_bytes"`
	FooterHex   string `json:"footer_hex"`
	Err         string `json:"error,omitempty"`
}

// ParseElemTable parses the decompressed ElemTable payload.
func ParseElemTable(data []byte) (*ElemTable, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("rvt: elemtable: short (%d bytes)", len(data))
	}
	et := &ElemTable{
		Magic: binary.LittleEndian.Uint32(data[0:]),
		Pad:   binary.LittleEndian.Uint16(data[4:]),
	}
	if et.Magic != 0x2054055a {
		return nil, fmt.Errorf("rvt: elemtable: bad magic %#08x", et.Magic)
	}
	body := data[6:]
	n := len(body) / 28
	et.Entries = make([]ElemTableEntry, 0, n)
	for i := 0; i < n; i++ {
		off := i * 28
		rec := body[off : off+28]
		e := ElemTableEntry{
			ID:       binary.LittleEndian.Uint32(rec[0:]),
			ID2:      binary.LittleEndian.Uint32(rec[4:]),
			Field2:   binary.LittleEndian.Uint32(rec[8:]),
			DepA:     binary.LittleEndian.Uint32(rec[12:]),
			DepB:     binary.LittleEndian.Uint32(rec[16:]),
			Field5:   binary.LittleEndian.Uint32(rec[20:]),
			Sentinel: binary.LittleEndian.Uint32(rec[24:]),
		}
		if e.ID2 != e.ID {
			et.IDErrors++
		}
		et.Entries = append(et.Entries, e)
	}
	et.Footer = body[n*28:]
	return et, nil
}

// Info summarises the table for JSON output.
func (et *ElemTable) Info() ElemTableInfo {
	info := ElemTableInfo{
		RecordCount: len(et.Entries),
		IDErrors:    et.IDErrors,
		MagicHex:    fmt.Sprintf("%08x", et.Magic),
		FooterBytes: len(et.Footer),
		FooterHex:   fmt.Sprintf("%x", et.Footer),
	}
	if len(et.Entries) > 0 {
		info.FirstID = et.Entries[0].ID
		info.LastID = et.Entries[len(et.Entries)-1].ID
	}
	return info
}
