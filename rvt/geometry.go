package rvt

import (
	"bytes"
	"encoding/binary"
	"math"
	"sort"

	"github.com/flywave/go-rvt/rvt/classindex"
	"github.com/flywave/go-rvt/rvt/geom"
)

// isBBoxClass: GRep-family classes whose objects carry the twin-bbox head
// (Geometry/GBrep/GRep/GElement per the reader schemas — R54/R67).
func isBBoxClass(cls int16, lookup func(int16) string) bool {
	switch lookup(cls) {
	case "OdBmGeometry", "OdBmGBrep", "OdBmGRep", "OdBmGElement":
		return true
	}
	return false
}

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

// GCatalogRun is one detected run of 6-byte G-database catalog entries
// [i16 class][u32 id] with consecutively increasing ids (findings R56).
// The class-103 geometry region catalogs its G nodes this way — e.g. a
// GBrep's Faces and Edges appear as interleaved Face/Edge id sequences.
type GCatalogRun struct {
	Offset    int    `json:"offset"`
	Class     int16  `json:"class"`
	ClassName string `json:"class_name,omitempty"`
	Entries   int    `json:"entries"`
	FirstID   uint32 `json:"first_id"`
	LastID    uint32 `json:"last_id"`
}

// minGCatalogRun is the minimum run length worth reporting (noise gate).
const minGCatalogRun = 4

// maxGCatalogID filters float-noise false positives (real G-node ids on the
// samples stay below ~1e3; misread f64 pairs produce ~1e9 values).
const maxGCatalogID = 1 << 20

// ScanGCatalogRuns scans data from `from` for catalog runs and returns runs
// of at least minGCatalogRun entries.
func ScanGCatalogRuns(data []byte, from int, lookup func(int16) string) []GCatalogRun {
	var runs []GCatalogRun
	off := from
	for off+6 <= len(data) {
		cls := int16(binary.LittleEndian.Uint16(data[off:]))
		name := lookup(cls)
		if name == "" {
			off += 2
			continue
		}
		n := 0
		pos := off
		prev := int64(binary.LittleEndian.Uint32(data[off+2:])) - 1
		for pos+6 <= len(data) {
			c2 := int16(binary.LittleEndian.Uint16(data[pos:]))
			i2 := int64(binary.LittleEndian.Uint32(data[pos+2:]))
			if lookup(c2) == "" || i2 != prev+1 || i2 < 0 || i2 > maxGCatalogID {
				break
			}
			prev = i2
			pos += 6
			n++
		}
		if n >= minGCatalogRun {
			runs = append(runs, GCatalogRun{
				Offset:    off,
				Class:     cls,
				ClassName: name,
				Entries:   n,
				FirstID:   uint32(prev - int64(n) + 1),
				LastID:    uint32(prev),
			})
			off = pos
			continue
		}
		off += 2
	}
	return runs
}

// G-tree groups (findings R67): the class-103 geometry region is organized
// as per-deposit G-tree blocks:
//
//	[u32 counter][8B key][i16 GElement][u32 counter]…object sequence…
//
// The object sequence mixes inline objects — e.g. Geometry(2160) carrying
// two inline f64 bboxes (GeomBBox + GTightBBox, 48B each) — and typed child
// references ([i16 class][u32 child-id], ids sequential per class). GInfo
// blocks end with GInfoFlags (DefaultModel = 0x00088004).

// GTree is one deposit's G-node group (declared in rvt/geom).
type GTree = geom.GTree

// isPlausibleBBox reports whether 48 bytes look like a sane f64 bbox
// (min <= max per axis, magnitudes within internal-feet range, non-all-zero).
func isPlausibleBBox(b []byte) bool {
	var v [6]float64
	for k := 0; k < 6; k++ {
		v[k] = math.Float64frombits(binary.LittleEndian.Uint64(b[k*8:]))
		if math.IsNaN(v[k]) || math.IsInf(v[k], 0) || math.Abs(v[k]) > 1e7 {
			return false
		}
	}
	for k := 0; k < 3; k++ {
		if v[k] > v[k+3] {
			return false
		}
	}
	for k := 0; k < 6; k++ {
		if v[k] != 0 {
			return true
		}
	}
	return false
}

// ScanGTrees walks the class-103 data region after the GeomStep directory
// and collects GElement tree groups: strict group starts are
// [u32 counter][8B][i16 GElement][u32 counter]; each group body extends to
// the next strict start. Within a body, inline Geometry bboxes and typed
// child references are collected.
func ScanGTrees(data []byte, from int, gElement int16, lookup func(int16) string) []GTree {
	isGroupStart := func(off int) bool {
		if off+18 > len(data) {
			return false
		}
		ctr := binary.LittleEndian.Uint32(data[off:])
		return ctr != 0 &&
			int16(binary.LittleEndian.Uint16(data[off+12:])) == gElement &&
			binary.LittleEndian.Uint32(data[off+14:]) == ctr
	}
	var trees []GTree
	pos := from
	for pos < len(data) {
		if !isGroupStart(pos) {
			pos++
			continue
		}
		g := GTree{Counter: binary.LittleEndian.Uint32(data[pos:]), Offset: pos}
		next := len(data)
		for k := pos + 18; k+18 <= len(data); k++ {
			if isGroupStart(k) {
				next = k
				break
			}
		}
		body := data[pos+18 : next]
		for k := 0; k+2 <= len(body); k += 2 {
			cls := int16(binary.LittleEndian.Uint16(body[k:]))
			// inline GRep-family object: [i16 cls][GeomBBox 48B][GTightBBox 48B]
			// — the two boxes are byte-identical on real objects (R67)
			if k+2+96 <= len(body) &&
				isBBoxClass(cls, lookup) &&
				isPlausibleBBox(body[k+2:k+50]) &&
				bytes.Equal(body[k+2:k+50], body[k+50:k+98]) {
				bb := geom.BBox{
					Min: geom.Point3{
						X: math.Float64frombits(binary.LittleEndian.Uint64(body[k+2:])),
						Y: math.Float64frombits(binary.LittleEndian.Uint64(body[k+10:])),
						Z: math.Float64frombits(binary.LittleEndian.Uint64(body[k+18:])),
					},
					Max: geom.Point3{
						X: math.Float64frombits(binary.LittleEndian.Uint64(body[k+26:])),
						Y: math.Float64frombits(binary.LittleEndian.Uint64(body[k+34:])),
						Z: math.Float64frombits(binary.LittleEndian.Uint64(body[k+42:])),
					},
				}
				g.BBoxes = append(g.BBoxes, bb)
				k += 98
				continue
			}
			if name := lookup(cls); name != "" && cls > 16 && k+6 <= len(body) {
				id := binary.LittleEndian.Uint32(body[k+2:])
				if id <= maxElementID {
					g.Refs = append(g.Refs, geom.GRef{Class: cls, Name: name, ID: id})
					k += 4
				}
			}
		}
		trees = append(trees, g)
		pos = next
	}
	return trees
}

// Inline curve objects in the class-102 object store (findings R68):
// [i16 GLine][u32 0][f64×6] — Origin + second point (schema names the
// second vector DirectionVector; on-disk values read as an endpoint pair).
// Distinguishing marker: the u32 slot is 0 for inline instances while
// typed references carry non-zero ids.

// ScanGLines collects inline GLine objects from a class-102 object store.
// A payload qualifies when all six f64s are finite, within internal-feet
// magnitude, not all zero, and origin != second point.
func ScanGLines(data []byte, gLine int16) []geom.Line {
	var out []geom.Line
	for off := 0; off+6+48 <= len(data); off += 2 {
		if int16(binary.LittleEndian.Uint16(data[off:])) != gLine {
			continue
		}
		if binary.LittleEndian.Uint32(data[off+2:]) != 0 {
			continue
		}
		var f [6]float64
		finite := true
		nonzero := false
		for k := 0; k < 6; k++ {
			f[k] = math.Float64frombits(binary.LittleEndian.Uint64(data[off+6+k*8:]))
			if math.IsNaN(f[k]) || math.IsInf(f[k], 0) || math.Abs(f[k]) > 1e6 {
				finite = false
				break
			}
			if f[k] != 0 {
				nonzero = true
			}
		}
		if !finite || !nonzero {
			continue
		}
		dx, dy, dz := f[3]-f[0], f[4]-f[1], f[5]-f[2]
		if math.Abs(dx)+math.Abs(dy)+math.Abs(dz) < 1e-9 {
			continue
		}
		out = append(out, geom.Line{
			Origin:    geom.Point3{X: f[0], Y: f[1], Z: f[2]},
			Direction: geom.Point3{X: f[3], Y: f[4], Z: f[5]},
		})
	}
	return out
}

// ScanGArcs collects circular arcs from a class-102 object store by
// content anchoring (findings R68/R69): a 10-f64 window of unit XDir,
// unit YDir (dot ~ 0), positive radius, followed by a 3-f64 center.
// These are model-space arcs (radii tens of feet observed).
func ScanGArcs(data []byte) []geom.Arc {
	var out []geom.Arc
	for off := 0; off+80 <= len(data); off += 2 {
		var f [10]float64
		ok := true
		for k := 0; k < 10; k++ {
			f[k] = math.Float64frombits(binary.LittleEndian.Uint64(data[off+k*8:]))
			if math.IsNaN(f[k]) || math.IsInf(f[k], 0) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		xm := math.Sqrt(f[0]*f[0] + f[1]*f[1] + f[2]*f[2])
		ym := math.Sqrt(f[3]*f[3] + f[4]*f[4] + f[5]*f[5])
		dot := f[0]*f[3] + f[1]*f[4] + f[2]*f[5]
		if xm < 0.99 || xm > 1.01 || ym < 0.99 || ym > 1.01 || math.Abs(dot) > 0.01 {
			continue
		}
		if f[6] <= 1e-6 || f[6] > 1e4 {
			continue
		}
		out = append(out, geom.Arc{
			XDir:   geom.Point3{X: f[0], Y: f[1], Z: f[2]},
			YDir:   geom.Point3{X: f[3], Y: f[4], Z: f[5]},
			Radius: f[6],
			Center: geom.Point3{X: f[7], Y: f[8], Z: f[9]},
		})
		off += 78
	}
	return out
}

// KeyedBinding (findings R77/R84): attributes inline geometry objects to
// elements via the nearest-preceding element-counter key occurrence in the
// class-102 store. R84 grounded this: DiskManager writes deposits as pure
// appends with NO block headers, and the block-leading u32 IS the owning
// object's handle (OdBmDepositId(u64 handle)); for element-owned deposits
// the handle IS the element id (head blocks 1098306-1098316 match the
// ElemTable id domain). Keys are restricted to the class-101 record
// counter set (element ids).
type KeyedBinding struct {
	ElementID uint32 `json:"element_id"`
	Offset    int    `json:"-"`
}

// KeyPositions returns the sorted offsets where any of the given counters
// occurs (4-byte aligned) in data.
func KeyPositions(data []byte, counters map[uint32]bool) []KeyedBinding {
	var out []KeyedBinding
	for off := 0; off+4 <= len(data); off += 4 {
		c := binary.LittleEndian.Uint32(data[off:])
		if counters[c] {
			out = append(out, KeyedBinding{ElementID: c, Offset: off})
		}
	}
	return out
}

// bindElementID returns the element of the nearest key at or before off.
func bindElementID(keys []KeyedBinding, off int) (uint32, bool) {
	i := sort.Search(len(keys), func(i int) bool { return keys[i].Offset > off })
	if i == 0 {
		return 0, false
	}
	return keys[i-1].ElementID, true
}

// ScanGLinesKeyed collects inline GLine objects with element attribution.
func ScanGLinesKeyed(data []byte, gLine int16, keys []KeyedBinding) []geom.BoundLine {
	var out []geom.BoundLine
	for off := 0; off+6+48 <= len(data); off += 2 {
		if int16(binary.LittleEndian.Uint16(data[off:])) != gLine {
			continue
		}
		if binary.LittleEndian.Uint32(data[off+2:]) != 0 {
			continue
		}
		var f [6]float64
		finite := true
		nonzero := false
		for k := 0; k < 6; k++ {
			f[k] = math.Float64frombits(binary.LittleEndian.Uint64(data[off+6+k*8:]))
			if math.IsNaN(f[k]) || math.IsInf(f[k], 0) || math.Abs(f[k]) > 1e6 {
				finite = false
				break
			}
			if f[k] != 0 {
				nonzero = true
			}
		}
		if !finite || !nonzero {
			continue
		}
		dx, dy, dz := f[3]-f[0], f[4]-f[1], f[5]-f[2]
		if math.Abs(dx)+math.Abs(dy)+math.Abs(dz) < 1e-9 {
			continue
		}
		elID, hasEl := bindElementID(keys, off)
		if !hasEl {
			continue
		}
		out = append(out, geom.BoundLine{
			ElementID: elID,
			Line: geom.Line{
				Origin:    geom.Point3{X: f[0], Y: f[1], Z: f[2]},
				Direction: geom.Point3{X: f[3], Y: f[4], Z: f[5]},
			},
		})
	}
	return out
}

// ScanGArcsKeyed collects content-anchored arcs with element attribution.
func ScanGArcsKeyed(data []byte, keys []KeyedBinding) []geom.BoundArc {
	var out []geom.BoundArc
	for off := 0; off+80 <= len(data); off += 2 {
		var f [10]float64
		ok := true
		for k := 0; k < 10; k++ {
			f[k] = math.Float64frombits(binary.LittleEndian.Uint64(data[off+k*8:]))
			if math.IsNaN(f[k]) || math.IsInf(f[k], 0) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		xm := math.Sqrt(f[0]*f[0] + f[1]*f[1] + f[2]*f[2])
		ym := math.Sqrt(f[3]*f[3] + f[4]*f[4] + f[5]*f[5])
		dot := f[0]*f[3] + f[1]*f[4] + f[2]*f[5]
		if xm < 0.99 || xm > 1.01 || ym < 0.99 || ym > 1.01 || math.Abs(dot) > 0.01 {
			continue
		}
		if f[6] <= 1e-6 || f[6] > 1e4 {
			continue
		}
		elID, hasEl := bindElementID(keys, off)
		if !hasEl {
			continue
		}
		out = append(out, geom.BoundArc{
			ElementID: elID,
			Arc: geom.Arc{
				XDir:   geom.Point3{X: f[0], Y: f[1], Z: f[2]},
				YDir:   geom.Point3{X: f[3], Y: f[4], Z: f[5]},
				Radius: f[6],
				Center: geom.Point3{X: f[7], Y: f[8], Z: f[9]},
			},
		})
		off += 78
	}
	return out
}

// Inline G-node objects (findings R86): the class-103 G trees carry inline
// fast-path objects as [i16 class][payload] — e.g. GLine(1798) objects are
// [i16 1798][f64 origin][f64 second-point] (48B payload). These hold the
// model-space line geometry of the G trees.

// ScanInlineGLines collects inline GLine[f64×6] objects from the class-103
// G trees with plausibility gates (finite, |v| <= 1e5 ft, non-degenerate).
func ScanInlineGLines(data []byte, gLine int16) []geom.Line {
	var out []geom.Line
	for off := 0; off+2+48 <= len(data); off += 2 {
		if int16(binary.LittleEndian.Uint16(data[off:])) != gLine {
			continue
		}
		var p [6]float64
		plain := true
		nonzero := false
		for k := 0; k < 6; k++ {
			p[k] = math.Float64frombits(binary.LittleEndian.Uint64(data[off+2+k*8:]))
			if math.IsNaN(p[k]) || math.IsInf(p[k], 0) || math.Abs(p[k]) > 1e5 {
				plain = false
				break
			}
			if p[k] != 0 {
				nonzero = true
			}
		}
		if !plain || !nonzero {
			continue
		}
		dx, dy, dz := p[3]-p[0], p[4]-p[1], p[5]-p[2]
		if math.Abs(dx)+math.Abs(dy)+math.Abs(dz) < 1e-9 {
			continue
		}
		out = append(out, geom.Line{
			Origin:    geom.Point3{X: p[0], Y: p[1], Z: p[2]},
			Direction: geom.Point3{X: p[3], Y: p[4], Z: p[5]},
		})
	}
	return out
}

// classindexIndex2023 resolves a class name in the 2023 table (helper for
// tests and tools that do not carry a version).
func classindexIndex2023(name string) (int16, bool) {
	return classindex.Index("2023", name)
}
