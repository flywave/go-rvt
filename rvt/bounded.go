package rvt

// Element serialised data and its bounded-curve records (RE-49/RE-75;
// ported from the rvt-rs reference implementation — partition_beam_axes /
// partition_names — and cross-validated on rac_basic_2023: 56 of 56
// placed walls carry a location line).
//
// An element's serialised data opens with a 10-byte header — four 0xff,
// the u16 tag of the schema class `CellList`, 01 00 00 00 — followed by
// the element's id (u32 + four non-zero bytes on Revit 2023, u64 on 2024
// and later). The data proper starts 8 bytes past the id's first byte.
// Inside it, a bounded line reads
//
//	+0   04 00 08 01   BOUNDED_LINE_TAG
//	+4   f64           start parameter, feet
//	+12  f64           end parameter, feet
//	+20  f64 × 3       origin, model feet
//	+44  f64 × 3       unit direction
//
// running from origin + start·direction to origin + end·direction. A
// bounded arc is the same tag preceded by a u64 1, laid out
//
//	+4   f64           start angle, radians
//	+12  f64           end angle, radians
//	+20  f64 × 3       unit X axis
//	+44  f64 × 3       unit Y axis, square to X
//	+68  f64           radius, feet
//	+76  f64 × 3       centre, model feet
//
// running from centre + radius·(cos a·X + sin a·Y) at the start angle to
// the end angle. The line/arcs are walls' location curves (2023-2026),
// beams' and sketch lines' exact geometry where present.

import (
	"encoding/binary"
	"math"
)

// BoundedLineTag opens every bounded-curve record.
var BoundedLineTag = [4]byte{0x04, 0x00, 0x08, 0x01}

// BoundedLine is one straight location line.
type BoundedLine struct {
	StartParam float64 `json:"start_param"`
	EndParam   float64 `json:"end_param"`
	Origin     [3]float64 `json:"origin"`
	Direction  [3]float64 `json:"direction"` // unit
}

// Endpoints returns the line's two endpoints in model feet.
func (l *BoundedLine) Endpoints() ([3]float64, [3]float64) {
	var a, b [3]float64
	for k := 0; k < 3; k++ {
		a[k] = l.Origin[k] + l.StartParam*l.Direction[k]
		b[k] = l.Origin[k] + l.EndParam*l.Direction[k]
	}
	return a, b
}

// BoundedArc is one circular location arc.
type BoundedArc struct {
	StartAngle float64    `json:"start_angle"`
	EndAngle   float64    `json:"end_angle"`
	XAxis      [3]float64 `json:"x_axis"` // unit
	YAxis      [3]float64 `json:"y_axis"` // unit, square to X
	Radius     float64    `json:"radius"`
	Center     [3]float64 `json:"center"`
}

// PointAt returns the arc point at parameter t (radians in the X/Y basis).
func (a *BoundedArc) PointAt(t float64) [3]float64 {
	var p [3]float64
	c, s := math.Cos(t), math.Sin(t)
	for k := 0; k < 3; k++ {
		p[k] = a.Center[k] + a.Radius*(c*a.XAxis[k]+s*a.YAxis[k])
	}
	return p
}

// elementDataHeader derives the element-data header from the file's own
// schema: four 0xff, the CellList class tag, 01 00 00 00. (The release
// constants in the reference implementation — 0x02c0/0x02d3/0x02ef/0x02fd
// for 2023-2026 — are all that class's tag per release.)
func elementDataHeader(schema *Schema) ([10]byte, bool) {
	var h [10]byte
	c := schema.ByName("CellList")
	if c == nil {
		return h, false
	}
	for i := 0; i < 4; i++ {
		h[i] = 0xff
	}
	binary.LittleEndian.PutUint16(h[4:], c.Tag)
	h[6], h[7], h[8], h[9] = 1, 0, 0, 0
	return h, true
}

func boundedLineAt(data []byte, at int) (*BoundedLine, bool) {
	if at+80 > len(data) {
		return nil, false
	}
	start := math.Float64frombits(binary.LittleEndian.Uint64(data[at+4:]))
	end := math.Float64frombits(binary.LittleEndian.Uint64(data[at+12:]))
	var l BoundedLine
	l.StartParam, l.EndParam = start, end
	for k := 0; k < 3; k++ {
		l.Origin[k] = math.Float64frombits(binary.LittleEndian.Uint64(data[at+20+k*8:]))
		l.Direction[k] = math.Float64frombits(binary.LittleEndian.Uint64(data[at+44+k*8:]))
	}
	mag := math.Sqrt(l.Direction[0]*l.Direction[0] + l.Direction[1]*l.Direction[1] + l.Direction[2]*l.Direction[2])
	if !isFiniteRec(start) || !isFiniteRec(end) || end < start ||
		math.Abs(mag-1) > 0.01 ||
		!isFiniteRec(l.Origin[0]) || !isFiniteRec(l.Origin[1]) || !isFiniteRec(l.Origin[2]) ||
		math.Hypot(math.Hypot(l.Origin[0], l.Origin[1]), l.Origin[2]) > 1e6 {
		return nil, false
	}
	return &l, true
}

func boundedArcAt(data []byte, at int) (*BoundedArc, bool) {
	if at < 8 || at+84 > len(data) {
		return nil, false
	}
	if binary.LittleEndian.Uint64(data[at-8:]) != 1 {
		return nil, false
	}
	var a BoundedArc
	a.StartAngle = math.Float64frombits(binary.LittleEndian.Uint64(data[at+4:]))
	a.EndAngle = math.Float64frombits(binary.LittleEndian.Uint64(data[at+12:]))
	for k := 0; k < 3; k++ {
		a.XAxis[k] = math.Float64frombits(binary.LittleEndian.Uint64(data[at+20+k*8:]))
		a.YAxis[k] = math.Float64frombits(binary.LittleEndian.Uint64(data[at+44+k*8:]))
	}
	a.Radius = math.Float64frombits(binary.LittleEndian.Uint64(data[at+68:]))
	for k := 0; k < 3; k++ {
		a.Center[k] = math.Float64frombits(binary.LittleEndian.Uint64(data[at+76+k*8:]))
	}
	xm := math.Sqrt(a.XAxis[0]*a.XAxis[0] + a.XAxis[1]*a.XAxis[1] + a.XAxis[2]*a.XAxis[2])
	ym := math.Sqrt(a.YAxis[0]*a.YAxis[0] + a.YAxis[1]*a.YAxis[1] + a.YAxis[2]*a.YAxis[2])
	dot := a.XAxis[0]*a.YAxis[0] + a.XAxis[1]*a.YAxis[1] + a.XAxis[2]*a.YAxis[2]
	sweep := a.EndAngle - a.StartAngle
	ok := isFiniteRec(a.StartAngle) && isFiniteRec(a.EndAngle) &&
		math.Abs(sweep) > 1e-9 && math.Abs(sweep) <= 2*math.Pi+1e-9 &&
		math.Abs(xm-1) < 0.01 && math.Abs(ym-1) < 0.01 && math.Abs(dot) < 0.05 &&
		a.Radius > 0 && a.Radius < 1e4 &&
		math.Hypot(math.Hypot(a.Center[0], a.Center[1]), a.Center[2]) < 1e6
	if !ok {
		return nil, false
	}
	return &a, true
}

// firstBoundedLine returns the first valid line record in data, provided
// the first curve record is not an arc.
func firstBoundedLine(data []byte) (*BoundedLine, bool) {
	first := indexOfBytes(data, BoundedLineTag[:], 0)
	if first < 0 {
		return nil, false
	}
	if _, isArc := boundedArcAt(data, first); isArc {
		return nil, false
	}
	for at := first; ; {
		if l, ok := boundedLineAt(data, at); ok {
			return l, true
		}
		next := indexOfBytes(data, BoundedLineTag[:], at+4)
		if next < 0 {
			return nil, false
		}
		at = next
	}
}

// firstBoundedArc returns the arc record when the first curve record in
// data is one.
func firstBoundedArc(data []byte) (*BoundedArc, bool) {
	first := indexOfBytes(data, BoundedLineTag[:], 0)
	if first < 0 {
		return nil, false
	}
	return boundedArcAt(data, first)
}

func indexOfBytes(haystack []byte, needle []byte, from int) int {
	if from < 0 {
		from = 0
	}
	for i := from; i+len(needle) <= len(haystack); i++ {
		match := true
		for k := range needle {
			if haystack[i+k] != needle[k] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// ScanFirstBoundedLines returns the location line in the element data of
// each id in `ids` (e.g. placed walls), from the partition's element-data
// records. An id whose copies disagree is dropped. Only releases whose
// element-data layout is measured yield results (2023-2026; fail closed).
//
// Callers must hold the whole record scan when they need arc walls too:
// a wall whose first curve record is an arc yields no line here.
func ScanFirstBoundedLines(p *Partition, schema *Schema, ids map[uint32]bool, version int) map[uint32]*BoundedLine {
	out := scanFirstBounded(p, schema, ids, version,
		func(data []byte) (interface{}, bool) {
			if _, isArc := firstBoundedArc(data); isArc {
				return nil, false
			}
			return firstBoundedLine(data)
		})
	res := make(map[uint32]*BoundedLine, len(out))
	for id, v := range out {
		if l, ok := v.(*BoundedLine); ok {
			res[id] = l
		}
	}
	return res
}

// ScanFirstBoundedArcs is ScanFirstBoundedLines for arc location curves.
func ScanFirstBoundedArcs(p *Partition, schema *Schema, ids map[uint32]bool, version int) map[uint32]*BoundedArc {
	out := scanFirstBounded(p, schema, ids, version,
		func(data []byte) (interface{}, bool) {
			return firstBoundedArc(data)
		})
	res := make(map[uint32]*BoundedArc, len(out))
	for id, v := range out {
		if a, ok := v.(*BoundedArc); ok {
			res[id] = a
		}
	}
	return res
}

// scanFirstBounded walks every element-data record whose id is wanted and
// reads its first bounded curve with `read`. An id whose occurrences
// disagree is dropped.
func scanFirstBounded(p *Partition, schema *Schema, ids map[uint32]bool, version int, read func([]byte) (interface{}, bool)) map[uint32]interface{} {
	if schema == nil || version < 2023 || version > 2026 || len(ids) == 0 {
		return nil
	}
	header, ok := elementDataHeader(schema)
	if !ok {
		return nil
	}
	wideID := version >= 2024
	found := map[uint32]interface{}{}
	for _, ss := range p.SubStreams {
		d := ss.Data
		for at := 0; at+18 <= len(d); {
			idx := indexOfBytes(d, header[:], at)
			if idx < 0 {
				break
			}
			idAt := idx + len(header)
			var id uint32
			if wideID {
				u := binary.LittleEndian.Uint64(d[idAt:])
				if u > math.MaxUint32 {
					at = idx + 1
					continue
				}
				id = uint32(u)
			} else {
				id = binary.LittleEndian.Uint32(d[idAt:])
			}
			var next int
			if nextIdx := indexOfBytes(d, header[:], idx+1); nextIdx >= 0 {
				next = nextIdx
			} else {
				next = len(d)
			}
			at = next
			if !ids[id] {
				continue
			}
			dataStart := idAt + 8
			end := next
			if end > dataStart+0x10000 {
				end = dataStart + 0x10000
			}
			if dataStart >= end {
				continue
			}
			keep, ok := read(d[dataStart:end])
			if !ok {
				continue
			}
			if held, seen := found[id]; seen {
				if !boundedEqual(held, keep) {
					found[id] = nil
				}
			} else {
				found[id] = keep
			}
		}
	}
	for id, v := range found {
		if v == nil {
			delete(found, id)
		}
	}
	return found
}

func boundedEqual(a, b interface{}) bool {
	switch x := a.(type) {
	case *BoundedLine:
		y, ok := b.(*BoundedLine)
		return ok && *x == *y
	case *BoundedArc:
		y, ok := b.(*BoundedArc)
		return ok && *x == *y
	}
	return false
}
