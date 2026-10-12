package rvt

import (
	"encoding/binary"
	"fmt"
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
	return h, nil
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
