package rvt

// Partition element records (Revit 2023 layout; cross-validated against
// the rvt-rs reference implementation — 1,197 records / 876 placed on
// rac_basic_2023, October 2026).
//
// A 2023 element record sits behind the marker
// [Outline tag][0xFF×4][ElementParents tag] — all three tags from the
// file's own Formats/Latest schema. Relative to the marker:
//
//	-52  u32  ElementId (must be declared in Global/ElemTable)
//	-44  u16  ElementHeader tag (schema)
//	-38  i64  BuiltInCategory (negative, in [-2_100_000, -1_990_000])
//	-80  …    record start: +0x32 container u64, +0x42 placement kind
//	          u32, +0x4a class tag u16 (Formats/Latest ordinal)
//	  0  8B   bbox marker (the marker itself)
//	  +8 48B  bounding box: min x/y/z, max x/y/z, feet, f64 LE
//	+56  u32  reference-list count, then u32 slots (two lists,
//	          framed identically: the slot before the record's own id
//	          is the host wall on door/window records; the last slot of
//	          the second list names the owner — sketch lines carry it)
//
// The marker offset (80 bytes before the box) matches the 2024 layout;
// 2023 writes 32-bit element ids and 32-bit reference slots where 2024
// writes u64.

import (
	"encoding/binary"
	"math"
	"strings"
)

const (
	builtinCategoryMin = -2_100_000
	builtinCategoryMax = -1_990_000

	recCategoryOffset   = 0x12
	recContainerOffset  = 0x32
	recPlacementOffset  = 0x42
	recClassTagOffset   = 0x4a
	recBBoxMarkerOffset = 0x50
	recBBoxOffset       = 0x58
	recMinLen           = recBBoxOffset + 48

	recIDBeforeMarker    = 52
	recCategoryBeforeMrk = 38
	recHeaderTagAfterID  = 8
	recListMaxEntries    = 1024

	// PlacementKind values (recorded, not decoded — rvt-rs RE-21).
	PlacementPlaced = 0xffffef7f
	PlacementSymbol = 0xffff8000

	containerNone = ^uint64(0)
)

// ElementRecord is one decoded partition element record.
type ElementRecord struct {
	Stream        string  `json:"stream,omitempty"`
	Offset        int     `json:"-"`
	ElementID     uint32  `json:"id"`
	Category      int64   `json:"category"`
	Container     uint64  `json:"container,omitempty"`
	PlacementKind uint32  `json:"placement_kind"`
	ClassTag      uint16  `json:"class_tag,omitempty"`
	BBox          [6]float64 `json:"bbox"`
	References    []uint32 `json:"references,omitempty"`
	PrecedingRef  uint32  `json:"preceding_ref,omitempty"` // host on doors/windows
	OwnerRef      uint32  `json:"owner_ref,omitempty"`     // sketch owner
}

// IsPlacedInstance reports whether the record carries the placed-instance
// placement kind. Eight corpus records carry a third, recorded-not-decoded
// kind value; they are neither placed instances nor type symbols.
func (r *ElementRecord) IsPlacedInstance() bool { return r.PlacementKind == PlacementPlaced }

// IsTypeSymbol reports whether the record is a type/symbol envelope.
func (r *ElementRecord) IsTypeSymbol() bool { return r.PlacementKind == PlacementSymbol }

// HasVolume reports whether the bbox has extent on all three axes.
func (r *ElementRecord) HasVolume() bool {
	for k := 0; k < 3; k++ {
		d := r.BBox[k+3] - r.BBox[k]
		if !isFiniteRec(d) || d <= 1e-6 {
			return false
		}
	}
	return true
}

// HasContainer reports the container reference at +0x32 is set.
func (r *ElementRecord) HasContainer() bool {
	return r.Container != containerNone && r.Container <= math.MaxUint32
}

func isFiniteRec(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// schemaRecordMarker derives the element-record marker and the
// ElementHeader tag from the file's own schema. ok is false when the
// schema lacks any of the three classes.
func schemaRecordMarker(s *Schema) ([8]byte, uint16, bool) {
	var marker [8]byte
	outline, parents, header := s.ByName("Outline"), s.ByName("ElementParents"), s.ByName("ElementHeader")
	if outline == nil || parents == nil || header == nil {
		return marker, 0, false
	}
	binary.LittleEndian.PutUint16(marker[0:], outline.Tag)
	for i := 2; i < 6; i++ {
		marker[i] = 0xff
	}
	binary.LittleEndian.PutUint16(marker[6:], parents.Tag)
	return marker, header.Tag, true
}

// ScanElementRecords scans the element substream of the partition for
// Revit-2023-layout element records. declared is the Global/ElemTable id
// set (records whose id is not declared are rejected). version gates the
// scan: only 2023 has a proven record shape here (fail closed).
func ScanElementRecords(p *Partition, stream string, schema *Schema, declared map[uint32]bool, version int) []ElementRecord {
	if version == 2022 {
		return nil // needs the classindex lookup; use ScanElementRecordsVersioned
	}
	if schema == nil || version < 2023 || version > 2026 {
		return nil
	}
	marker, headerTag, ok := schemaRecordMarker(schema)
	if !ok {
		return nil
	}
	wide := version >= 2024
	idBefore, catBefore := 52, 38
	if wide {
		idBefore, catBefore = 80, 62
	}
	var data []byte
	for _, ss := range p.SubStreams {
		if len(ss.Deposits) > 0 {
			data = ss.Data
		}
	}
	if len(data) < recMinLen {
		return nil
	}
	var out []ElementRecord
	for at := 0; at+recMinLen+4 <= len(data); at++ {
		if !bytesEqual8(data[at:at+8], marker[:]) {
			continue
		}
		if r, ok := decodeRecordAt(data, at, marker, headerTag, declared, wide, idBefore, catBefore); ok {
			r.Stream = stream
			out = append(out, r)
		}
	}
	return out
}

func bytesEqual8(a, b []byte) bool {
	for i := 0; i < 8; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// decodeRecordAt decodes the record whose marker is at `at`
// (version-parameterised: 32-bit 2023 frame, 64-bit 2024+ frame).
func decodeRecordAt(data []byte, at int, marker [8]byte, headerTag uint16, declared map[uint32]bool, wide bool, idBefore, catBefore int) (ElementRecord, bool) {
	var r ElementRecord
	idAt := at - idBefore
	if idAt < 0 {
		return r, false
	}
	if wide {
		u := binary.LittleEndian.Uint64(data[idAt:])
		if u > math.MaxUint32 {
			return r, false
		}
		r.ElementID = uint32(u)
	} else {
		r.ElementID = binary.LittleEndian.Uint32(data[idAt:])
	}
	if !declared[r.ElementID] {
		return r, false
	}
	// The 2023 frame carries the ElementHeader tag at id+8; the 2024+
	// prologue has its flags word there instead.
	if !wide {
		if int16(binary.LittleEndian.Uint16(data[idAt+recHeaderTagAfterID:])) != int16(headerTag) {
			return r, false
		}
	}
	catAt := at - catBefore
	if catAt < 0 {
		return r, false
	}
	r.Category = int64(binary.LittleEndian.Uint64(data[catAt:]))
	if r.Category < builtinCategoryMin || r.Category > builtinCategoryMax {
		return r, false
	}
	base := at - recBBoxMarkerOffset
	if base < 0 {
		return r, false
	}
	r.Container = binary.LittleEndian.Uint64(data[base+recContainerOffset:])
	r.PlacementKind = binary.LittleEndian.Uint32(data[base+recPlacementOffset:])
	r.ClassTag = binary.LittleEndian.Uint16(data[base+recClassTagOffset:])
	for k := 0; k < 6; k++ {
		v := math.Float64frombits(binary.LittleEndian.Uint64(data[at+8+k*8:]))
		if !isFiniteRec(v) {
			return r, false
		}
		r.BBox[k] = v
	}
	for k := 0; k < 3; k++ {
		if r.BBox[k+3] < r.BBox[k] {
			return r, false
		}
	}
	r.Offset = idAt
	listAt := at + 8 + 48
	if wide {
		first, ok := decodeU64List(data, listAt)
		if !ok {
			return r, true
		}
		r.References = first
		r.PrecedingRef = slotBeforeU32(first, r.ElementID)
		secondAt := listAt + 4 + len(first)*8
		if second, ok := decodeU64List(data, secondAt); ok && len(second) > 0 {
			if last := second[len(second)-1]; last != 0 && last <= math.MaxUint32 {
				r.OwnerRef = uint32(last)
			}
		}
		return r, true
	}
	first, ok := decodeU32List(data, listAt)
	if !ok {
		return r, true
	}
	r.References = first
	r.PrecedingRef = slotBeforeU32(first, r.ElementID)
	secondAt := listAt + 4 + len(first)*4
	if second, ok := decodeU32List(data, secondAt); ok && len(second) > 0 {
		last := second[len(second)-1]
		if last != 0 {
			r.OwnerRef = last
		}
	}
	return r, true
}

// decodeU64List reads a u32-counted list of u64 slots (widened to u32
// ids; out-of-range slots read 0).
func decodeU64List(data []byte, at int) ([]uint32, bool) {
	if at < 0 || at+4 > len(data) {
		return nil, false
	}
	n := int(binary.LittleEndian.Uint32(data[at:]))
	if n == 0 || n > recListMaxEntries || at+4+n*8 > len(data) {
		return nil, false
	}
	out := make([]uint32, n)
	for i := 0; i < n; i++ {
		u := binary.LittleEndian.Uint64(data[at+4+i*8:])
		if u > math.MaxUint32 {
			out[i] = 0
			continue
		}
		out[i] = uint32(u)
	}
	return out, true
}

// decodeU32List reads a u32-counted list of u32 slots.
func decodeU32List(data []byte, at int) ([]uint32, bool) {
	if at < 0 || at+4 > len(data) {
		return nil, false
	}
	n := int(binary.LittleEndian.Uint32(data[at:]))
	if n == 0 || n > recListMaxEntries || at+4+n*4 > len(data) {
		return nil, false
	}
	out := make([]uint32, n)
	for i := 0; i < n; i++ {
		out[i] = binary.LittleEndian.Uint32(data[at+4+i*4:])
	}
	return out, true
}

// slotBeforeU32 returns the slot immediately before the last occurrence
// of `id` in entries (0 when absent or not a plausible element id).
func slotBeforeU32(entries []uint32, id uint32) uint32 {
	for i := len(entries) - 1; i > 0; i-- {
		if entries[i] == id {
			return entries[i-1]
		}
	}
	return 0
}

// ScanElementRecords2022 decodes the 2022 element-record frame: the
// record body opens with the OdBmElementHeader static head (decoded by
// DecodeElementHeader — category in the BuiltInCategory space, verified
// wall-class 8/8 = OST_Walls), then the model bbox as six f64 at offset
// 52 (anchored: 4/4 walls with independent G-tree bboxes on
// rst_advanced_2022), then the counted reference list(s) — u32 slots,
// semantics as on 2023. Placement is not a 2022 frame field; the class
// name decides (Symbol/Type classes are the type envelopes).
func ScanElementRecords2022(p *Partition, stream string, lookup func(int16) string) []ElementRecord {
	if lookup == nil {
		return nil
	}
	var data []byte
	for _, ss := range p.SubStreams {
		if len(ss.Deposits) > 0 {
			data = ss.Data
		}
	}
	if data == nil {
		return nil
	}
	var out []ElementRecord
	for i := range p.SubStreams {
		if len(p.SubStreams[i].Deposits) == 0 {
			continue
		}
		for _, rec := range p.SubStreams[i].Records {
			if len(rec.Body) < 104 {
				continue
			}
			h, err := DecodeElementHeader(rec.Body, lookup)
			if err != nil {
				continue
			}
			if h.CategoryID < builtinCategoryMin || h.CategoryID > builtinCategoryMax {
				continue
			}
			// bbox at 52: six finite f64, min <= max
			var bb [6]float64
			ok := true
			for k := 0; k < 6; k++ {
				v := math.Float64frombits(binary.LittleEndian.Uint64(rec.Body[52+k*8:]))
				if !isFiniteRec(v) {
					ok = false
					break
				}
				bb[k] = v
			}
			if !ok {
				continue
			}
			for k := 0; k < 3; k++ {
				if bb[k+3] < bb[k] {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			r := ElementRecord{
				Stream:    stream,
				Offset:    rec.Offset,
				ElementID: rec.Counter,
				Category:  int64(h.CategoryID),
				BBox:      bb,
			}
			r.References, _ = decodeU32List(rec.Body, 100)
			if len(r.References) > 0 {
				r.PrecedingRef = slotBeforeU32(r.References, r.ElementID)
				secondAt := 100 + 4 + len(r.References)*4
				if second, ok2 := decodeU32List(rec.Body, secondAt); ok2 && len(second) > 0 {
					if last := second[len(second)-1]; last != 0 {
						r.OwnerRef = last
					}
				}
			}
			// placement from the class: Symbol/Type classes are the type
			// envelopes (2022 frames carry no placement word)
			if strings.HasSuffix(h.ClassName, "Symbol") || strings.HasSuffix(h.ClassName, "Type") {
				r.PlacementKind = PlacementSymbol
			} else {
				r.PlacementKind = PlacementPlaced
			}
			out = append(out, r)
		}
	}
	return out
}


// ScanElementRecordsVersioned dispatches by release: 2023-2026 use the
// schema-derived marker frame, 2022 uses the ElementHeader frame (the
// classindex lookup resolves record class names).
func ScanElementRecordsVersioned(p *Partition, stream string, schema *Schema, declared map[uint32]bool, version int, lookup func(int16) string) []ElementRecord {
	if version == 2022 {
		return ScanElementRecords2022(p, stream, lookup)
	}
	return ScanElementRecords(p, stream, schema, declared, version)
}
