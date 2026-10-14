package rvt

// Element-data names: the framed UTF-16LE name an element's serialised
// data carries behind its element-data header (RE-63/RE-111; grammar per
// the rvt-rs reference implementation — partition_names.rs).
//
// A name frame is four 0xff, a u16 field tag (never 0xffff — that slots
// another element's data header, which frames an ElementId, not a name),
// a u32 length in UTF-16 code units and the name itself. System-family
// types (walls, floors, roofs, ceilings, railings) have no separate name
// entry, so their data opening is where the type name lives — the join
// from an instance to its type's data then names the instance's type
// ("Basic Wall: Generic - 8""; measured 2023: family and type names
// 50/50, system-family types 24/24, RE-109/RE-111/RE-63).

import (
	"encoding/binary"
	"math"
	"sort"
	"strings"
	"unicode/utf16"
)

// nameMaxUnits bounds a framed name's UTF-16 length.
const nameMaxUnits = 256

// Names are searched across the element's whole data window (bounded by
// the next element-data header): system-family types frame their name
// shallowly (<0x600), but FamilySymbol data can carry it past 3 KB
// (measured: a window symbol's name at depth 3484, R112).

// firstFramedName reads the first framed name at or after `start`
// (exclusive of the record's own data header at `hdrStart`, when >= 0).
func firstFramedName(d []byte, start, hdrStart, end int, hdr [10]byte) (string, bool) {
	if start < 0 {
		return "", false
	}
	stop := end
	for at := start; at+10 <= stop; at++ {
		if d[at] != 0xff || d[at+1] != 0xff || d[at+2] != 0xff || d[at+3] != 0xff {
			continue
		}
		tag := binary.LittleEndian.Uint16(d[at+4:])
		if tag == 0xffff {
			continue
		}
		if hdrStart >= 0 && at >= hdrStart && at+10 <= hdrStart+10 && bytesEqual8(d[hdrStart:hdrStart+8], hdr[:]) {
			continue
		}
		units := int(binary.LittleEndian.Uint32(d[at+6:]))
		if units < 1 || units > nameMaxUnits || at+10+units*2 > stop {
			continue
		}
		if at+10+units*2 > len(d) {
			continue
		}
		raw := d[at+10 : at+10+units*2]
		units16 := make([]uint16, units)
		for i := 0; i < units; i++ {
			units16[i] = binary.LittleEndian.Uint16(raw[i*2:])
		}
		name := string(utf16.Decode(units16))
		if strings.TrimSpace(name) == "" {
			continue
		}
		// Quality gate: a framed name is printable text — no C0/C1
		// controls, no replacement sentinels, at least one ASCII
		// letter/digit (real Revit names all carry one; a frame that
		// fails this is field data misread as a name).
		bad := false
		alnum := false
		for _, c := range name {
			if c == 0xfffd || c == 0xfffe || c == 0xffff || c < 0x20 || (c >= 0x7f && c <= 0x9f) {
				bad = true
				break
			}
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
				alnum = true
			}
		}
		if bad || !alnum || len([]rune(name)) < 3 {
			continue
		}
		return name, true
	}
	return "", false
}

// ScanElementDataNames returns the framed name of each element in `ids`,
// from its element-data records. An id whose occurrences disagree is
// dropped. Only releases with a measured element-data layout yield
// results (2023-2026; fail closed).
func ScanElementDataNames(p *Partition, schema *Schema, ids map[uint32]bool, version int) map[uint32]string {
	if schema == nil || version < 2023 || version > 2026 || len(ids) == 0 {
		return nil
	}
	hdr, ok := elementDataHeader(schema)
	if !ok {
		return nil
	}
	wideID := version >= 2024
	found := map[uint32]string{}
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
			nameStart := idx + 18
			end := next
			if nameStart >= end {
				continue
			}
			if name, ok := firstFramedName(d, nameStart, idx, end, hdr); ok {
				if held, seen := found[id]; seen && held != name {
					delete(found, id)
				} else if !seen {
					found[id] = name
				}
			}
		}
	}
	return found
}

// SymbolTypeJoin associates each placed instance record with its family
// symbol (the instance's type — the FamilySymbol element a door's
// "Single-Flush 0915 x 2134mm" type is). Candidates are the same-category
// symbol records in the instance's reference list; when that leaves zero
// or several candidates the instance's element data decides: the type id
// is the one framed behind four 0xff ([ffffffff][id] — the
// ElementId-reference framing; other references to sibling symbols sit
// behind different bytes). Instances that resolve to exactly one symbol
// are returned; zero/ambiguous ones are absent.
//
// Measured on rac_basic_2023: 378 instances join through the reference
// list alone (furniture, railings, curtain panels, …); doors and windows
// join through the data framing.
func SymbolTypeJoin(records []ElementRecord, p *Partition, schema *Schema, version int) map[uint32]uint32 {
	if version < 2023 || version > 2026 || schema == nil {
		return nil
	}
	symbolCat := map[uint32]int64{}
	cats := map[int64]map[uint32]bool{}
	for i := range records {
		r := &records[i]
		if r.IsTypeSymbol() {
			symbolCat[r.ElementID] = r.Category
			if cats[r.Category] == nil {
				cats[r.Category] = map[uint32]bool{}
			}
			cats[r.Category][r.ElementID] = true
		}
	}
	if len(symbolCat) == 0 {
		return nil
	}
	hdr, ok := elementDataHeader(schema)
	if !ok {
		return nil
	}
	// pass 1: reference-list candidates
	out := map[uint32]uint32{}
	var deferred []uint32
	for i := range records {
		r := &records[i]
		if !r.IsPlacedInstance() {
			continue
		}
		catSyms := cats[r.Category]
		if catSyms == nil {
			continue
		}
		hit := uint32(0)
		n := 0
		for _, ref := range r.References {
			if ref != r.ElementID && catSyms[ref] {
				hit, n = ref, n+1
			}
		}
		switch n {
		case 1:
			out[r.ElementID] = hit
		default:
			deferred = append(deferred, r.ElementID)
		}
	}
	if len(deferred) == 0 {
		return out
	}
	// pass 2: data-framing disambiguation for the deferred instances
	want := make(map[uint32]bool, len(deferred))
	for _, id := range deferred {
		want[id] = true
	}
	var vb [4]byte
	for _, ss := range p.SubStreams {
		d := ss.Data
		for at := 0; at+18 <= len(d); {
			idx := indexOfBytes(d, hdr[:], at)
			if idx < 0 {
				break
			}
			id := binary.LittleEndian.Uint32(d[idx+10:])
			next := len(d)
			if n := indexOfBytes(d, hdr[:], idx+1); n >= 0 {
				next = n
			}
			at = next
			if !want[id] {
				continue
			}
			end := next
			if end > idx+18+0x10000 {
				end = idx + 18 + 0x10000
			}
			if idx+18 >= end {
				continue
			}
			window := d[idx+18 : end]
			catSyms := cats[catOf(records, id)]
			// candidates: same-category symbol ids framed behind 0xff×4
			hit := uint32(0)
			n := 0
			for sid := range catSyms {
				binary.LittleEndian.PutUint32(vb[:], sid)
				for k := 0; k+8 <= len(window); k++ {
					if window[k] == 0xff && window[k+1] == 0xff && window[k+2] == 0xff && window[k+3] == 0xff &&
						window[k+4] == vb[0] && window[k+5] == vb[1] && window[k+6] == vb[2] && window[k+7] == vb[3] {
						hit, n = sid, n+1
						break
					}
				}
			}
			if n == 1 {
				out[id] = hit
			}
		}
	}
	return out
}

// catOf returns the record's category for id (0 when absent).
func catOf(records []ElementRecord, id uint32) int64 {
	for i := range records {
		if records[i].ElementID == id {
			return records[i].Category
		}
	}
	return 0
}

// VoteInstanceDims recovers type dimensions (width, height) for family
// symbols from the element data of their placed instances: an f64
// constant that appears in nearly every instance of one symbol is a
// type-level parameter (a window type's width/height are baked into
// every instance's data). The two most-agreed values become (w, h) with
// w <= h. Symbols with fewer than three instances are skipped (no vote).
//
// Measured on rac_basic_2023: the "Standard" window symbol (17 instances)
// votes 4.921260 ft (1500 mm) and 8.858268 ft (2700 mm) at 15/17 each;
// 1.000000 ft (304.8 mm) is a third common constant (frame).
func VoteInstanceDims(p *Partition, schema *Schema, symbolJoin map[uint32]uint32, version int) map[uint32][2]float64 {
	if version < 2023 || version > 2026 || schema == nil || len(symbolJoin) == 0 {
		return nil
	}
	hdr, ok := elementDataHeader(schema)
	if !ok {
		return nil
	}
	// instances grouped by symbol
	bySym := map[uint32][]uint32{}
	for inst, sym := range symbolJoin {
		bySym[sym] = append(bySym[sym], inst)
	}
	out := map[uint32][2]float64{}
	for sym, insts := range bySym {
		if len(insts) < 3 {
			continue
		}
		want := map[uint32]bool{}
		for _, id := range insts {
			want[id] = true
		}
		// per-instance candidate sets, then vote
		vote := map[float64]int{}
		for _, ss := range p.SubStreams {
			d := ss.Data
			for at := 0; at+18 <= len(d); {
				idx := indexOfBytes(d, hdr[:], at)
				if idx < 0 {
					break
				}
				id := binary.LittleEndian.Uint32(d[idx+10:])
				next := len(d)
				if n := indexOfBytes(d, hdr[:], idx+1); n >= 0 {
					next = n
				}
				at = next
				if !want[id] {
					continue
				}
				end := next
				if end > idx+18+0x1000 {
					end = idx + 18 + 0x1000
				}
				if idx+18 >= end {
					continue
				}
				seen := map[float64]bool{}
				window := d[idx+18 : end]
				for k := 0; k+8 <= len(window); k++ {
					v := math.Float64frombits(binary.LittleEndian.Uint64(window[k:]))
					// skip the 1-ft default constant (304.8 mm appears in
					// nearly every element and starves the vote)
					if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0.05 || v > 100 || v == 1.0 || seen[v] {
						continue
					}
					seen[v] = true
					vote[v]++
				}
			}
		}
		// top values agreed by >= n-2 instances
		type kv struct {
			v float64
			n int
		}
		var common []kv
		need := len(insts) - 2
		for v, n := range vote {
			if n >= need {
				common = append(common, kv{v, n})
			}
		}
		if len(common) < 2 {
			continue
		}
		sort.Slice(common, func(i, j int) bool { return common[i].v < common[j].v })
		// the two most-voted values (largest n first), w <= h
		sort.SliceStable(common, func(i, j int) bool { return common[i].n > common[j].n })
		w, h := common[0].v, common[1].v
		if w > h {
			w, h = h, w
		}
		out[sym] = [2]float64{w, h}
	}
	return out
}

// ScanFirstDimPair scans one symbol's element data for the first
// plausible (height, width) f64 pair: height 4-12 ft, |width| 0.8-6 ft,
// width < height, the two values within 64 bytes of each other (the
// layout observed on the name-verified door symbol). A pair found this
// way is a heuristic — without the type name there is no ground truth;
// callers should treat it as best-effort (R121: only 1 of 15 door
// symbols carries this layout).
func ScanFirstDimPair(p *Partition, schema *Schema, symbolID uint32, version int) (float64, float64, bool) {
	if version < 2023 || version > 2026 || schema == nil {
		return 0, 0, false
	}
	hdr, ok := elementDataHeader(schema)
	if !ok {
		return 0, 0, false
	}
	for _, ss := range p.SubStreams {
		d := ss.Data
		for at := 0; at+18 <= len(d); {
			idx := indexOfBytes(d, hdr[:], at)
			if idx < 0 {
				break
			}
			id := binary.LittleEndian.Uint32(d[idx+10:])
			next := len(d)
			if n := indexOfBytes(d, hdr[:], idx+1); n >= 0 {
				next = n
			}
			at = next
			if id != symbolID {
				continue
			}
			end := next
			if end > idx+18+0x10000 {
				end = idx + 18 + 0x10000
			}
			if idx+18 >= end {
				continue
			}
			window := d[idx+18 : end]
			for k := 0; k+8 <= len(window); k++ {
				h := math.Float64frombits(binary.LittleEndian.Uint64(window[k:]))
				if h < 4 || h > 12 {
					continue
				}
				for dd := 8; dd <= 64 && k+dd+8 <= len(window); dd += 8 {
					wRaw := math.Float64frombits(binary.LittleEndian.Uint64(window[k+dd:]))
					if math.IsNaN(wRaw) || math.IsInf(wRaw, 0) {
						continue
					}
					w := math.Abs(wRaw)
					if w < 0.8 || w > 6 || w >= h {
						continue
					}
					return w, h, true
				}
			}
			return 0, 0, false
		}
	}
	return 0, 0, false
}
