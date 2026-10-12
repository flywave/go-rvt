# RVT 格式总纲（go-rvt）

> 权威索引文档：整合 M1–M3.5 实现 + 三轮 REA/样例提取（台账 R1–R39）。
> 分层详情见编号文档；本文件只给全局图景与入口。
> 证据约定：E1=BimRv 头文件，E2=TB_* 模块反编译（Ghidra/nm/objdump），E3=双样例字节实测
> （2023 racbasic / 2022 rstadvanced，官方 sample；大流存在系统性 +353 索引陈旧页 R29）。

## 0. 全景

```
.rvt = OLE/CFB 容器                                          [rvt-container.md]
 ├ BasicFileInfo        TLV UTF-16 元数据（rev=14）           [rvt-basicfileinfo.md]
 ├ TransmissionData     u32 len + UTF-16 XML（外部引用）
 ├ ProjectInformation   ZIP
 ├ RevitPreview4.0      62192205 帧 + gzip（预览图）
 ├ Contents             魔数帧链：GLOBAL 索引（名称+GUID+build）
 ├ Formats/Latest       0头+gzip：A3Party(ACIS) 注册表
 ├ Global/*             8B 头+单 gzip 成员+OdBmFileCRC 尾
 │   ├ ElemTable        6B 头 + 28B×N [id,id,0,depA,depB,0,-1]   [02-elemtable.md]
 │   ├ Latest/History/ContentDocuments/DocumentIncrementTable
 │   └ PartitionTable   工作集名（任意字符串）
 └ Partitions/<n>       12B 头(v9) + 32B 页记录×(N+1) + gzip 页链 + 签名块 + 终结器
     ├ class-101 子流   103B deposit 记录区
     ├ class-102 子流   同 counter 异型头
     ├ class-103 子流   目录表（递增小 id）
     └ 页数据           对象记录流 → 对象信封解码              [01-object-envelope.md]
```

## 1. 对象信封（核心契约）

```
记录循环（分区页数据层）: [u32 counter][u32 size][对象体…]     [counter/size 消费点待追]
对象体 = Int16 类索引（符号扩展→ClassesContainer::find）
       → per-class 生成 reader（快）或 GenericObjectReader（慢）
       → 属性序列（无盘上属性头；序=类元数据注册序）
每属性值 = fieldType 分发:
 1=bool(1B) 2=i8 3=i16 4/5=i32 6=f32→f64 7=f64
 8=string[布局?] 9=guid16B 10=i16
 13=itemMode(0/6标量,1定长数组,5变长数组)
 14=对象引用(递归 Int16 类索引+体; 值类型名 ElementId/ElementIdPlusIndex/GUIDvalue)
之后: initReferences（IdReference 两阶段回填）+ **动态属性段=队列驱动连续标准对象读**（R57，非位压缩）
表流尾: u32 UnitSystem{0=Metric,1=Imperial}
```

类索引**按年份重编号**：查 `tables/class-index-{2022,2023}.csv`
（1398@2023 = 1340@2022 = OdBmElementHeader）。91/102/103 是分区页/子流标签，非类索引。

## 2. 几何（go-topo 对接面）

- 决策：**直读 G 节点缓存树**（Element.getGeometry→GElement→GRep→…），非参数化再生 [03-geometry-model.md]
- G* 字段表（GLine=OriginPoint+DirectionVector、GArc/GEllipse/NurbSpline/BRep/Face/Edge 链表…）
  → [04-g-classes.md] + `tables/g-node-properties.txt`
- 类索引锚点：GElement=2073、GBrep=2044、GLine=1798、GEdge=1301、GPoint=2098、
  GeomTable=2155、Geometry=2160（2023）
- 外部导入形状：A3PartyObject（SAT）；链接模型：GRvtLink

## 3. 压缩/校验体系

- 表流：8B 头 + 单 gzip（大流 CRC/ISIZE footer 非标准——宽容校验）
- 分区页：gzip 成员 + 32B 页记录（ownLen/prevLen 链、markA/B、class 101/102/103）+ OdBmFileCRC
- 页内链式字典（部分页以 32KB 窗口 dict 跨页压缩）
- +353 系统性索引陈旧页（R29）：可解码、索引长度差恒 −353

## 4. 未知状态（第 4 轮收口后）

| 项 | 状态 |
|---|---|
| +353 页回收 | ✅ 实用规则定案（ownLen−8/+353/dict/重同步；R40，05-unknowns-closeout.md §A） |
| String/Guid 布局 | ✅ i32+raw / raw16B（R43） |
| G* 属性×Type | ✅ 用法表（R44；disk fieldType 映射仍 [?]） |
| Formats/Latest | ✅ 类型反射注册表（R44） |
| ProjectInformation | ✅ ZIP→Atom XML（R44） |
| GInfoFlags | ✅ E1 全表（04-g-classes §3b） |
| 签名 64B | 🟡 非对称签名定性（R41）；公钥[?] |
| [counter][size] 循环 | 🟡 路由=DepositId×3 定案；字节层在 BmIODepositPE（R42） |
| 动态段 | 🟡 队列序标准信封（R57，位压缩假设作废）；字节框架需队列语义 |
| ElementId 裸宽 | 🟡 队列回填定案；裸宽在 PE/objectReader[?] |
| NurbSpline 权重 | ✅ WPoint={f64 weight, Point3d}=4×f64 内联（R54） |
| 动态属性段 | ✅ 队列驱动标准对象读（R57）；~~位压缩~~作废 |
| BFI 修订 | ✅ 接受表 2011→{3}…2023–26→{14}（R53）；rev≠14 布局=外部样例 |
| 签名 64B | ✅ SDK 无验证路径=跳过（R55）；算法=外部（Revit 侧） |
| ElementHeader/ElemRec schema | ✅ 属性表+字节级（R45/R49/R56） |
| [counter][size] 循环 | ✅ stride=12+size，class-101（R45） |
| ElementId 裸宽 | ✅ i32，null=-1（R49） |
| ElemTable depA/B、deposit91、Global 头 u32、BFI u8 标记 | [?]（BFI 标记需协同样例） |
| 干净/协同样例、签名公钥算法、BFI rev≠14、CondInt16 v≠0 长度、Contents 帧外层 | **外部依赖/低价值** |

## 5. 文档索引

| 文件 | 内容 |
|---|---|
| rvt-container.md | CFB 流地图、帧/压缩体系 |
| rvt-basicfileinfo.md | BFI TLV |
| 01-object-envelope.md | 信封调用链 + fieldType 表 |
| 02-elemtable.md | ElemTable 布局 |
| 03-geometry-model.md | 几何模型决策、UnitSystem、坐标、实例 |
| 04-g-classes.md | G 节点类型系统 + 基元字段表 |
| 06-class-index.md | 类索引全表方法与勘误 |
| bim-sdk-modules.md / oda-bim-clues.md | SDK 模块地图 / 消费方线索 |
| findings.md | 台账 R1–R39 |
| tables/*.csv, g-node-properties.txt | 提取产物 |
