package rvt

// Family-instance placement transforms (RE-87; ported from the rvt-rs
// reference implementation — partition_instance_transforms.rs).
//
// A family instance's serialised data holds its placement as twelve f64:
// a 3×3 rotation stored row by row, then the origin in model feet.
//
//	[Xx Yx Zx Xy Yy Zy Xz Yz Zz ox oy oz]
//
// The rotation's columns are the instance's X, Y and Z axes. The reader
// takes the first place in the data (within a 0x1000 window) where nine
// f64 form a rotation: orthonormal with determinant 1.

import (
	"encoding/binary"
	"math"
)

// instanceTransformWindow is how far into an instance's data its
// transform is looked for.
const instanceTransformWindow = 0x1000

// rotationTolerance is how closely the nine values must form a rotation.
const rotationTolerance = 1e-9

// InstanceTransform is a family instance's placement: unit X/Y/Z axes
// and the origin, in model feet.
type InstanceTransform struct {
	Axes   [3][3]float64 `json:"axes"` // columns: instance X, Y, Z
	Origin [3]float64    `json:"origin"`
}

// IsUpright reports whether the instance's Z axis is the model's.
func (t *InstanceTransform) IsUpright() bool {
	return math.Abs(t.Axes[2][0]) <= rotationTolerance &&
		math.Abs(t.Axes[2][1]) <= rotationTolerance &&
		math.Abs(t.Axes[2][2]-1) <= rotationTolerance
}

// PlanAngle returns the plan angle of the X axis, radians.
func (t *InstanceTransform) PlanAngle() float64 {
	return math.Atan2(t.Axes[0][1], t.Axes[0][0])
}

func rotationAt(d []byte, at int) ([3][3]float64, bool) {
	var rows [3][3]float64
	for i := 0; i < 9; i++ {
		if at+8*(i+1) > len(d) {
			return rows, false
		}
		v := math.Float64frombits(binary.LittleEndian.Uint64(d[at+8*i:]))
		if !isFiniteRec(v) || math.Abs(v) > 1+rotationTolerance {
			return rows, false
		}
		rows[i/3][i%3] = v
	}
	dot := func(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			want := 0.0
			if i == j {
				want = 1
			}
			if math.Abs(dot(rows[i], rows[j])-want) > rotationTolerance {
				return rows, false
			}
		}
	}
	a, b, c := rows[0], rows[1], rows[2]
	det := a[0]*(b[1]*c[2]-b[2]*c[1]) - a[1]*(b[0]*c[2]-b[2]*c[0]) + a[2]*(b[0]*c[1]-b[1]*c[0])
	if math.Abs(det-1) > rotationTolerance {
		return rows, false
	}
	return rows, true
}

func instanceTransform(d []byte) (*InstanceTransform, bool) {
	end := len(d)
	if end > instanceTransformWindow {
		end = instanceTransformWindow
	}
	for at := 0; at+96 <= end; at++ {
		rows, ok := rotationAt(d, at)
		if !ok {
			continue
		}
		var t InstanceTransform
		t.Origin = [3]float64{
			math.Float64frombits(binary.LittleEndian.Uint64(d[at+72:])),
			math.Float64frombits(binary.LittleEndian.Uint64(d[at+80:])),
			math.Float64frombits(binary.LittleEndian.Uint64(d[at+88:])),
		}
		if !isFiniteRec(t.Origin[0]) || !isFiniteRec(t.Origin[1]) || !isFiniteRec(t.Origin[2]) {
			continue
		}
		for j := 0; j < 3; j++ {
			t.Axes[j] = [3]float64{rows[0][j], rows[1][j], rows[2][j]}
		}
		return &t, true
	}
	return nil, false
}

// ScanInstanceTransforms returns the placement transform of each element
// in `ids` whose data holds one (family instances). An instance whose
// copies disagree is dropped. Only releases with a measured element-data
// layout yield results (2023-2026; fail closed).
func ScanInstanceTransforms(p *Partition, schema *Schema, ids map[uint32]bool, version int) map[uint32]InstanceTransform {
	if schema == nil || version < 2023 || version > 2026 || len(ids) == 0 {
		return nil
	}
	hdr, ok := elementDataHeader(schema)
	if !ok {
		return nil
	}
	wideID := version >= 2024
	found := map[uint32]InstanceTransform{}
	for _, ss := range p.SubStreams {
		d := ss.Data
		for at := 0; at+18 <= len(d); {
			idx := indexOfBytes(d, hdr[:], at)
			if idx < 0 {
				break
			}
			var id uint32
			if wideID {
				u := binary.LittleEndian.Uint64(d[idx+10:])
				if u > 0xffffffff {
					at = idx + 1
					continue
				}
				id = uint32(u)
			} else {
				id = binary.LittleEndian.Uint32(d[idx+10:])
			}
			next := len(d)
			if n := indexOfBytes(d, hdr[:], idx+1); n >= 0 {
				next = n
			}
			at = next
			if !ids[id] {
				continue
			}
			dataStart := idx + 18
			end := next
			if end > dataStart+instanceTransformWindow {
				end = dataStart + instanceTransformWindow
			}
			if dataStart >= end {
				continue
			}
			t, ok := instanceTransform(d[dataStart:end])
			if !ok {
				continue
			}
			if held, seen := found[id]; seen {
				if held != *t {
					delete(found, id)
				}
			} else {
				found[id] = *t
			}
		}
	}
	return found
}
