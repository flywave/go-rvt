package rvt

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// OdBmElementHeader static schema (REA round 5 — TB_Format2023Readers
// CustomDirectReader<1398>::read @0xe8914a, validated byte-aligned on the
// 2023/2022 samples; findings R49/R50):
//
//	[i16 class index]                1398 (2023) / 1340 (2022)
//	[i32 count]                      ElementRegenHistory map size
//	[count × entry]                  entry = [i64][ElementId][i16][i32]
//	[ElementId ×6]                   Category, Family, OwnerView, DesignOption,
//	                                 UnplacedOwner, Misc — i32 each (null = -1)
//	[i32]                            RejectedViewRules.visibleViewFlags bitmask
//	[i32]                            AbFlags4Bytes bitfield
//	[i16 class][i32][i32]            ClassDefinitionRef — the element's actual
//	                                 class index (resolves via classindex) + 2 refs
//	[CondInt16 ×2]                   [i32 v][i16] = 6B each (all branches)
//	[dynamic properties]             readDynamicProperties token section
//	                                 (bit-packed; bounded unknown — R52)
//
// Fast-path nested objects carry no i16 class prefix (types are known from
// the schema); the ClassDefinitionRef i16 is data (a class reference), not
// an envelope prefix.

// elementHeaderFixedBytes is the guaranteed size through ClassDefinitionRef.
const elementHeaderFixedBytes = 48

// ElementHeader is the decoded static head of one OdBmElementHeader record.
type ElementHeader struct {
	HistoryCount   int32  `json:"history_count"`
	CategoryID     int32  `json:"category_id"`
	FamilyID       int32  `json:"family_id"`
	OwnerViewID    int32  `json:"owner_view_id"`
	DesignOptionID int32  `json:"design_option_id"`
	UnplacedOwner  int32  `json:"unplaced_owner_id"`
	MiscID         int32  `json:"misc_id"`
	ViewRulesFlags uint32 `json:"view_rules_flags"`
	AbFlags        uint32 `json:"ab_flags"`
	ClassIndex     int16  `json:"class_index"`
	ClassName      string `json:"class_name,omitempty"`
	ClassRefA      int32  `json:"class_ref_a"`
	ClassRefB      int32  `json:"class_ref_b"`

	HeadBytes    int `json:"head_bytes"`    // static bytes consumed (incl. CondInt16s)
	DynamicBytes int `json:"dynamic_bytes"` // undecoded token-section size

	// Dynamic section (findings R63/R64): after the 60B static head the
	// body carries n 4-byte queued values followed by a 3-byte zero pad —
	// n = (size-63)/4. ~98% (2023) / ~94% (2022) of records fit this pure
	// 4B framing (TailScalars). The rest carry per-class extended data
	// (R64): element-id sequences (DBViewPlan view members), f64 coordinate
	// arrays (FamilySymbol), etc — classified into TailKind.
	DynamicValues []uint32 `json:"dynamic_values,omitempty"` // raw 4B values (int or float bits)
	TailKind      string   `json:"tail_kind,omitempty"`      // scalars | ids | floats64 | mixed
	ExtendedTail  bool     `json:"extended_tail,omitempty"`  // tail breaks the pure-4B framing
}

// DecodeElementHeader decodes the static head of an OdBmElementHeader body
// (as found in ObjectRecord.Body). lookup resolves the element class name.
// It never fails on well-formed sizes; short or malformed bodies return an
// error and leave the header partially filled.
func DecodeElementHeader(body []byte, lookup func(int16) string) (*ElementHeader, error) {
	if len(body) < elementHeaderFixedBytes {
		return nil, fmt.Errorf("rvt: elementheader: body %d < %d", len(body), elementHeaderFixedBytes)
	}
	h := &ElementHeader{}
	pos := 0
	rd16 := func() int16 { v := int16(binary.LittleEndian.Uint16(body[pos:])); pos += 2; return v }
	rd32 := func() int32 { v := int32(binary.LittleEndian.Uint32(body[pos:])); pos += 4; return v }

	h.ClassIndex = rd16()
	if lookup != nil {
		h.ClassName = lookup(h.ClassIndex)
	}
	h.HistoryCount = rd32()
	if h.HistoryCount < 0 || pos+int(h.HistoryCount)*22 > len(body) {
		return h, fmt.Errorf("rvt: elementheader: bad history count %d", h.HistoryCount)
	}
	for k := int32(0); k < h.HistoryCount; k++ {
		pos += 8 // otherInt64
		rd32()   // otherElementId
		pos += 2 // atomType
		rd32()   // pair value (int)
	}
	h.CategoryID = rd32()
	h.FamilyID = rd32()
	h.OwnerViewID = rd32()
	h.DesignOptionID = rd32()
	h.UnplacedOwner = rd32()
	h.MiscID = rd32()
	h.ViewRulesFlags = uint32(rd32())
	h.AbFlags = uint32(rd32())
	h.ClassIndex = rd16() // ClassDefinitionRef — the element's real class
	if lookup != nil {
		h.ClassName = lookup(h.ClassIndex)
	}
	h.ClassRefA = rd32()
	h.ClassRefB = rd32()
	// Two OdBmCondInt16 properties: [i32 v][i16] — every branch of the
	// reader consumes exactly 6 bytes (REA round 8: the v!=0 path also reads
	// a single i16 before returning; the earlier 8B model was wrong and
	// misaligned the dynamic section).
	for k := 0; k < 2; k++ {
		if pos+6 > len(body) {
			break
		}
		rd32()
		rd16()
	}
	h.HeadBytes = pos
	h.DynamicBytes = len(body) - pos
	// Dynamic section (R63/R64)
	tail := body[h.HeadBytes : len(body)-3]
	if len(tail) >= 0 && len(body) >= h.HeadBytes+3 && (len(body)-h.HeadBytes-3)%4 == 0 {
		n := len(tail) / 4
		h.DynamicValues = make([]uint32, n)
		for k := 0; k < n; k++ {
			h.DynamicValues[k] = binary.LittleEndian.Uint32(tail[k*4:])
		}
		h.TailKind = classifyTailValues(h.DynamicValues)
	} else {
		h.ExtendedTail = true
		h.TailKind = ClassifyExtendedTail(body[h.HeadBytes:])
	}
	return h, nil
}

// TailKind values.
const (
	TailScalars  = "scalars"  // uniform 4B values (queued defaults/params)
	TailIDRun    = "ids"      // element-id sequence (e.g. DBViewPlan members)
	TailFloat32s = "floats32" // f32 parameter array
	TailFloat64s = "floats64" // f64 coordinate/parameter array
	TailMixed    = "mixed"
)

// maxElementID bounds id-run detection (2023 ElemTable ids reach 1098316).
const maxElementID = 1 << 24

// f32Plausible: zero or a sane magnitude (style/param floats, not pointers).
func f32Plausible(v uint32) bool {
	f := math.Float32frombits(v)
	return f == 0 || (math.Abs(float64(f)) < 1e6 && math.Abs(float64(f)) > 1e-9)
}

func f64Plausible(v uint64) bool {
	f := math.Float64frombits(v)
	return f == 0 || (math.Abs(f) < 1e6 && math.Abs(f) > 1e-9)
}

// classifyTailValues names a pure-4B dynamic section by value fractions.
func classifyTailValues(vals []uint32) string {
	if len(vals) == 0 {
		return TailScalars
	}
	ids, f32s := 0, 0
	for _, v := range vals {
		if v >= 1 && v <= maxElementID {
			ids++
		}
		if f32Plausible(v) {
			f32s++
		}
	}
	n := len(vals)
	switch {
	case ids >= n*4/5:
		return TailIDRun
	case f32s >= n*4/5:
		return TailFloat32s
	default:
		return TailScalars
	}
}

// ClassifyExtendedTail heuristically names non-4B-uniform tails (R64):
// element-id runs (may start 2 bytes in) vs f64 coordinate arrays vs
// unknown mixtures.
func ClassifyExtendedTail(tail []byte) string {
	n4 := len(tail) / 4
	if n4 == 0 {
		return TailScalars
	}
	// id runs may start 2 bytes in (short prefix before the id list)
	idFrac := 0.0
	for _, start := range []int{0, 2} {
		ids, samples := 0, 0
		for k := start; k+4 <= len(tail); k += 4 {
			v := binary.LittleEndian.Uint32(tail[k:])
			samples++
			if v >= 1 && v <= maxElementID {
				ids++
			}
		}
		if samples > 0 {
			if fr := float64(ids) / float64(samples); fr > idFrac {
				idFrac = fr
			}
		}
	}
	// f64 fraction: try 4-byte start offsets, keep the best
	best := 0.0
	for start := 0; start < 4 && start < len(tail); start++ {
		ok, samples := 0, 0
		for k := start; k+8 <= len(tail); k += 8 {
			v := binary.LittleEndian.Uint64(tail[k:])
			samples++
			if f64Plausible(v) {
				ok++
			}
		}
		if samples > 0 {
			if fr := float64(ok) / float64(samples); fr > best {
				best = fr
			}
		}
	}
	switch {
	case idFrac >= 0.8:
		return TailIDRun
	case best >= 0.8:
		return TailFloat64s
	default:
		return TailMixed
	}
}

// ElementClassCensus decodes every OdBmElementHeader record of a sub-stream
// and histograms the element classes (ClassDefinitionRef names).
func ElementClassCensus(ss *SubStream, lookup func(int16) string) []ClassCount {
	counts := map[string]int{}
	for i := range ss.Records {
		r := &ss.Records[i]
		if r.ClassIndex != elementHeaderClassIndex(ss) {
			continue
		}
		h, err := DecodeElementHeader(r.Body, lookup)
		if err != nil {
			continue
		}
		if h.ClassName == "" {
			continue
		}
		counts[h.ClassName]++
	}
	return topCounts(counts)
}

// elementHeaderClassIndex returns the record class index of ElementHeader
// records in this sub-stream (the majority class), or 0 when unknown.
func elementHeaderClassIndex(ss *SubStream) int16 {
	if len(ss.Records) == 0 {
		return 0
	}
	return ss.Records[0].ClassIndex
}

func topCounts(counts map[string]int) []ClassCount {
	out := make([]ClassCount, 0, len(counts))
	for name, n := range counts {
		out = append(out, ClassCount{Name: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}
