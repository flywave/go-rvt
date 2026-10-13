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

## 架构

```
.rvt (OLE/CFB)
 ├ BasicFileInfo        BFI TLV（rev 10/14）
 ├ TransmissionData     u32 + UTF-16 XML（外部引用）
 ├ Global/ElemTable     6B 头 + 28B×N [id,id,0,depA,depB,0,-1]
 ├ Global/PartitionTable 工作集名
 ├ Partitions/<n>       12B 头 + 记录环 [counter][size][body][echo] × N
 │   ├ class-101 子流   ElementHeader 静态 schema
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
