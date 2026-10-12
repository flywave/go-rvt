# G 节点类型系统与几何基元字段表（P3 #14）— TB_Geometry 属性注册提取

证据：E2 `TB_Geometry.tx`（sha256=949017e1…，8MB）——每个 `__OdBmG<Class>propertiesConstructor`
内 `OdString(wchar*)` 字面量按注册序提取（objdump lea→UTF-32 串）；
属性注册序 = GenericObjectReader 的迭代序 = **序列化顺序**（R31/E4）。
产物：`tables/g-node-properties.txt`。

## 1. 类型系统鸟瞰

```
OdBmGNode (GInfo)
 ├ GRep (GeomBBox, ObjectType, GRepFlags, GTightBBox, ElementId)
 │  ├ GElement（视图相关几何容器；壳）
 │  │  └ …视图相关 GRep 树…
 │  ├ GBrep (Faces) ──→ GFace 循环
 │  ├ GGroup (AllSubNodes)
 │  ├ GPolyMesh (InteriorGStyleID, MaterialID, PolyMeshFlags, FacetedTopology)
 │  └ GRvtLink (InstanceInfo)   # 链接模型
 ├ 曲线: GCurve(EndParams) ─ GLine / GArc / GEllipse / GNurbSpline /
 │        GHermiteSpline / GPolyLine / GPolyCurve / GCylindricalHelix / GOffsetCurve
 ├ GPoint / GPointCloud
 ├ GInstance (InstanceInfo, EmbeddedSymbolGRep, …TagId)
 ├ GBitmap / GImage / GImposter / GDirectContext3D
 └ Overrider 族（LinePattern/Material/Fill*/StyleColor/…）与 Condition 族（视图条件）
```

## 2. 几何基元字段表（go-topo 转换核心）

| 类 | 属性（注册序=盘上序） | 解读 |
|---|---|---|
| **GLine** | OriginPoint, DirectionVector | 点向式直线 |
| **GPoint** | Coord, Size, PointFlags, BorderSize | |
| **GArc** | XDirection, YDirection, Radius, Center, IsFilled | 轴+半径+圆心 |
| **GEllipse** | Center, XDirection, YDirection, RadiusX, RadiusY | |
| **GNurbSpline** | Degree, Closed, Rational, ControlPoints, Knots | **无 Weights 单列——Rational 标志+可能点含权重 [?]** |
| **GHermiteSpline** | Periodic, NodeArray | |
| **GPolyLine** | Coordinates, Extents, IsFilled | 折线=坐标数组 |
| **GPolyCurve** | IsFilled, Curves | 曲线容器 |
| **GPolyMesh** | InteriorGStyleID, MaterialID, PolyMeshFlags, FacetedTopology | 网格 |
| **GCylindricalHelix** | Radius, PitchOver2PI, BasePoint, XVector, YVector, ZVector | |
| **GOffsetCurve** | BaseCurve, ReferenceVector, SignedOffsetDistance, NaturalDomain | |
| **GBrep** | Faces | BRep=面列表 |
| **GFace** | FirstLoop, FaceRegions, GFilling, CutType, FaceFlags, RenderStyleId, BackgroundFilling | |
| **GEdgeLoop** | NextLoop, Face, Next, Prev, Envelope, Open | 环=双向链表 |
| **GEdge** | Faces, Next, Prev, InteriorEdgePnts, FirstAndLastEdgePnts, Flags | 边=双向链表+点列 |
| **GRep** | GeomBBox, ObjectType, GRepFlags, GTightBBox, ElementId(i64) | 缓存条目头 |
| **GNode** | GInfo | GInfoFlags 位域 |
| **GGroup** | AllSubNodes | 子节点数组 |
| **GInstance** | InstanceInfo, EmbeddedSymbolGRep, ForbiddenTarget, ResolveSymInView, HasScale, TagId | 实例=内嵌符号 GRep+信息 |
| **GRvtLink** | InstanceInfo | 链接模型引用 |

**拓扑骨架**：GBrep→Faces→(GFace)→FirstLoop→(GEdgeLoop)→Next/Prev 链→(GEdge)→
InteriorEdgePnts/FirstAndLastEdgePnts——经典 winged-edge 变体；点列在 Edge 级而非独立 Vertex 表。

## 3. OdTfVariant::Type 用法表（第 4 轮 F 批，tables/g-node-fieldtypes.txt）

**注意**：Variant::Type ≠ 盘上 fieldType（01-object-envelope §2 为 disk 枚举）。

| Type | 含义 | 证据属性 |
|---|---|---|
| 2 | bool | IsFilled/Closed/Rational/Open/HasScale |
| 5 | i32 | Size/PointFlags/Degree/FaceFlags/CutType |
| 6 | i64 | GRep.ElementId |
| 7 | f64 | Radius/RadiusX/Y |
| 9 | 对象引用（单） | OriginPoint/Coord/Center/Envelope/Extents |
| 13 | ElementId/handle | MaterialId/LinePatternId/TagId/RenderStyleId/TargetViewId |
| 14 | 托管对象引用 | GEdgeLoop Face/Next/Prev |
| 263 | f64 数组 | GNurbSpline.Knots |
| 265 | 对象数组 | ControlPoints/Coordinates/Faces/PointPnts/FaceRegions |
| 270 | 链式引用数组 | GEdge.Faces/Next/Prev |

**NurbSpline 权重**：属性集仅 Degree/Closed/Rational/ControlPoints/Knots——无独立 Weights；
权重疑在 ControlPoints 元素对象内或 Rational 隐含 [?]。

## 3b. GInfoFlags 位域（E1 Geometry/Enums/BmGInfoFlags.h）

IsPickable=0x4、IsTransparent=0x8、IsMasking=0x10、IsReference=0x200、IsHandle=0x400、
ContainsControls=0x800、IsDetail=0x1000、IsModelGNode=0x8000、IsSubElement=0x40000、
IsActive=0x80000、IsDoubleSided=0x200000、IsCurveBounded=0x1000000、
IsHiPriHandle=0x10000000、DatumSurface=0x20000000、TransparentSurface=0x40000000、
PickableTransient=0x80000000；Default=0x80004、DefaultModel=0x88004。

## 4. 未竟（收口后残留）

- GElement 壳构造器属性在别处注册；GeomTable 属性表未单独提取
- Variant::Type → 盘上 fieldType 的映射函数（外部 getFieldType 实现）
