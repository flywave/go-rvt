# go-rvt

纯 Go 的 Autodesk Revit `.rvt` 文件解析库——无 CGo、无外部依赖。从 RVT 复合文档（OLE/CFB）中提取元素模型、几何拓扑与元数据。

## 安装

```
go get github.com/flywave/go-rvt
```

## 快速开始

### 命令行

```
go run ./cmd/rvtinfo model.rvt
```

输出 JSON（元素模型摘要、包围盒计数、类身份普查、视图成员、deposit 语义等）。

### Go API

```go
// 全量解析：元数据 + 几何模型
md, err := rvt.OpenFile("model.rvt")

// 几何模型（需分区数据）
f, _ := rvt.OpenCFB(data)
raw, _ := f.Stream("Partitions/13")
model := part.Model("2023") // 或 "2017" / "2022" 等

// 元素清单（100% 类身份）
for _, e := range model.Elements {
    fmt.Println(e.ID, e.ClassName, e.BBox)
}

// 视图成员表
for _, v := range model.Views {
    fmt.Println(v.ID, v.Members)
}

// BRep 拓扑骨架
for _, b := range model.BReps {
    fmt.Println("faces:", len(b.FaceIDs), "edges:", len(b.EdgeIDs))
}

// 按元素绑定的线段与弧
for _, c := range model.BoundCurves {
    fmt.Println(c.ElementID, c.Origin, c.Direction)
}
for _, a := range model.InlineArcs {
    fmt.Println(a.ElementID, a.Radius, a.Center)
}

// 内嵌 schema（Formats/Latest，类名→序列化 tag）
schema, _ := rvt.OpenSchema("model.rvt")
fmt.Println(schema.ByName("ElementHeader").Tag)

// 2023 元素记录（BuiltInCategory/放置类型/宿主引用/包围盒）
rep, _ := rvt.RecordsFromStream(data)
for _, rec := range rep.Records {
    fmt.Println(rec.ElementID, rec.Category, rec.IsPlacedInstance(), rec.BBox)
}

// 草图平面轮廓（楼板/屋顶/天花的真实边界，约束闭合求解）
for id, profile := range rep.Profiles {
    fmt.Println(id, len(profile.OuterXY), "vertices")
}

// 墙位置线（有界线/弧）与类型层厚
walls := rvt.ScanFirstBoundedLines(partition, rep.Schema, wallIDs, 2023)
layers := rvt.ScanCompoundLayerLists(partition, declared, 2023)

// 类型层厚→实例关联 + 类型名（构件树维度）
thk := rvt.TypeLayerThickness(partition, rep.Schema, declared, 2023)
join := rvt.TypeJoinByReference(rep.Records, thk)   // 实例→类型，零歧义
names := rvt.ScanElementDataNames(partition, rep.Schema, typeIDs, 2023)
```

## 支持的 Revit 版本

| Revit | BFI rev | 分区版本 | 类索引 |
|---|---|---|---|
| 2015 | 10 | 7 | ✅ (1,975 类) |
| 2016 | 10 | 7 | ✅ (2,628 类) |
| 2017 | 10 | 7 | ✅ (2,691 类) |
| 2018 | 10 | 7 | ✅ (2,749 类) |
| 2019 | 10 | 7 | ✅ (2,782 类) |
| 2020 | 13 | 9 | ✅ (2,823 类) |
| 2021 | 13 | 9 | ✅ (2,849 类) |
| 2022 | 13,14 | 9 | ✅ (2,862 类) |
| 2023 | 14 | 9 | ✅ (2,889 类) |
| 2024–2026 | 14 | 9 | ✅ (2,889–3,196 类) |

## 包结构

```
rvt/            主包：Open/Model/ReadPartition + 元素层/动态段/扩展尾
rvt/geom/       对象模型契约（Point3/BBox/BRep/View/Element/Arc/NurbSpline…）
rvt/classindex/ 全年份类索引（2011–2026 嵌入 CSV；Index4Enum→OdBm* 类名）
bfi/            BasicFileInfo TLV 解析（rev 10/14）
internal/cfb/   OLE/CFB 复合文档只读（嵌套存储递归）
internal/zg/    gzip 成员解压（精确边界；footer 宽容校验）
cmd/rvtinfo/    CLI：rvtinfo <file.rvt> → JSON
```

## 与参考实现的交叉验证

`ref/rvt-rs-main`（净室 Rust 实现，Apache-2.0）作为对拍基准（2026-10）：

- **容器层逐字节一致**：页剥离（65,249B 页 = 64,896B 载荷 + 353B 校验尾）后，
  三个回归样例的膨胀分区与 rvt-rs 完全相同（2023/2022/2017，
  94.4/75.8/75.7 MB）——该修复把此前的"损坏页"全部转为干净验证页。
- **Formats/Latest schema**：精确文法解析 4,418 类（2023），关键 tag 与
  rvt-rs 逐一相同（Outline=316 / ElementParents=1410 / ElementHeader=1398）。
- **2023 元素记录**：1,197 条 / 876 放置实例，含 BuiltInCategory、放置类型、
  宿主/所有者引用与包围盒；与 rvt-rs 逐条一致（bbox 1,197/1,197，1e-9）。
  其他版本 fail-closed（2024–2026 布局已就位并以合成帧测试锁定；
  **2022 布局已解码**——ElementHeader 帧 + bbox@52 + 引用列表@100，
  6,513 条记录接入桥接）。
- **参数化几何首批**（Go 侧超越基准 2023 能力）：草图平面轮廓 9 个闭合
  （楼板/屋顶真实边界，约束求解不闭合即拒绝）、56/56 墙位置线、类型层厚
  27 列表（0.39–1.51 ft）。
- **类型维度**：层厚→实例引用关联 62/62 零歧义（47 墙+8 楼板+7 天花/屋顶），
  类型名 27/27 全解析（"Interior - 165 Partition (1-hr)" 等）——构件树与
  精确墙体厚度就位。
- **家族维度**：FamilySymbol 关联 434/876 实例（门 16/16、窗 17/17，数据侧
  [ff×4][id] 消歧），类型名全解（门 "800 x 2100"/"Entrance door"、
  窗 "Standard"——名字深度可达 3 KB+，全窗口搜索）。
- **类型尺寸投票**：同符号实例数据 f64 投票解锁类型宽高（窗 1500×2700mm
  等 15 符号），17/17 窗 + 16/16 门建成定向叶片盒并开洞（trusted 几何门
  + 启发式几何一致性门）；参数显示名不在 .rvt 内（内置参数名=SDK 枚举
  2,284 条，BuiltinParamByID/SearchBuiltinParams）。
- **内置参数表**：SDK BmBuiltInParameter.h 的 2,284 条定义生成
  id→名/类型/单位 Go 映射（`BuiltinParamByID`/`SearchBuiltinParams`）。
- **实例摆位**：372/373 FamilyInstance 解出变换矩阵（3×3 旋转正交门
  + 原点），92.7% origin 落在记录 bbox 内。
- **实体覆盖**：桥接 shaped=2,585/12,194（65 真实实体 + 2,520 bbox 代理盒），
  15 处门窗开洞从宿主墙实体布尔扣除。

## 架构

```
.rvt (OLE/CFB)
 ├ BasicFileInfo        BFI TLV（rev 10/14）
 ├ TransmissionData     u32 + UTF-16 XML（外部引用）
 ├ Global/ElemTable     6B 头 + 28B×N [id,id,0,depA,depB,0,-1]
 ├ Global/PartitionTable 工作集名
 ├ Formats/Latest       内嵌 schema：类定义文法（tag=定义序数，首类=12）
 ├ Partitions/<n>       校验页（65,249B=64,896B+353B 尾，解码前剥离）
 │                      12B 头 + 记录环 [counter][size][body][echo] × N
 │   ├ class-101 子流   ElementHeader 静态 schema + 2023 元素记录帧
 │   ├ class-102 子流   G 节点对象存储
 │   └ class-103 子流   GeomStep 目录 + G 树 + 内联几何
 ├ Contents             魔数帧 [62 19 22 05]
 └ ProjectInformation   ZIP → Atom XML（项目参数）
```

### 解码管线

```
OpenCFB → ReadPartition → ScanGTrees（G 树块）
                        → ScanGCatalogRuns（Face/Edge 交替序列）
                        → ScanInlineGLines（[GLine][f64×6] 内联）
                        → ScanGArcs（单位正交+正半径内容锚定）
                        → ScanGLines（记录环 [counter][size][body][echo]）
                        → ScanDeposits（deposit counter 域）
Model(version) → Elements + Views + BReps + BoundCurves + BoundArcs + InlineCurves
```

## 测试

```bash
# 快速测试（无样例也能跑核心层）
go test ./...

# 完整回归测试（需将 RVT 样例放入 rvt/testdata/，见 rvt/testdata/README.md）
go test ./rvt/ -v
```

## 文档

| 文档 | 内容 |
|---|---|
| [`analysis/rvt-format.md`](analysis/rvt-format.md) | RVT 格式总纲（权威索引） |
| [`analysis/findings.md`](analysis/findings.md) | 结论台账 R3–R103（101 条，含证据链） |
| [`analysis/rvt-container.md`](analysis/rvt-container.md) | CFB 流地图 + 帧/压缩体系 |
| [`analysis/rvt-basicfileinfo.md`](analysis/rvt-basicfileinfo.md) | BasicFileInfo TLV 布局 |
| [`analysis/01-object-envelope.md`](analysis/01-object-envelope.md) | 对象信封与值解码链 |
| [`analysis/02-elemtable.md`](analysis/02-elemtable.md) | ElemTable 布局 |
| [`analysis/03-geometry-model.md`](analysis/03-geometry-model.md) | 几何存储模型决策 |
| [`analysis/04-g-classes.md`](analysis/04-g-classes.md) | G 节点类型系统 + 基元字段表 |
| [`analysis/05-unknowns-closeout.md`](analysis/05-unknowns-closeout.md) | 未知收口批次 |
| [`analysis/06-class-index.md`](analysis/06-class-index.md) | 类索引全表方法 |
| [`analysis/07-dynamic-queue.md`](analysis/07-dynamic-queue.md) | DynamicQueue 队列语义 |

## 许可

MIT
