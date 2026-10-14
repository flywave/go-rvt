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
		g.End = next
		body := data[pos+18 : next]
		for k := 0; k+2 <= len(body); k += 2 {
			// inline GRep-family object: [i16 cls][GeomBBox 48B][GTightBBox 48B]
			// — the two boxes are byte-identical on real objects (R67); the
			// class tag varies by version/block (v7 uses other tags — R89),
			// so detection is shape-based only.
			if k+2+96 <= len(body) &&
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
			cls := int16(binary.LittleEndian.Uint16(body[k:]))
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
		if math.Hypot(math.Hypot(f[7], f[8]), f[9]) > 1e6 {
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
		if math.Hypot(math.Hypot(f[7], f[8]), f[9]) > 1e6 {
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

// Inline G-node objects (findings R86/R102): the class-103 G trees carry
// inline fast-path objects as [i16 class][payload]. Since R102 the payload
// is decoded with the inlined-GInfo prefix model (see ginfoBodyLen below).
// ---- GInfo inlined-prefix model (R102) -------------------------------------
//
// GNode.m_GInfo is a type-9 object property with loadingMode=0 (static):
// it is serialized INLINE at the start of every G* static body as the raw
// 16-byte struct {Tag i32, ControlCommand i32, GStyleElemId i32, Flags u32}.
// Flags uses the BmGInfoFlags word (E1): Default=0x80004, DefaultModel=0x88004.
// The static body layout (version2023 runtime property descriptors):
//
//	GLine = [GInfo 16][EndParams f64×2 = 16][Origin f64×3][Direction f64×3]
//	GArc  = [GInfo 16][EndParams 16][XDir 24][YDir 24][Radius 8][C 24][Filled]
//	Nurb  = [GInfo 16][EndParams 16][Degree i32][Closed b][Rational b]...
//
// GRep is an observed exception (no GInfo prefix before its bboxes).

// ginfoFlagsOK reports whether w is a plausible BmGInfoFlags word (E1 §3b):
// Default=0x80004, DefaultModel=0x88004, plus observed per-element variants
// (0x88084, 0x1088244, ...) — all carry IsPickable=0x4; the empty word is
// also legal. Words without IsPickable (e.g. catalog noise 0xffff) reject.
func ginfoFlagsOK(w uint32) bool {
	if w == 0xffff {
		return false // catalog-noise word (undefined bit mix)
	}
	return w == 0 || w&0x4 == 0x4
}

// hasGInfoPrefix reports whether an object payload starts with the inlined
// GInfo struct (R102). GStyleElemId uses -1 for "null" (R38).
func hasGInfoPrefix(p []byte) bool {
	if len(p) < 16 {
		return false
	}
	w0 := int32(binary.LittleEndian.Uint32(p[0:]))
	w1 := int32(binary.LittleEndian.Uint32(p[4:]))
	w2 := int32(binary.LittleEndian.Uint32(p[8:]))
	w3 := binary.LittleEndian.Uint32(p[12:])
	if !(w0 >= -1 && w0 <= 64) {
		return false
	}
	if w1 != 0 {
		return false
	}
	if !(w2 == -1 || (w2 >= 0 && w2 < 1<<24)) {
		return false
	}
	return ginfoFlagsOK(w3)
}

// ginfoBodyLen is the fixed GInfo(16) + EndParams(16) prefix shared by all
// GCurve-derived static bodies.
const ginfoBodyLen = 32

// ScanInlineGLines collects inline GLine objects from the class-103 trees
// using the GInfo prefix model (R102):
// [i16 GLine][GInfo 16][EndParams 16][Origin 24][Direction 24].
func ScanInlineGLines(data []byte, gLine int16) []geom.Line {
	var out []geom.Line
	for off := 0; off+2+ginfoBodyLen+48 <= len(data); off += 2 {
		if int16(binary.LittleEndian.Uint16(data[off:])) != gLine {
			continue
		}
		p := data[off+2:]
		if !hasGInfoPrefix(p) {
			continue
		}
		q := p[ginfoBodyLen:]
		var f [6]float64
		plain := true
		for k := 0; k < 6; k++ {
			f[k] = math.Float64frombits(binary.LittleEndian.Uint64(q[k*8:]))
			if math.IsNaN(f[k]) || math.IsInf(f[k], 0) || math.Abs(f[k]) > 1e5 {
				plain = false
				break
			}
		}
		if !plain {
			continue
		}
		if f[0] == f[1] && f[1] == f[2] && f[3] == f[4] && f[4] == f[5] {
			continue // fully degenerate
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

// classindexIndex2023 resolves a class name in the 2023 table (helper for
// tests and tools that do not carry a version).
func classindexIndex2023(name string) (int16, bool) {
	return classindex.Index("2023", name)
}

// InlineObj is one inline G-node object attributed to its enclosing tree.
type InlineObj struct {
	ElementID uint32 `json:"element_id"`
	Class     int16  `json:"class"`
	Name      string `json:"name,omitempty"`
	Offset    int    `json:"-"`
}

// InlineArc is one decoded GArc instance (reader order: XDirection,
// YDirection, Radius, Center, IsFilled — R54/R86 method). StartAngle/
// EndAngle carry the GCurve EndParams when they decode (0,0 = unknown).
type InlineArc struct {
	ElementID  uint32      `json:"element_id"`
	XDir       geom.Point3 `json:"xdir"`
	YDir       geom.Point3 `json:"ydir"`
	Radius     float64     `json:"radius"`
	Center     geom.Point3 `json:"center"`
	StartAngle float64     `json:"start_angle,omitempty"`
	EndAngle   float64     `json:"end_angle,omitempty"`
}

// isUnitish: |v| within [0.99, 1.01].
func isUnitish(x, y, z float64) bool {
	m := math.Sqrt(x*x + y*y + z*z)
	return m > 0.99 && m < 1.01
}

// dot3 of two vectors given as consecutive float64 triples starting at b.
func dot3(b []byte) float64 {
	x1 := math.Float64frombits(binary.LittleEndian.Uint64(b))
	y1 := math.Float64frombits(binary.LittleEndian.Uint64(b[8:]))
	z1 := math.Float64frombits(binary.LittleEndian.Uint64(b[16:]))
	x2 := math.Float64frombits(binary.LittleEndian.Uint64(b[24:]))
	y2 := math.Float64frombits(binary.LittleEndian.Uint64(b[32:]))
	z2 := math.Float64frombits(binary.LittleEndian.Uint64(b[40:]))
	return x1*x2 + y1*y2 + z1*z2
}

// ScanInlineGArcs collects GArc instances with the validated full-frame
// layout (R102b): [i16 GArc][GInfo 16B][EndParams 16B][XDir 24][YDir 24]
// [Radius 8][Center 24][IsFilled 1]. Strict gates: finite values, unit
// orthogonal X/Y axes, positive radius, finite center.
func ScanInlineGArcs(data []byte, gArc int16, lookup func(int16) string) []geom.Arc {
	var out []geom.Arc
	f64 := func(b []byte, o int) float64 {
		return math.Float64frombits(binary.LittleEndian.Uint64(b[o:]))
	}
	fin := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

	for off := 0; off+2+113 <= len(data); off += 2 {
		if int16(binary.LittleEndian.Uint16(data[off:])) != gArc {
			continue
		}
		p := data[off+2:]

		// GInfo 16B at +0: {Tag, CC, StyleElemId, Flags}
		gTag := int32(binary.LittleEndian.Uint32(p[0:]))
		gFlags := binary.LittleEndian.Uint32(p[12:])

		// EndParams 2×f64 at +16
		ep0 := f64(p, 16)
		ep1 := f64(p, 24)
		if !fin(ep0) || !fin(ep1) {
			continue
		}

		// XDir 3×f64 at +32: unit vector
		x1, x2, x3 := f64(p, 32), f64(p, 40), f64(p, 48)
		xm := math.Sqrt(x1*x1 + x2*x2 + x3*x3)
		if !fin(x1) || !fin(x2) || !fin(x3) || xm < 0.9 || xm > 1.1 {
			continue
		}

		// YDir 3×f64 at +56: unit, orthogonal to X
		y1, y2, y3 := f64(p, 56), f64(p, 64), f64(p, 72)
		ym := math.Sqrt(y1*y1 + y2*y2 + y3*y3)
		if !fin(y1) || !fin(y2) || !fin(y3) || ym < 0.9 || ym > 1.1 {
			continue
		}
		dot := x1*y1 + x2*y2 + x3*y3
		if math.Abs(dot) > 0.05 {
			continue
		}

		// Radius at +80
		radius := f64(p, 80)
		if math.IsNaN(radius) || radius <= 1e-6 || radius > 1e4 {
			continue
		}

		// Center 24B at +88: finite 3D point
		c1, c2, c3 := f64(p, 88), f64(p, 96), f64(p, 104)
		if !fin(c1) || !fin(c2) || !fin(c3) {
			continue
		}

		// IsFilled at +112
		_ = p[112]

		out = append(out, geom.Arc{
			XDir:       geom.Point3{X: x1, Y: x2, Z: x3},
			YDir:       geom.Point3{X: y1, Y: y2, Z: y3},
			Radius:     radius,
			Center:     geom.Point3{X: c1, Y: c2, Z: c3},
			StartAngle: ep0,
			EndAngle:   ep1,
		})
		_ = gTag
		_ = gFlags
	}
	return out
}

// ScanInlineGLinesKeyed attributes inline GLine objects to their enclosing
// G-tree element (tree counter) — the class-103 trees carry both the bboxes
// and the inline geometry, so attribution is exact by range.
func ScanInlineGLinesKeyed(data []byte, gLine int16, trees []GTree) []geom.BoundLine {
	var out []geom.BoundLine
	for _, tr := range trees {
		body := data[tr.Offset+18 : tr.End]
		for k := 0; k+2+ginfoBodyLen+48 <= len(body); k += 2 {
			if int16(binary.LittleEndian.Uint16(body[k:])) != gLine {
				continue
			}
			p0 := body[k+2:]
			if !hasGInfoPrefix(p0) {
				continue
			}
			q := p0[ginfoBodyLen:]
			var p [6]float64
			plain := true
			nonzero := false
			for kk := 0; kk < 6; kk++ {
				p[kk] = math.Float64frombits(binary.LittleEndian.Uint64(q[kk*8:]))
				if math.IsNaN(p[kk]) || math.IsInf(p[kk], 0) || math.Abs(p[kk]) > 1e5 {
					plain = false
					break
				}
				if p[kk] != 0 {
					nonzero = true
				}
			}
			if !plain || !nonzero {
				continue
			}
			dx := p[3] - p[0]
			dy := p[4] - p[1]
			dz := p[5] - p[2]
			if math.Abs(dx)+math.Abs(dy)+math.Abs(dz) < 1e-9 {
				continue
			}
			out = append(out, geom.BoundLine{
				ElementID: tr.Counter,
				Line: geom.Line{
					Origin:    geom.Point3{X: p[0], Y: p[1], Z: p[2]},
					Direction: geom.Point3{X: p[3], Y: p[4], Z: p[5]},
				},
			})
		}
	}
	return out
}

// dotMagnitude returns the product of the two vector magnitudes of six
// f64s at b (for relative-orthogonality gating).
func dotMagnitude(b []byte) float64 {
	x1 := math.Float64frombits(binary.LittleEndian.Uint64(b))
	y1 := math.Float64frombits(binary.LittleEndian.Uint64(b[8:]))
	z1 := math.Float64frombits(binary.LittleEndian.Uint64(b[16:]))
	x2 := math.Float64frombits(binary.LittleEndian.Uint64(b[24:]))
	y2 := math.Float64frombits(binary.LittleEndian.Uint64(b[32:]))
	z2 := math.Float64frombits(binary.LittleEndian.Uint64(b[40:]))
	return math.Sqrt(x1*x1+y1*y1+z1*z1) * math.Sqrt(x2*x2+y2*y2+z2*z2)
}

// ScanInlineGArcsAttributed scans the whole class-103 G region for GArc
// instances and attributes each to the nearest-preceding G-tree counter
// (inter-group objects belong to the neighbouring element's G-tree
// neighborhood — R91). Trees must be sorted by Offset.
func ScanInlineGArcsAttributed(data []byte, trees []GTree, gArc int16, lookup func(int16) string) []InlineArc {
	var out []InlineArc
	// group start offsets (attraction anchors)
	starts := make([]int, 0, len(trees))
	ctrs := make([]uint32, 0, len(trees))
	for _, tr := range trees {
		starts = append(starts, tr.Offset)
		ctrs = append(ctrs, tr.Counter)
	}
	for off := 0; off+2+99 <= len(data); off += 2 {
		if int16(binary.LittleEndian.Uint16(data[off:])) != gArc {
			continue
		}
		base := off + 2
		ep0, ep1 := f64at2(data, base), f64at2(data, base+8)
		// EP denormal kill: a real curve parameter is exactly 0 or a true
		// radian measure; denormal-scale values (1e-14…1e-308) mark frame
		// misalignment (observed: XDir reading the Center shifted one slot).
		if !fin2(ep0) || !fin2(ep1) || math.Abs(ep0) > 100 || math.Abs(ep1) > 100 {
			continue
		}
		if (ep0 != 0 && math.Abs(ep0) < 1e-6) || (ep1 != 0 && math.Abs(ep1) < 1e-6) {
			continue
		}
		x1, x2, x3 := f64at2(data, base+16), f64at2(data, base+24), f64at2(data, base+32)
		if !fin2(x1) || !fin2(x2) || !fin2(x3) {
			continue
		}
		if math.Abs(x1) < 1e-9 && math.Abs(x2) < 1e-9 && math.Abs(x3) < 1e-9 {
			continue
		}
		if math.Hypot(math.Hypot(x1, x2), x3) > 1e3 {
			continue
		}
		y1, y2, y3 := f64at2(data, base+40), f64at2(data, base+48), f64at2(data, base+56)
		if !fin2(y1) || !fin2(y2) || !fin2(y3) {
			continue
		}
		if math.Hypot(math.Hypot(y1, y2), y3) > 1e3 {
			continue
		}
		if math.Abs(x1*y1+x2*y2+x3*y3) > 0.05*vecMag6(data[base:base+48]) {
			continue
		}
		r := f64at2(data, base+64)
		if !fin2(r) || r <= 1e-6 || r > 1e4 {
			continue
		}
		c1, c2, c3 := f64at2(data, base+72), f64at2(data, base+80), f64at2(data, base+88)
		if !fin2(c1) || !fin2(c2) || !fin2(c3) {
			continue
		}
		if math.Hypot(math.Hypot(c1, c2), c3) > 1e6 {
			continue
		}
		// nearest preceding tree counter
		i := sort.Search(len(starts), func(i int) bool { return starts[i] > off }) - 1
		if i < 0 {
			continue
		}
		out = append(out, InlineArc{
			ElementID:  ctrs[i],
			XDir:       geom.Point3{X: x1, Y: x2, Z: x3},
			YDir:       geom.Point3{X: y1, Y: y2, Z: y3},
			Radius:     r,
			Center:     geom.Point3{X: c1, Y: c2, Z: c3},
			StartAngle: ep0,
			EndAngle:   ep1,
		})
	}
	return out
}

func f64at2(b []byte, off int) float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(b[off:]))
}

func fin2(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func vecMag6(b []byte) float64 {
	var s float64
	for k := 0; k+8 <= 48; k += 8 {
		v := math.Float64frombits(binary.LittleEndian.Uint64(b[k:]))
		s += v * v
	}
	return math.Sqrt(s)
}

// NurbSpline scanner (R101): GNurbSpline body per R98 schema:
//	[Degree i32][Closed bool][Rational bool]
//	[ControlPoints: count i32 + count × WPoint(weight,x,y,z) = 4×f64 each]
//	[Knots: count i32 + count × f64]
// GNode lazy fields (GInfo) deferred to dynamic section (R97).

// WPoint is one NURBS control point (weight + 3D position).
type WPoint struct {
	W, X, Y, Z float64
}

// NurbSplineData is a decoded GNurbSpline inline object.
type NurbSplineData struct {
	ElementID     uint32    `json:"element_id"`
	Degree        int32     `json:"degree"`
	Closed        bool      `json:"closed"`
	Rational      bool      `json:"rational"`
	ControlPoints []WPoint  `json:"control_points"`
	Knots         []float64 `json:"knots"`
}

// ScanNurbSplines scans class-103 G trees for GNurbSpline inline objects
// using the R98 schema under the R102 GInfo prefix model:
// [i16 Nurb][GInfo 16][EndParams 16][Degree i32][Closed bool][Rational bool]
// [ControlPoints: i32 count + count×WPoint 4×f64][Knots: i32 count + count×f64],
// attributed via tree counter. Samples so far carry no physics-valid
// splines (R73/R101); the scanner is schema-anchored for richer files.
func ScanNurbSplines(data []byte, gNurb int16, trees []GTree) []NurbSplineData {
	var out []NurbSplineData
	for _, tr := range trees {
		body := data[tr.Offset+18 : tr.End]
		for k := 0; k+2+ginfoBodyLen+8 <= len(body); k += 2 {
			if int16(binary.LittleEndian.Uint16(body[k:])) != gNurb {
				continue
			}
			hdr := body[k+2:]
			if !hasGInfoPrefix(hdr) {
				continue
			}
			ns := NurbSplineData{ElementID: tr.Counter}
			p := hdr[ginfoBodyLen:] // skip GInfo + EndParams
			// Degree i32
			if len(p) < 4 {
				continue
			}
			ns.Degree = int32(binary.LittleEndian.Uint32(p))
			p = p[4:]
			// Closed bool
			if len(p) < 1 {
				continue
			}
			ns.Closed = p[0] != 0
			p = p[1:]
			// ControlPoints: count i32 + count × 4×f64 (WPoint)
			if len(p) < 4 {
				continue
			}
			cpCount := int(binary.LittleEndian.Uint32(p))
			p = p[4:]
			if cpCount < 0 || cpCount > 4096 || len(p) < cpCount*32 {
				continue
			}
			for j := 0; j < cpCount; j++ {
				w := math.Float64frombits(binary.LittleEndian.Uint64(p))
				x := math.Float64frombits(binary.LittleEndian.Uint64(p[8:]))
				y := math.Float64frombits(binary.LittleEndian.Uint64(p[16:]))
				z := math.Float64frombits(binary.LittleEndian.Uint64(p[24:]))
				if math.IsNaN(w) || math.IsInf(w, 0) || math.Abs(w) > 1e6 ||
					math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > 1e6 ||
					math.IsNaN(y) || math.IsInf(y, 0) || math.Abs(y) > 1e6 ||
					math.IsNaN(z) || math.IsInf(z, 0) || math.Abs(z) > 1e6 {
					cpCount = 0
					break
				}
				ns.ControlPoints = append(ns.ControlPoints, WPoint{W: w, X: x, Y: y, Z: z})
				p = p[32:]
			}
			if cpCount == 0 {
				continue
			}
			// Knots: count i32 + count × f64
			if len(p) < 4 {
				continue
			}
			knCount := int(binary.LittleEndian.Uint32(p))
			p = p[4:]
			if knCount < 0 || knCount > 4096 || len(p) < knCount*8 {
				continue
			}
			for j := 0; j < knCount; j++ {
				kv := math.Float64frombits(binary.LittleEndian.Uint64(p))
				if math.IsNaN(kv) || math.IsInf(kv, 0) {
					knCount = 0
					break
				}
				ns.Knots = append(ns.Knots, kv)
				p = p[8:]
			}
			if ns.Degree >= 1 && ns.Degree <= 10 && len(ns.ControlPoints) > 0 {
				out = append(out, ns)
			}
		}
	}
	return out
}

// PolyMeshData is a decoded GPolyMesh inline object (R98 schema).
type PolyMeshData struct {
	ElementID   uint32 `json:"element_id"`
	StyleID     int32  `json:"style_id"`
	MaterialID  int32  `json:"material_id"`
	Flags       uint32 `json:"flags"`
	FacetedTopo []byte `json:"faceted_topology_raw"`
}

// ScanPolyMeshes scans class-103 G trees for GPolyMesh inline objects.
// PolyMesh body: [InteriorGStyleID i32][MaterialID ElementId i32]
//
//	[PolyMeshFlags u32][FacetedTopology obj ref — variable].
//
// The FacetedTopology is stored as raw bytes (layout requires reader REA).
func ScanPolyMeshes(data []byte, gPoly int16, trees []GTree) []PolyMeshData {
	var out []PolyMeshData
	for _, tr := range trees {
		body := data[tr.Offset+18 : tr.End]
		for k := 0; k+2+ginfoBodyLen+12 <= len(body); k += 2 {
			if int16(binary.LittleEndian.Uint16(body[k:])) != gPoly {
				continue
			}
			hdr := body[k+2:]
			if !hasGInfoPrefix(hdr) {
				continue
			}
			// R102 runtime descriptors: m_pFacetedTopology is loadingMode=1
			// (dynamic → NOT in the static body); static = polyMeshFlags,
			// interiorGStyleID, materialID (i32 each).
			p := hdr[ginfoBodyLen:]
			pm := PolyMeshData{ElementID: tr.Counter}
			pm.Flags = binary.LittleEndian.Uint32(p)
			pm.StyleID = int32(binary.LittleEndian.Uint32(p[4:]))
			pm.MaterialID = int32(binary.LittleEndian.Uint32(p[8:]))
			out = append(out, pm)
		}
	}
	return out
}
