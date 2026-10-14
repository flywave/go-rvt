package rvt

// Type compound structures: the layer lists of wall/floor/roof/ceiling
// types (ported from the rvt-rs reference implementation —
// partition_compound_structure.rs — where the 2023 layer layout is
// measured: 29-byte records, function at +8, material at +16, deck at
// +20, u32 ids). A type's layers frame a u32 count behind four 0xff and
// the release's layer-frame tag, or behind the type's UTF-16LE name.
// Summing the layer widths gives the type's thickness (walls draw half
// of it either side of their location line).

import (
	"encoding/binary"
	"math"
)

// CompoundLayer is one layer of a type's compound structure.
type CompoundLayer struct {
	WidthFeet float64 `json:"width_feet"`
	Material  *uint32 `json:"material,omitempty"` // nil: takes its category's
	Function  uint32  `json:"function"`
}

// compoundLayerLayout2023 is the layer record layout measured on 2023.
var compoundLayerLayout2023 = struct {
	recordLen  int
	functionAt int
	materialAt int
	deckAt     int
}{recordLen: 29, functionAt: 8, materialAt: 16, deckAt: 20}

// maxCompoundLayers bounds a type's layer count.
const maxCompoundLayers = 32

// layerFrameTag2023 frames a 2023 type's layer count.
var layerFrameTag2023 = [2]byte{0x6f, 0x10}

// maxNameUnits bounds the UTF-16 name a layer count may sit behind.
const maxNameUnits = 256

// compoundLayersAt reads the layer list whose count sits at `at`.
func compoundLayersAt(buf []byte, at int, declared map[uint32]bool) ([]CompoundLayer, bool) {
	if at < 0 || at+4 > len(buf) {
		return nil, false
	}
	count := int(binary.LittleEndian.Uint32(buf[at:]))
	if count < 1 || count > maxCompoundLayers {
		return nil, false
	}
	l := compoundLayerLayout2023
	layers := make([]CompoundLayer, 0, count)
	for i := 0; i < count; i++ {
		rec := at + 4 + l.recordLen*i
		if rec+l.recordLen > len(buf) {
			return nil, false
		}
		width := math.Float64frombits(binary.LittleEndian.Uint64(buf[rec:]))
		fn := binary.LittleEndian.Uint32(buf[rec+l.functionAt:])
		mat := binary.LittleEndian.Uint32(buf[rec+l.materialAt:])
		deck := binary.LittleEndian.Uint32(buf[rec+l.deckAt:])
		// A membrane has no width, and nor has a structural deck whose
		// profile is carried by the layer above it.
		widthless := fn == 100 || fn == 200
		widthOK := !math.IsNaN(width) && !math.IsInf(width, 0) && width < 10 &&
			(width >= 1e-3 || (widthless && width == 0))
		fnOK := fn <= 5 || fn == 100 || fn == 200
		deckOK := deck == 0xffffffff || declared[deck]
		if !widthOK || !fnOK || !deckOK {
			return nil, false
		}
		layer := CompoundLayer{WidthFeet: width, Function: fn}
		if mat != 0xffffffff {
			id := mat
			layer.Material = &id
		}
		layers = append(layers, layer)
	}
	return layers, true
}

// compoundLayerFramed reports whether the count at `at` is framed as a
// type's layers are: behind four 0xff and the frame tag, or behind the
// type's UTF-16LE name and a u32 0.
func compoundLayerFramed(buf []byte, at int) bool {
	if at >= 6 && buf[at-6] == 0xff && buf[at-5] == 0xff && buf[at-4] == 0xff && buf[at-3] == 0xff &&
		buf[at-2] == layerFrameTag2023[0] && buf[at-1] == layerFrameTag2023[1] {
		return true
	}
	if at < 4 {
		return false
	}
	if binary.LittleEndian.Uint32(buf[at-4:]) != 0 {
		return false
	}
	for k := 1; k <= maxNameUnits; k++ {
		start := at - 4 - 2*k - 4
		if start < 0 {
			return false
		}
		if binary.LittleEndian.Uint32(buf[start:]) != uint32(k) {
			continue
		}
		printable := true
		for i := 0; i < k; i++ {
			unit := binary.LittleEndian.Uint16(buf[start+4+2*i:])
			if unit < 0x20 {
				printable = false
				break
			}
		}
		if printable {
			return true
		}
	}
	return false
}

// ScanCompoundLayerLists returns every type layer list in the partition,
// each with its total thickness. The lists appear in no element order;
// joining one to the walls of its type needs the type-record layer
// (not decoded here — see the type_symbol_reference of the wall records).
func ScanCompoundLayerLists(p *Partition, declared map[uint32]bool, version int) [][]CompoundLayer {
	if version != 2023 {
		return nil // the per-release frame tags differ; fail closed
	}
	var out [][]CompoundLayer
	for _, ss := range p.SubStreams {
		d := ss.Data
		for at := 6; at+4+compoundLayerLayout2023.recordLen <= len(d); at++ {
			if !compoundLayerFramed(d, at) {
				continue
			}
			if layers, ok := compoundLayersAt(d, at, declared); ok {
				out = append(out, layers)
				at += 4 + compoundLayerLayout2023.recordLen*len(layers)
			}
		}
	}
	return out
}

// TypeLayerThickness scans every element-data record in the partition and
// returns the total layer thickness (feet) of each element whose data
// carries a valid compound layer list — the type elements (wall, floor,
// ceiling, roof types). A type's layers frame either behind four 0xff and
// the release's layer tag, or behind the type's UTF-16LE name.
func TypeLayerThickness(p *Partition, schema *Schema, declared map[uint32]bool, version int) map[uint32]float64 {
	if version != 2023 || schema == nil {
		return nil
	}
	hdr, ok := elementDataHeader(schema)
	if !ok {
		return nil
	}
	out := map[uint32]float64{}
	for _, ss := range p.SubStreams {
		d := ss.Data
		// element-data record starts in this substream
		type start struct {
			id   uint32
			at   int
			next int
		}
		var starts []start
		for at := 0; at+14 <= len(d); {
			idx := indexOfBytes(d, hdr[:], at)
			if idx < 0 {
				break
			}
			id := binary.LittleEndian.Uint32(d[idx+10:])
			next := len(d)
			if n := indexOfBytes(d, hdr[:], idx+1); n >= 0 {
				next = n
			}
			starts = append(starts, start{id, idx, next})
			at = idx + 1
		}
		for at := 0; at+4+compoundLayerLayout2023.recordLen <= len(d); at++ {
			if !compoundLayerFramed(d, at) {
				continue
			}
			layers, ok := compoundLayersAt(d, at, declared)
			if !ok {
				continue
			}
			total := 0.0
			for _, l := range layers {
				total += l.WidthFeet
			}
			// attribute the frame to the element-data record containing it
			i := sortSearch(len(starts), func(i int) bool { return starts[i].at > at }) - 1
			if i >= 0 && at < starts[i].next && total > 0 {
				out[starts[i].id] = total
			}
			at += 4 + compoundLayerLayout2023.recordLen*len(layers)
		}
	}
	return out
}

func sortSearch(n int, f func(int) bool) int {
	lo, hi := 0, n
	for lo < hi {
		mid := (lo + hi) / 2
		if f(mid) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

// TypeJoinByReference associates each placed instance record with its
// type element: the instance's reference list must name exactly one
// element that carries a compound layer list (measured: 47 of 56 placed
// walls on rac_basic_2023 join exactly, 9 name no layered type, 0 are
// ambiguous). Instances whose references do not resolve uniquely are
// absent.
func TypeJoinByReference(records []ElementRecord, typeThickness map[uint32]float64) map[uint32]uint32 {
	if len(typeThickness) == 0 {
		return nil
	}
	out := map[uint32]uint32{}
	for i := range records {
		r := &records[i]
		if !r.IsPlacedInstance() {
			continue
		}
		hit := uint32(0)
		n := 0
		for _, ref := range r.References {
			if ref == r.ElementID {
				continue
			}
			if _, ok := typeThickness[ref]; ok {
				hit = ref
				n++
			}
		}
		if n == 1 {
			out[r.ElementID] = hit
		}
	}
	return out
}
