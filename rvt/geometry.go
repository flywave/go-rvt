package rvt

import (
	"encoding/binary"
	"sort"
)

// Class-103 sub-stream layout (REA round 6, dual-sample byte-validated —
// findings R53):
//
// The class-103 sub-stream is the geometry deposit stream. It opens with a
// run of 18-byte fixed-shape directory records keyed by deposit counters
// from the SAME space as the class-101 deposit/record counters:
//
//	[u32 counter][i16 class][u16][u32 k][u16][u32 k]   (18B, two k marks)
//
// The record class varies per version and entry — 2023: OdBmGeomStep(97)
// ×15 (counters 1098306–1098316 = the class-101 deposit counters, then
// 312/313/314/315 = record counters); 2022: OdBmAnalyticalLinkGStep(251)
// ×2 (counters 99585/1708 = the 2022 deposit counters). Detection is
// therefore shape-based (the twin u32 marks), not class-based.
//
// After the GeomStep run, geometry-bearing deposits follow with G-node
// object sequences (slow-path envelope [i16 class][fields…]; markers
// confirmed in-stream on 2023: GLine×17803, GElement×5868, Geometry×1652,
// GPoint×149, GBrep×33, GEdge×67, GEdgeLoop×46, GRep×41, GeomTable×30).
//
// G-node reader schemas (REA, generated readers in TB_Format2023Readers):
//
//	GNode(1303)     ← GInfo (object ref)
//	GRep(2074)      ← GNode; GeomBBox(obj), GTightBBox(obj), ObjectType(i32),
//	                  GRepFlags(u32), ElementId(i64)
//	GElement(2073)  ← GRep (base read inlined)
//	GPoint(2098)    ← GNode; Coord(obj), Size(i32), BorderSize(i32),
//	                  PointFlags(i32)
//	GLine(1798)     ← GCurve(1797); OriginPoint(obj), DirectionVector(obj)
//	GBrep(2044)     ← GNode; Faces(collection)
//	GEdge(1301)     ← GNode; Faces/Next/Prev (idrefs), InteriorEdgePnts
//	                  (array<OdBmEdgePnt>), FirstAndLastEdgePnts
//	                  (array<OdBmEdgePnt,2>), Flags(i32)
//	GEdgeLoop(1312) ← GNode; CondInt16, NextLoop/Face/Next/Prev (idrefs),
//	                  Envelope(obj), Open(bool)

// geomStepRecordSize is the fixed directory record size on the samples.
const geomStepRecordSize = 18

// GeomDirEntry is one OdBmGeomStep directory record of the class-103
// sub-stream.
type GeomDirEntry struct {
	Counter uint32 `json:"counter"` // deposit counter (shared with class-101)
	Class   int16  `json:"class"`   // 97 = OdBmGeomStep
	Field6  uint16 `json:"field6"`  // 180 on 2023 [?]
	Field8  uint32 `json:"field8"`  // 2 on 2023 [?]
	Field12 uint16 `json:"field12"` // 3282 on 2023 [?]
	Field14 uint32 `json:"field14"` // 2 on 2023 [?]
}

// GeometryIndex is the parsed head of the class-103 (geometry) sub-stream.
type GeometryIndex struct {
	Entries    []GeomDirEntry `json:"entries"`
	Counters   []uint32       `json:"-"`           // directory counters, ascending as stored
	DataOffset int            `json:"data_offset"` // end of the GeomStep run
}

// ParseGeometryIndex walks the leading directory of a class-103 sub-stream
// (18-byte fixed-shape records; the run ends at the first shape mismatch —
// geometry object data follows).
func ParseGeometryIndex(data []byte) *GeometryIndex {
	gi := &GeometryIndex{}
	pos := 0
	for pos+geomStepRecordSize <= len(data) {
		// shape gate: twin u32 marks at +8/+14, small and equal
		m8 := binary.LittleEndian.Uint32(data[pos+8:])
		m14 := binary.LittleEndian.Uint32(data[pos+14:])
		if m8 != m14 || m8 == 0 || m8 >= 1024 {
			break
		}
		e := GeomDirEntry{
			Counter: binary.LittleEndian.Uint32(data[pos:]),
			Class:   int16(binary.LittleEndian.Uint16(data[pos+4:])),
			Field6:  binary.LittleEndian.Uint16(data[pos+6:]),
			Field8:  binary.LittleEndian.Uint32(data[pos+8:]),
			Field12: binary.LittleEndian.Uint16(data[pos+12:]),
			Field14: binary.LittleEndian.Uint32(data[pos+14:]),
		}
		gi.Entries = append(gi.Entries, e)
		gi.Counters = append(gi.Counters, e.Counter)
		pos += geomStepRecordSize
	}
	gi.DataOffset = pos
	return gi
}

// HasCounter reports whether the directory carries the given deposit
// counter (i.e. that deposit has a GeomStep entry).
func (gi *GeometryIndex) HasCounter(c uint32) bool {
	for _, e := range gi.Entries {
		if e.Counter == c {
			return true
		}
	}
	return false
}

// GMarkerCensus scans a byte region for aligned i16 G-class envelope
// markers and returns counts sorted by class name.
func GMarkerCensus(data []byte, lookup func(int16) string) []ClassCount {
	targets := map[int16]bool{2073: true, 2074: true, 1798: true, 2044: true, 1301: true, 1312: true, 2098: true, 1303: true, 2160: true, 2155: true}
	counts := map[string]int{}
	for off := 0; off+2 <= len(data); off += 2 {
		v := int16(binary.LittleEndian.Uint16(data[off:]))
		if targets[v] {
			name := lookup(v)
			if name != "" {
				counts[name]++
			}
		}
	}
	out := make([]ClassCount, 0, len(counts))
	for name, n := range counts {
		out = append(out, ClassCount{Name: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}
