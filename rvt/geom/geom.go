// Package geom declares the RVT geometry object model — the contract
// between go-rvt and downstream consumers (go-topo).
//
// Two kinds of declarations live here:
//
//  1. Populated types: Element, View, BRep and BBox are extracted from the
//     partition streams today (findings R49/R50/R64/R56) and validated on
//     the official samples.
//  2. Declared-pending types: the curve/solid primitives (Line, Arc,
//     NurbSpline, …) mirror the OdBmG* reader schemas (findings R54) but
//     are NOT yet populated — G-node blob delimiting is the pending M7
//     campaign (R65). They fix the target shape of that work.
//
// Units: all coordinates are Revit internal units (feet); angles radians.
package geom

// Point3 is a 3D point or vector.
type Point3 struct {
	X, Y, Z float64
}

// BBox is an axis-aligned bounding box (Geometry(2160) objects carry their
// min/max corners inline — findings R56).
type BBox struct {
	Min, Max Point3
}

// Element is one element identity decoded from an OdBmElementHeader record
// (static schema, findings R49/R50).
type Element struct {
	ID             uint32 // deposit counter / element id (shared id space)
	ClassIndex     int16  // serialization class index (per-version)
	ClassName      string // resolved via classindex (e.g. OdBmWall)
	CategoryID     int32
	FamilyID       int32
	OwnerViewID    int32
	DesignOptionID int32
	UnplacedOwner  int32
	MiscID         int32
	ViewRulesFlags uint32 // rejected-view-types bitmask
	ABFlags        uint32
}

// View is one view's element membership, extracted from DBViewPlan
// extended tails (element-id runs — findings R64).
type View struct {
	ID      uint32   // view element id (record counter)
	Members []uint32 // element ids in the view
}

// BRep is a boundary-representation topology skeleton: the Face/Edge id
// sequences cataloged in the class-103 G region (findings R56). IDs index
// the G-node id space; geometry payloads await the blob campaign (R65).
type BRep struct {
	FaceIDs []uint32
	EdgeIDs []uint32
}

// WPoint is a NURBS control point: weight + position (OdBmWPoint —
// findings R59: weight is inline, no separate Weights property).
type WPoint struct {
	Weight float64
	Point  Point3
}

// Line is a point-direction straight line (OdBmGLine: OriginPoint,
// DirectionVector).
type Line struct {
	Origin    Point3
	Direction Point3
}

// Arc is a circular arc (OdBmGArc: XDirection, YDirection, Radius, Center,
// IsFilled).
type Arc struct {
	Center Point3
	XDir   Point3
	YDir   Point3
	Radius float64
	Filled bool
}

// Ellipse is an ellipse (OdBmGEllipse: Center, XDirection, YDirection,
// RadiusX, RadiusY).
type Ellipse struct {
	Center  Point3
	XDir    Point3
	YDir    Point3
	RadiusX float64
	RadiusY float64
}

// NurbSpline is a NURBS curve (OdBmGNurbSpline: Degree, Closed, Rational,
// ControlPoints, Knots — weights inline in WPoint).
type NurbSpline struct {
	Degree        int
	Closed        bool
	Rational      bool
	ControlPoints []WPoint
	Knots         []float64
}

// PolyLine is a polyline (OdBmGPolyLine: Coordinates, Extents, IsFilled).
type PolyLine struct {
	Coordinates []Point3
	Filled      bool
}

// Facet is one triangle of a poly-mesh.
type Facet struct{ A, B, C Point3 }

// PolyMesh is a faceted mesh (OdBmGPolyMesh: FacetedTopology + style ids).
type PolyMesh struct {
	Facets     []Facet
	MaterialID int32
	Flags      uint32
}

// Instance places a GRep symbol in model space (OdBmGInstance:
// InstanceInfo + EmbeddedSymbolGRep + HasScale + TagId).
type Instance struct {
	Transform [4][3]float64 // 4x3 affine (row-major)
	HasScale  bool
	TagID     int32
}
