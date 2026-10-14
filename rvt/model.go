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
	InlineArcs  []geom.Arc
	// InlineCurves: validated GLine instances from the class-103 trees
	// (R88 EndParams-prefix model: [EndParams 2×f64][Origin 3×f64]
	// [Direction 3×f64]; 3,874 non-degenerate segments on 2023).
	InlineCurves []geom.Line
	// Tree-attributed inline geometry (exact per-element binding by G-tree
	// range — R91): inline GLines and GArcs bound to their enclosing tree's
	// element counter.
	BoundInlineCurves []geom.BoundLine
	InlineArcsBound   []geom.BoundArc
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
		var store *SubStream
		for _, ss := range p.SubStreams {
			if ss.Class == geometryStoreClass {
				store = ss
			}
		}
		gEl, hasGEl := classindex.Index(version, "OdBmGElement")
		if hasGEl {
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
			// tree-range attribution of inline geometry (R91)
			if gl, ok := classindex.Index(version, "OdBmGLine"); ok {
				m.BoundInlineCurves = ScanInlineGLinesKeyed(geo.Data, int16(gl), m.Trees)
			}
			if gArc, ok := classindex.Index(version, "OdBmGArc"); ok {
				m.InlineArcs = ScanInlineGArcs(geo.Data, int16(gArc), lookup)
				for _, ia := range ScanInlineGArcsAttributed(geo.Data, m.Trees, int16(gArc), lookup) {
					m.InlineArcsBound = append(m.InlineArcsBound, geom.BoundArc{
						ElementID: ia.ElementID,
						Arc: geom.Arc{
							XDir:       ia.XDir,
							YDir:       ia.YDir,
							Radius:     ia.Radius,
							Center:     ia.Center,
							StartAngle: ia.StartAngle,
							EndAngle:   ia.EndAngle,
						},
					})
				}
			}
			if gl, ok := classindex.Index(version, "OdBmGLine"); ok {
				m.InlineCurves = ScanInlineGLines(geo.Data, int16(gl))
			}
		}
		m.collectBReps(geo.Data, lookup, m.Trees)
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
// within a small window belong to one BRep blob's topology. When a group's
// first run falls inside a G tree's body range the skeleton is attributed
// to that tree's element counter (0 = unattributed).
func (m *Model) collectBReps(data []byte, lookup func(int16) string, trees []GTree) {
	gi := ParseGeometryIndex(data)
	runs := ScanGCatalogRuns(data, gi.DataOffset, lookup)
	// tree body ranges for attribution (sorted by Offset)
	type treeRange struct{ lo, hi int; id uint32 }
	ranges := make([]treeRange, 0, len(trees))
	for _, tr := range trees {
		if tr.End > tr.Offset {
			ranges = append(ranges, treeRange{tr.Offset, tr.End, tr.Counter})
		}
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].lo < ranges[j].lo })
	treeFor := func(off int) uint32 {
		i := sort.Search(len(ranges), func(i int) bool { return ranges[i].lo > off }) - 1
		if i >= 0 && off < ranges[i].hi {
			return ranges[i].id
		}
		return 0
	}
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
			cur = &geom.BRep{ElementID: treeFor(r.Offset)}
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

// GeometryManifest assembles the per-element decoded geometry — the unit
// the go-topo bridge consumes. An entry exists for every element with
// geometry evidence: a GElement tree (HasTree), tree- or store-attributed
// curves/arcs, or an attributed BRep skeleton. Elements without any
// evidence are absent (metadata-only: datums, parameters, tables, …).
// Tree counters that do not identify a known element are dropped.
func (m *Model) GeometryManifest() map[uint32]*geom.ElementGeometry {
	known := make(map[uint32]bool, len(m.Elements))
	for i := range m.Elements {
		known[m.Elements[i].ID] = true
	}
	out := map[uint32]*geom.ElementGeometry{}
	get := func(id uint32) *geom.ElementGeometry {
		if !known[id] {
			return nil
		}
		g := out[id]
		if g == nil {
			g = &geom.ElementGeometry{ElementID: id}
			out[id] = g
		}
		return g
	}
	for _, tr := range m.Trees {
		if g := get(tr.Counter); g != nil {
			g.HasTree = true
			if g.BBox == nil && len(tr.BBoxes) > 0 {
				bb := tr.BBoxes[0]
				g.BBox = &bb
			}
		}
	}
	for _, bl := range m.BoundInlineCurves {
		if g := get(bl.ElementID); g != nil {
			g.Lines = append(g.Lines, bl.Line)
		}
	}
	for _, ba := range m.InlineArcsBound {
		if g := get(ba.ElementID); g != nil {
			g.Arcs = append(g.Arcs, ba.Arc)
		}
	}
	for _, bl := range m.BoundCurves {
		if g := get(bl.ElementID); g != nil {
			g.Lines = append(g.Lines, bl.Line)
		}
	}
	for _, ba := range m.BoundArcs {
		if g := get(ba.ElementID); g != nil {
			g.Arcs = append(g.Arcs, ba.Arc)
		}
	}
	for i := range m.BReps {
		if b := &m.BReps[i]; b.ElementID != 0 {
			if g := get(b.ElementID); g != nil && g.BRep == nil {
				cp := *b
				g.BRep = &cp
			}
		}
	}
	return out
}
