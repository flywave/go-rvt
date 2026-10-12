# 几何存储模型（P3 #13 决策项）+ UnitSystem（#4）+ 坐标基准（#16）

证据：E1 头文件（路径相对 `BimRv/Include/`）+ `tables/class-index-2023.csv` + Formats/Latest 解压实测。

## 1. 决策：文件同时存「参数化定义」与「BRep 几何缓存」——go-topo 走**直读 G\* 缓存树**

| 证据 | 内容 |
|---|---|
| E1 `Database/BmElement.h:126,648` | `getGeometry()`；`getGeometry(GeometryOptions, obj)` 注释明言 *"geometry … collected from **OdBmGElement (geometry cache)**"*；Element 还有 `GeomTable`/`GeomStepList` 设置器（再生步骤表） |
| E1 `Geometry/Entities/BmGElement.h` | `OdBmGElement : OdBmGRep`——"stores geometric representation of the element; one or more **view-dependent geometries**" |
| E1 `Geometry/Entities/BmGeometry.h` | `OdBmGeometry : OdBmGBrep`——"stores geometry of **B-Rep objects**"：edges(OdBmGEdge[])、flags、GeometryTag、TessEpsCntrl（三角化容差控制器——与 FileProcessorRvt 的 wrTriangulationParams 呼应） |
| E1 `Geometry/Entities/BmGRep.h` | GRep 基类：GeomBBox/ObjectType/GRepFlags/TightBBox/**ElementId**(i64) |
| E1 `Geometry/Entities/BmGeomTable.h` | GeomTable = GeomTabEntry + GeomMaterialMarker + GeomFaceTypeMarker（缓存索引表） |
| csv 2023 | G 族全部在**序列化类索引空间**：GEdge=1301、GEdgeBase=1302、GNode=1303、GEdgeLoop=1312、GLine=1798、GBrep=2044、GElement=2073、GPoint=2098、GPointCloud=2099、GeomTable=2155、Geometry=2160（另 A3Party 相关在 Main） |
| E1 `Main/Entities/BmA3PartyObject.h` | A3PartyObject/AImage/SECImage/SECJpeg = 第三方几何/图像容器（公开头为空壳，字段在 Impl；Formats/Latest 注册表已见 A3PartyAImage/Object/SECImage 三名） |
| E1 参数化侧 | CurveElemData/EllipseElemData/HermiteSplineData 等存**参考线/点**（getEllipseRefLines/RefPoints）+ CurveDriver/AssocProp——定义层而非网格层 |

**结论（R34）**：RVT 模型几何的可直读载体是 **GElement→GRep→(GBrep|GLine|GPoint|GEdge…) 对象缓存树**（经对象信封 + 类索引表解码）；参数化驱动层（Driver/GStep/AssocProp）仅在需要再生时重要。go-topo 策略 = **转换器**（读缓存 BRep 树→topo），**不是再生引擎**。SAT/A3Party 仅覆盖外部导入形状，非主路径。

**工作量含义**：完整 G* 字段级解码 = 下一轮 REA 主战役（GBrep/GEdge/GLine 序列化 reader，快路径 per-class 生成码在 TB_Geometry/TB_FormatReaders）；本轮先钉类图与入口（GElement/GeomTable 按信封通用规则可解出骨架）。

## 2. UnitSystem（#4 定案）

E1 `Database/Enums/BmUnitSystem.h`：

| 值 | 名 |
|---|---|
| 0x00000000 | Metric |
| 0x00000001 | Imperial |

表流 FixedFooterReader 的 u32 尾 ∈ {0,1} 即单位制（R25）。分区流无此尾（分区未见 FixedFooter 路径）。

## 3. 坐标基准（#16 头文件层）

E1 `Essential/Entities/BmBasePoint.h`：
- 每项目一个 Project Base Point（category OST_ProjectBasePoint）+ 一个 Survey Point（OST_SharedBasePoint）
- `getLocationType()`：`LocationType::Project` / `LocationType::Shared`
- `getPosition()` / `getSharedPosition()` / `getTransformedPosition()` / `getProjectPosition(coord, &east, &north)`
- 与 3D Repo 消费方代码（oda-bim-clues R7）三坐标系模型一致：Internal Origin ← Project Base ← Survey（共享）

GeoLocation.getActiveLocationId().getTransform()（消费方代码）= 模型→共享坐标矩阵；E1 侧对应 `OdBmGeoLocation`（Essential/Entities/BmGeoLocation.h，getter 待逐项列出——并入下一轮）。

## 4. 实例放置（#15 头文件层）

E1 `Database/Entities/BmInstance.h:72`：`OdGeMatrix3d getTransform() const`（4×3 仿射，标准 OdGe 编码 [?]——盘上字段布局随 Instance 类信封解码时定）。

## 5. 场景骨架（#17 头文件层）

- `Essential/Entities/BmLevel.h`、`BmGrid.h`（+GridChain/IntPnt/Intersection 族）在库；字段级 getter 待逐项列出（下一轮并入 G* 战役时顺带）
- Category 目录已由 BuiltinCategories 宏与 csv 类名覆盖（OST_* 见 oda-bim-clues R6）

## 6. 下一轮 GEOMETRY 战役入口（预告）

1. REA：`OdBmGElement/GBrep/GEdge/GLine/GPoint` 的 per-class 生成 reader（csv 锚点 2073/2044/1301/1798/2098）
2. GeomTable(2155)/GeomTabEntry 布局 → ElementId(i64)→缓存条目路由
3. TessEpsCntrl / GInfoFlags 枚举（E1 Enums/BmGInfoFlags.h）
