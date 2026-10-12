package rvt

// Model assembly: extracts the populated geom objects (Element/View/BRep)
// from a parsed partition. See rvt/geom for the type contract.

import (
	"encoding/binary"
	"sort"

	"github.com/flywave/go-rvt/rvt/classindex"
	"github.com/flywave/go-rvt/rvt/geom"
)

// Model is the assembled object model of one partition.
type Model struct {
	Elements []geom.Element
	Views    []geom.View
	BReps    []geom.BRep
	Trees    []geom.GTree
	Curves   []geom.Line
	Arcs     []geom.Arc
	// Bound geometry: instances attributed to elements via the
	// element-counter keyed block contexts (R72/R77).
	BoundCurves []geom.BoundLine
	BoundArcs   []geom.BoundArc
	InlineArcs  []InlineArc
	// InlineCurves: validated GLine instances from the class-103 trees
	// (R88 EndParams-prefix model: [EndParams 2×f64][Origin 3×f64]
	// [Direction 3×f64]; 3,874 non-degenerate segments on 2023).
	InlineCurves []geom.Line
}

// geometrySubStreamClass is the class-103 page tag (both samples).
const geometrySubStreamClass = 103

// geometryStoreClass is the class-102 page tag (G node object store).
const geometryStoreClass = 102

// Model assembles the populated geometry object model from this partition's
// sub-streams: Elements from the deposit (class-101) ElementHeader records,
// Views from DBViewPlan extended tails, BRep topology skeletons from the
// class-103 G catalog. version selects the class-index table (e.g. "2023").
func (p *Partition) Model(version string) *Model {
	lookup := func(cls int16) string {
		name, _ := classindex.Lookup(version, cls)
		return name
	}
	m := &Model{}
	var geo *SubStream
	for _, ss := range p.SubStreams {
		if len(ss.Deposits) > 0 || len(ss.Records) > 0 {
			m.collectElements(ss, lookup)
			m.collectViews(ss, lookup, version)
		}
		if ss.Class == geometrySubStreamClass {
			geo = ss
		}
	}
	if geo != nil {
		m.collectBReps(geo.Data, lookup)
		if gl, ok := classindex.Index(version, "OdBmGLine"); ok {
			m.InlineCurves = ScanInlineGLines(geo.Data, int16(gl))
		}
		var store *SubStream
		for _, ss := range p.SubStreams {
			if ss.Class == geometryStoreClass {
				store = ss
			}
		}
		if geo != nil {
			gArc, ok2 := classindex.Index(version, "OdBmGArc")
			if ok2 {
				m.InlineArcs = ScanInlineGArcs(geo.Data, int16(gArc), m.Trees, lookup)
			}
		}
		if store != nil {
			// element-counter key positions for attribution (R77)
			keySet := map[uint32]bool{}
			for _, ss := range p.SubStreams {
				if len(ss.Deposits) > 0 {
					for i := range ss.Records {
						keySet[ss.Records[i].Counter] = true
					}
				}
			}
			keys := KeyPositions(store.Data, keySet)
			if gl, ok := classindex.Index(version, "OdBmGLine"); ok {
				m.Curves = ScanGLines(store.Data, int16(gl))
				m.BoundCurves = ScanGLinesKeyed(store.Data, int16(gl), keys)
			}
			m.Arcs = ScanGArcs(store.Data)
			m.BoundArcs = ScanGArcsKeyed(store.Data, keys)
		}
		gEl, ok := classindex.Index(version, "OdBmGElement")
		if ok {
			m.Trees = ScanGTrees(geo.Data, func() int {
				gi := ParseGeometryIndex(geo.Data)
				return gi.DataOffset
			}(), int16(gEl), lookup)
			// attach bboxes to elements by deposit counter
			byCounter := map[uint32]*geom.Element{}
			for i := range m.Elements {
				byCounter[m.Elements[i].ID] = &m.Elements[i]
			}
			for _, tr := range m.Trees {
				if len(tr.BBoxes) == 0 {
					continue
				}
				if e := byCounter[tr.Counter]; e != nil && e.BBox == nil {
					bb := tr.BBoxes[0]
					e.BBox = &bb
				}
			}
		}
	}
	sort.Slice(m.Elements, func(i, j int) bool { return m.Elements[i].ID < m.Elements[j].ID })
	sort.Slice(m.Views, func(i, j int) bool { return m.Views[i].ID < m.Views[j].ID })
	return m
}

func (m *Model) collectElements(ss *SubStream, lookup func(int16) string) {
	for i := range ss.Records {
		h, err := DecodeElementHeader(ss.Records[i].Body, lookup)
		if err != nil {
			continue
		}
		m.Elements = append(m.Elements, geom.Element{
			ID:             ss.Records[i].Counter,
			ClassIndex:     h.ClassIndex,
			ClassName:      h.ClassName,
			CategoryID:     h.CategoryID,
			FamilyID:       h.FamilyID,
			OwnerViewID:    h.OwnerViewID,
			DesignOptionID: h.DesignOptionID,
			UnplacedOwner:  h.UnplacedOwner,
			MiscID:         h.MiscID,
			ViewRulesFlags: h.ViewRulesFlags,
			ABFlags:        h.AbFlags,
		})
	}
}

// collectViews extracts DBViewPlan element-membership runs from extended
// tails (findings R64: [2B prefix][element ids…]).
func (m *Model) collectViews(ss *SubStream, lookup func(int16) string, version string) {
	dbp, ok := classindex.Index(version, "OdBmDBViewPlan")
	if !ok {
		return
	}
	for i := range ss.Records {
		body := ss.Records[i].Body
		if len(body) < 64 {
			continue
		}
		if int16(binary.LittleEndian.Uint16(body[38:])) != dbp {
			continue
		}
		h, err := DecodeElementHeader(body, lookup)
		if err != nil || h.TailKind != TailIDRun {
			continue
		}
		v := geom.View{ID: ss.Records[i].Counter}
		tail := body[h.HeadBytes:]
		for k := 2; k+4 <= len(tail)-3; k += 4 {
			id := binary.LittleEndian.Uint32(tail[k:])
			if id >= 1 && id <= maxElementID {
				v.Members = append(v.Members, id)
			}
		}
		if len(v.Members) > 0 {
			m.Views = append(m.Views, v)
		}
	}
}

// collectBReps groups Face/Edge catalog runs of the class-103 G region into
// topology skeletons (findings R56): consecutive OdBmFace/OdBmEdge runs
// within a small window belong to one BRep blob's topology.
func (m *Model) collectBReps(data []byte, lookup func(int16) string) {
	gi := ParseGeometryIndex(data)
	runs := ScanGCatalogRuns(data, gi.DataOffset, lookup)
	const brepGap = 1024 // max bytes between runs of one BRep group
	var cur *geom.BRep
	flush := func() {
		if cur != nil && (len(cur.FaceIDs) > 0 || len(cur.EdgeIDs) > 0) {
			m.BReps = append(m.BReps, *cur)
		}
		cur = nil
	}
	prevEnd := -1
	for _, r := range runs {
		isFace := r.ClassName == "OdBmFace"
		isEdge := r.ClassName == "OdBmEdge"
		if !isFace && !isEdge {
			flush()
			prevEnd = -1
			continue
		}
		if cur != nil && prevEnd >= 0 && r.Offset-prevEnd > brepGap {
			flush()
		}
		if cur == nil {
			cur = &geom.BRep{}
		}
		if isFace {
			cur.FaceIDs = append(cur.FaceIDs, idRange(r)...)
		} else {
			cur.EdgeIDs = append(cur.EdgeIDs, idRange(r)...)
		}
		prevEnd = r.Offset + r.Entries*6
	}
	flush()
}

func idRange(r GCatalogRun) []uint32 {
	out := make([]uint32, 0, r.Entries)
	for id := int64(r.FirstID); id <= int64(r.LastID) && len(out) < r.Entries; id++ {
		out = append(out, uint32(id))
	}
	return out
}
