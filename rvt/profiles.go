package rvt

import "math"

// Plan-profile reconstruction from sketch-line element records (#31,
// RE-25; ported from the rvt-rs reference implementation,
// element_record_plan_profiles.rs, and cross-validated on
// rac_basic_2023).
//
// A Revit floor/roof/ceiling is a sketch: boundary curves closing one
// outer loop and zero or more voids. Each boundary curve is its own
// OST_SKETCH_LINES element record carrying the segment's bounding box
// and the sketched element's id in its owner reference. A record's box
// is the segment for axis-parallel lines; a diagonal leaves two
// candidate endpoint pairs, resolved by the loop itself:
//
//  1. segments degenerate on exactly one axis contribute endpoints
//     outright;
//  2. every other segment is placed only when exactly ONE pair of open
//     vertices fits its box (corner pairs first, then a lengthwise
//     spanning pair);
//  3. placement repeats until nothing is left. A segment that never
//     has exactly one candidate, a vertex that does not end at degree
//     2, an unused edge, or a loop shorter than three vertices reject
//     the whole element — the solver returns nil rather than a guess.
//
// Collinear vertices are merged afterwards (Revit splits straight runs
// into several sketch lines). Loops are ordered by absolute area,
// largest first; even containment depth starts a piece, odd depth is a
// void of its container.

// vertexEpsFeet matches recorded plan coordinates carrying float dust.
const vertexEpsFeet = 1e-6

// collinearEps is the relative cross-product below which three vertices
// are collinear.
const collinearEps = 1e-9

// PlanPiece is one separate loop of a sketch: outer boundary plus its
// voids.
type PlanPiece struct {
	OuterXY []Point2 `json:"outer"`
	InnerXY [][]Point2 `json:"inner,omitempty"`
}

// Point2 is a plan coordinate in feet.
type Point2 struct {
	X, Y float64
}

// PlanProfile is a recovered plan profile: the first (largest) piece
// promoted, plus any further pieces.
type PlanProfile struct {
	OuterXY []Point2    `json:"outer"` // counter-clockwise, no closing vertex
	InnerXY [][]Point2  `json:"inner,omitempty"` // clockwise voids, largest first
	Pieces  []PlanPiece `json:"pieces,omitempty"`
}

// planVertexRegistry interns vertices with tolerance matching.
type planVertexRegistry struct {
	points []Point2
	degree []int
}

func sameVertex(a, b Point2) bool {
	return math64Abs(a.X-b.X) <= vertexEpsFeet && math64Abs(a.Y-b.Y) <= vertexEpsFeet
}

func (v *planVertexRegistry) intern(p Point2) int {
	for i, held := range v.points {
		if sameVertex(held, p) {
			return i
		}
	}
	v.points = append(v.points, p)
	v.degree = append(v.degree, 0)
	return len(v.points) - 1
}

func math64Abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// PlanProfileFromSegments reconstructs one plan profile from segments
// given as bounding boxes [x0, y0, x1, y1]. nil when the sketch does
// not close unambiguously.
func PlanProfileFromSegments(segments [][4]float64) *PlanProfile {
	loops := solveSketchLoops(segments)
	if loops == nil {
		return nil
	}
	return profileFromLoops(loops)
}

func solveSketchLoops(segments [][4]float64) [][]Point2 {
	if len(segments) < 3 {
		return nil
	}
	var vertices planVertexRegistry
	var edges [][2]int
	var pending [][4]float64
	for _, s := range segments {
		x0, y0, x1, y1 := s[0], s[1], s[2], s[3]
		if !isFiniteRec(x0) || !isFiniteRec(y0) || !isFiniteRec(x1) || !isFiniteRec(y1) {
			return nil
		}
		flatX := math64Abs(x1-x0) <= vertexEpsFeet
		flatY := math64Abs(y1-y0) <= vertexEpsFeet
		switch {
		case flatX && flatY:
			return nil // no extent at all: not a boundary line
		case flatX:
			if !pushSketchEdge(&vertices, &edges, Point2{x0, y0}, Point2{x0, y1}) {
				return nil
			}
		case flatY:
			if !pushSketchEdge(&vertices, &edges, Point2{x0, y0}, Point2{x1, y0}) {
				return nil
			}
		default:
			pending = append(pending, s)
		}
	}
	for len(pending) > 0 {
		placed := -1
		var pa, pb Point2
		for i, span := range pending {
			if a, b, ok := soleSketchCandidate(&vertices, edges, span); ok {
				placed, pa, pb = i, a, b
				break
			}
		}
		if placed < 0 {
			return nil
		}
		pending = append(pending[:placed], pending[placed+1:]...)
		if !pushSketchEdge(&vertices, &edges, pa, pb) {
			return nil
		}
	}
	for _, d := range vertices.degree {
		if d != 2 {
			return nil
		}
	}
	return traceSketchLoops(&vertices, edges)
}

func pushSketchEdge(vertices *planVertexRegistry, edges *[][2]int, a, b Point2) bool {
	ia := vertices.intern(a)
	ib := vertices.intern(b)
	if ia == ib {
		return false
	}
	for _, e := range *edges {
		if (e[0] == ia && e[1] == ib) || (e[0] == ib && e[1] == ia) {
			return false
		}
	}
	vertices.degree[ia]++
	vertices.degree[ib]++
	if vertices.degree[ia] > 2 || vertices.degree[ib] > 2 {
		return false
	}
	*edges = append(*edges, [2]int{ia, ib})
	return true
}

// soleSketchCandidate returns the one open vertex pair that fits span.
func soleSketchCandidate(vertices *planVertexRegistry, edges [][2]int, span [4]float64) (Point2, Point2, bool) {
	x0, y0, x1, y1 := span[0], span[1], span[2], span[3]
	var open []int
	for i := range vertices.points {
		p := vertices.points[i]
		if vertices.degree[i] < 2 &&
			p.X >= x0-vertexEpsFeet && p.X <= x1+vertexEpsFeet &&
			p.Y >= y0-vertexEpsFeet && p.Y <= y1+vertexEpsFeet {
			open = append(open, i)
		}
	}
	var corner, spanning [][2]Point2
	for s := 0; s < len(open); s++ {
		for _, ib := range open[s+1:] {
			ia := open[s]
			connected := false
			for _, e := range edges {
				if (e[0] == ia && e[1] == ib) || (e[0] == ib && e[1] == ia) {
					connected = true
					break
				}
			}
			if connected {
				continue
			}
			a, b := vertices.points[ia], vertices.points[ib]
			fillsX := nearVertex(f64min(a.X, b.X), x0) && nearVertex(f64max(a.X, b.X), x1)
			fillsY := nearVertex(f64min(a.Y, b.Y), y0) && nearVertex(f64max(a.Y, b.Y), y1)
			switch {
			case fillsX && fillsY:
				corner = append(corner, [2]Point2{a, b})
			case (fillsX && nearVertex(a.Y, b.Y)) || (fillsY && nearVertex(a.X, b.X)):
				// A straight sub-line spanning the box along one axis,
				// degenerate across it: the shape of a segment whose
				// recorded box is looser than the line.
				spanning = append(spanning, [2]Point2{a, b})
			}
		}
	}
	tier := corner
	if len(tier) == 0 {
		tier = spanning
	}
	if len(tier) == 1 {
		return tier[0][0], tier[0][1], true
	}
	return Point2{}, Point2{}, false
}

func nearVertex(a, b float64) bool { return math64Abs(a-b) <= vertexEpsFeet }

func f64min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func f64max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func traceSketchLoops(vertices *planVertexRegistry, edges [][2]int) [][]Point2 {
	type half struct{ edge, to int }
	adjacency := make([][]half, len(vertices.points))
	for i, e := range edges {
		adjacency[e[0]] = append(adjacency[e[0]], half{i, e[1]})
		adjacency[e[1]] = append(adjacency[e[1]], half{i, e[0]})
	}
	used := make([]bool, len(edges))
	var loops [][]Point2
	for start := 0; start < len(vertices.points); start++ {
		allUsed := true
		for _, h := range adjacency[start] {
			if !used[h.edge] {
				allUsed = false
				break
			}
		}
		if allUsed {
			continue
		}
		var order []int
		current := start
		last := -1
		for {
			step, ok := func() (half, bool) {
				for _, h := range adjacency[current] {
					if !used[h.edge] && h.edge != last {
						return h, true
					}
				}
				return half{}, false
			}()
			if !ok {
				break
			}
			used[step.edge] = true
			last = step.edge
			current = step.to
			if current == start {
				break
			}
			order = append(order, current)
		}
		if len(order) < 3 || current != start {
			return nil
		}
		ring := []Point2{vertices.points[start]}
		for _, i := range order {
			ring = append(ring, vertices.points[i])
		}
		loops = append(loops, ring)
	}
	for _, done := range used {
		if !done {
			return nil
		}
	}
	return loops
}

// profileFromLoops merges collinear vertices, orders loops by area and
// splits them into pieces and voids by containment depth.
func profileFromLoops(loops [][]Point2) *PlanProfile {
	merged := make([][]Point2, 0, len(loops))
	for _, one := range loops {
		simplified := mergeCollinear(one)
		if len(simplified) < 3 {
			return nil
		}
		merged = append(merged, simplified)
	}
	absArea := func(ring []Point2) float64 { return math64Abs(signedArea(ring)) }
	sortByArea := func(rings [][]Point2) {
		for i := 1; i < len(rings); i++ {
			for j := i; j > 0 && absArea(rings[j]) > absArea(rings[j-1]); j-- {
				rings[j], rings[j-1] = rings[j-1], rings[j]
			}
		}
	}
	sortByArea(merged)
	// Each loop's depth is how many larger loops contain it. An even
	// depth starts a piece; an odd depth is a void of the loop it sits
	// in. A loop partly inside another is not a region at all.
	container := make([]int, len(merged))
	depth := make([]int, len(merged))
	for i := range container {
		container[i] = -1
	}
	for i, one := range merged {
		parent := -1
		for earlier := 0; earlier < i; earlier++ {
			within := 0
			for _, p := range one {
				if pointInPolygon(merged[earlier], p) {
					within++
				}
			}
			if within == len(one) {
				parent = earlier
			} else if within > 0 {
				return nil
			}
		}
		if parent >= 0 {
			depth[i] = depth[parent] + 1
		}
		container[i] = parent
	}
	var pieces []PlanPiece
	pieceOf := make([]int, len(merged))
	for i := range pieceOf {
		pieceOf[i] = -1
	}
	for i, one := range merged {
		ring := append([]Point2(nil), one...)
		if depth[i]%2 == 0 {
			if signedArea(ring) < 0 {
				reverseRing(ring)
			}
			pieceOf[i] = len(pieces)
			pieces = append(pieces, PlanPiece{OuterXY: ring})
		} else {
			if signedArea(ring) > 0 {
				reverseRing(ring)
			}
			if container[i] < 0 {
				return nil
			}
			piece := pieceOf[container[i]]
			if piece < 0 {
				return nil
			}
			pieces[piece].InnerXY = append(pieces[piece].InnerXY, ring)
		}
	}
	if len(pieces) == 0 {
		return nil
	}
	return &PlanProfile{
		OuterXY: pieces[0].OuterXY,
		InnerXY: pieces[0].InnerXY,
		Pieces:  pieces[1:],
	}
}

func reverseRing(r []Point2) {
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
}

func mergeCollinear(points []Point2) []Point2 {
	count := len(points)
	out := make([]Point2, 0, count)
	for index := 0; index < count; index++ {
		previous := points[(index+count-1)%count]
		current := points[index]
		next := points[(index+1)%count]
		fx, fy := current.X-previous.X, current.Y-previous.Y
		sx, sy := next.X-current.X, next.Y-current.Y
		cross := fx*sy - fy*sx
		scale := math64Hypot(fx, fy) * math64Hypot(sx, sy)
		if scale > 0 && math64Abs(cross/scale) < collinearEps {
			continue
		}
		out = append(out, current)
	}
	return out
}

func math64Hypot(a, b float64) float64 {
	return math.Hypot(a, b)
}

// signedArea of a closed ring (positive = counter-clockwise).
func signedArea(points []Point2) float64 {
	total := 0.0
	for i := range points {
		p := points[i]
		q := points[(i+1)%len(points)]
		total += p.X*q.Y - q.X*p.Y
	}
	return total / 2
}

// pointInPolygon reports whether p lies inside the closed polygon
// (even-odd rule).
func pointInPolygon(polygon []Point2, p Point2) bool {
	within := false
	if len(polygon) == 0 {
		return false
	}
	previous := polygon[len(polygon)-1]
	for _, current := range polygon {
		if (current.Y > p.Y) != (previous.Y > p.Y) {
			crossing := current.X + (p.Y-current.Y)*(previous.X-current.X)/(previous.Y-current.Y)
			if p.X < crossing {
				within = !within
			}
		}
		previous = current
	}
	return within
}

// PlanProfilesFromRecords groups OST_SKETCH_LINES element records by
// their owner reference and reconstructs each owner's plan profile.
// Records without an owner, owners with fewer than three segments and
// sketches that do not close unambiguously are dropped. The result maps
// sketched element id → profile.
func PlanProfilesFromRecords(records []ElementRecord) map[uint32]*PlanProfile {
	const ostSketchLines = -2_000_045
	byOwner := map[uint32][][4]float64{}
	var owners []uint32
	for i := range records {
		r := &records[i]
		if r.Category != ostSketchLines || r.OwnerRef == 0 {
			continue
		}
		if _, seen := byOwner[r.OwnerRef]; !seen {
			owners = append(owners, r.OwnerRef)
		}
		byOwner[r.OwnerRef] = append(byOwner[r.OwnerRef],
			[4]float64{r.BBox[0], r.BBox[1], r.BBox[3], r.BBox[4]})
	}
	out := make(map[uint32]*PlanProfile, len(byOwner))
	for _, owner := range owners {
		segs := byOwner[owner]
		if len(segs) < 3 {
			continue
		}
		if p := PlanProfileFromSegments(segs); p != nil {
			out[owner] = p
		}
	}
	return out
}
