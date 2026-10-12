# REA 提取清单 — BimRv SDK（面向全量对象/场景建模 + go-topo 转换）

> **进度（第 4 轮未知收口后）**：#1-#6、#13-#16 ✅/🟡；**#14 ✅（Type 表+GInfoFlags）**；
> **#10 🟡（ownLen/+353 回收规则 R40）**；#11 🟡（CRC 表在 PE）；#12 🟡（签名=非对称定性 R41）；
> 总纲 ✅；收口详情 `05-unknowns-closeout.md`（R40–R44）。剩余全部为外部依赖或 PE 字节层：
> 干净/协同样例、BmIODepositPE.cpp、签名公钥、BFI rev≠14、NurbSpline 权重位置。

目标链：分区容器(✅M3.5) → **对象信封(✅R30–R32)** → 元素图/类注册表(✅#5/🔄#6) → 参数与几何(⬜) → go-topo。

SDK 位置：`go-dgn/ODA-Parse-DWGs-main/archives/BimRv_lnxX64_11dll.lic/`（83 模块、未 strip、4722 头文件）。
原则：**头文件优先（E1，便宜）**，反编译只用于序列化内部（read/write/布局代码）。
每项交付物统一为：伪代码 + 字段偏移表 + 枚举值表，落入 `analysis/` 编号文档。

## P0 对象信封（解锁分区子流数据 —— 当前最大黑盒）

| # | 目标 | 模块/符号 | 要回答的问题 | 解锁 |
|---|---|---|---|---|
| 1 | **子流记录信封** | TB_Database（或含 ObjectPtrReader 的模块；TB_Loader 栈分析已知其在外部库）| 我们实测记录头 `[u32 counter][u32 size][u32 类索引(1398/1340)]`，其后：信封还剩哪些字段（flags/版本/引用计数）？`size` 是否=记录字节长？嵌套子对象 vs 内联数据如何区分（实测 `0000ffff`/`0080ffff`/`ff×32` 标记段语义）？ | rvt 对象区全量解码 |
| 2 | **对象引用编码** | 同上 | 对象图边如何内联存储（ElementId？流内偏移？类索引+id 对？） | 元素关联建模 |
| 3 | **deposit 记录(103B) 字段表** | 类索引 91/1398/1340 对应类（先查 #5 的枚举表）| counter 语义（2023 +1 / 2022 跳变）；A/B 块内时间戳样字段（实测递增值 0x7d115 系）含义 | 子流头解释 + 增量保存史 |
| 4 | **UnitSystem 枚举** | FixedFooterReader 消费点（TB_Loader ObjectStreamReader 栈尾）| u32 尾的全部取值与含义（单位制） | 所有数值字段的单位解释 |

## P1 元素目录与类注册表（对象图入口）

| # | 目标 | 模块/符号 | 要回答的问题 | 解锁 |
|---|---|---|---|---|
| 5 | **Index4Enum 全表（2022/2023）** | TB_Format2022Classes / TB_Format2023Classes（静态注册表在 .data/.rodata；nm/objdump 可定位 RTINIT/registry 表）| classIndex→className 完整映射（CSV）。已有锚点：91、1398(2023)/1340(2022)、0x821/0x7df、ElemTable=1481@2026、PartitionTable=3200@2026 | 一切类索引解释；deposit/目录表类身份 |
| 6 | **ElemTable 布局** | TB_Format2023Readers `ElemTable2011..2023Reader::read`（Global/ElemTable 解压后 ~22KB+）| 每元素条目字段：id、类索引、**指向哪个分区/子流/deposit/偏移**？这是 ElementId→数据定位的总目录 | 元素清单 + 数据路由表 |
| 7 | **9 条表流分派全表** | TB_Loader `readStream@0x1a19ee` 字符串表 | 9 个路径名→reader 类完整对应；ContentDocuments（链接文档=场景！）、History、DocumentIncrementTable 各自布局 | 链接模型/历史/场景组织 |
| 8 | **class-103 目录表记录** | 类索引 0x821(2023)/0x7df(2022) 对应类 | 子流头 `[类索引][递增 id 4..N]` 表项结构：id 索引什么？ | 子流内部路由 |
| 9 | **class-102 头记录** | field2=87 对应类 | `[counter][0xf77f22cb][87]` 记录身份与字段 | 同上 |

## P2 分区容器残项（低优先，一次顺手）

| # | 目标 | 模块/符号 | 要回答的问题 |
|---|---|---|---|
| 10 | 页记录剩余字段 | 消费 kind/counter/payload 的读取代码 | kind(4/5/6/7)、counter(递减)、payload(~90%解压尺寸) 语义 |
| 11 | OdBmFileCRC 参数表 | `verifyAndCutCRC@0x197058` 的 findBySize 表 | size→CRC 参数集；markA/markB(0x0e47/0e4e) 是否即参数 | 
| 12 | 签名块哈希算法 | 签名记录写出点 | 64B 哈希（SHA-512/Ed25519?）+ 触发规律（每~2 个 class-103 页） |

## P3 几何与场景（go-topo 对接）

| # | 目标 | 模块/符号 | 要回答的问题 | 解锁 |
|---|---|---|---|---|
| 13 | **几何存储模型（关键决策项）** | TB_Database 加载后几何来源：TB_ModelerGeometry 调用点、`OdBmGeometryElement`/`OdBmSolid` 等类的 in-file 数据 vs 再生计算（regeneration/driver 触发点）| 文件里存的是参数化定义+缓存，还是 BRep？哪些类携带显式几何（A3Party/ACIS blob——Formats/Latest 已见 A3PartyObject 注册表）？ | go-topo 策略：直读 BRep vs 重放参数化 |
| 14 | **曲线/曲面基元序列化** | OdBmCurve 族（Line/Arc/Ellipse/NurbSpline/HermiteSpline）、OdBmProfile、扫描/放样/融合类 | 各基元字段布局（控制点/节点/权重/裁剪）——头文件给类字段，反编译给磁盘布局 | go-topo 线面转换 |
| 15 | **实例放置** | OdBmInstance/OdBmFamilyInstance | 变换(4x3)序列化格式与字段偏移 | 世界坐标定位 |
| 16 | **坐标基准** | OdBmBasePoint/OdBmProjectLocation（头文件为主）| 项目基点/测量点/共享坐标存储 | 地理配准 |
| 17 | **场景骨架** | OdBmLevel/OdBmGrid/OdBmCategory（头文件为主）| 标高/轴网/类别目录的字段与关联 | 场景层级建模 |
| 18 | A3Party(ACIS) blob 定位 | TB_Database 中 A3PartyObject 载荷的容器类 | 哪些元素带 SAT 几何、framing 格式 | 导入几何直读 |

## 执行建议

1. **先做 #5**（枚举表）：纯静态数据、可脚本化（nm/objdump dump 注册表），是 #3/#8/#9 的前置。
2. **#1+#2 是主战役**：建议从 TB_Loader ObjectStreamReader 栈引用的导入符号入手定位 ObjectPtrReader 所在模块，再顺 read 链展开。
3. **#13 早做**（哪怕粗粒度）：它决定 go-topo 对接是"转换器"还是"再生引擎"，影响整个工作量估算。
4. 每轮提取结果我方立即用双样例字节级验证（deposit/目录表/信封样本已就位，见 findings R24/R26–R29）。
