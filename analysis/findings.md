# RVT 结论台账（findings ledger）

格式：结论 → 证据 → 置信度 → 未知。R1/R2 历史条目在 go-dgn
`analysis/03-dgn/findings.md`（本包无 Revit 能力 / 需另行组件），此台账自 R3 起。

## SDK 与消费方（C0 输入）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R3–R10 | ODA BIM SDK 存在；读取管线/对象模型/坐标系/几何黑盒/版本门控（历史条目，已消化） | go-rvt `analysis/oda-bim-clues.md`（3D Repo FileProcessorRvt.cpp） | 观察（外部源码） | SDK 本体当时未见 → R11 已解 |
| R11 | **BimRv SDK 在库**：`archives/BimRv_lnxX64_11dll.lic`（Linux x64 ELF，2026，83 模块 not stripped；头文件4722个）；FileVersion=2011–2026 十六档，模块=TB_Loader/TB_LoaderBase/TB_Format{Year}Classes+Readers（+2026Writers）/TB_ModelerGeometry/TB_Domain 族 | E1 BmModuleNames.h / BmFileVersion.h；E2 bin 目录 | 观察 | — |
| R12 | **版本门控机制**：按 FileVersion 加载对应 Format{Year} 模块组；ElemTable 读取器分档 2011–2023 / 2024–2026（内部断点）；每版本 Classes 定义独立 class Index4Enum 编号（2026: ElemTable=1481、PartitionTable=3200…） | E2 nm TB_LoaderBase/TB_Format2026Readers | 观察 | 各年 Index 全表待批量提取 |
| R13 | **Loader 管线**：readFileInternal → readNonPartitions(表) + readPartitions(分区)，MT runner + LoadingManager/Reactor；流栈 = Ole2(CFB)→FixedHeader(0/8B)/ContentsHeader→GZip\|PKZip→PagedStream(65249)→Object/Partition 读取器→ZipChain(PartitionStreamVersion 6–9) | E2 nm TB_Loader.tx 全符号 | 观察 | 65249/4078 块尺寸对应[?] |

## 容器与元数据（样例实测）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R14 | **RVT=CFB 复合文档 CONFIRMED**：13 流/8 顶层名，2022/2023 样例同构；根 CLSID 空 | E3 olefile 两样例；go-dgn internal/cfb 根级流对拍通过 | 观察 | — |
| R15 | **流地图**：BasicFileInfo(TLV)、Contents(魔数帧+gzip，解压含"GLOBAL"+UUID)、Formats/Latest(整段gzip，ACIS A3Party 注册表)、Global/{ElemTable,Latest,ContentDocuments,History,PartitionTable,DocumentIncrementTable}(8B头+gzip链)、Partitions/N(元素主体,MB级)、ProjectInformation(ZIP)、TransmissionData(u32+UTF-16 XML v5)、RevitPreview4.0(魔数帧) | E3 各流头/解压实测；rvt-container.md | 观察 | 前缀u32语义、Partitions头、帧字段[?] |
| R16 | **压缩体系**：Global/*=8B 固定头+多成员 gzip（单成员≈4078B 块化）；Formats/Latest=0头 gzip；Contents/Preview=`62 19 22 05` 帧魔数+gzip；SDK 双压缩器 GZip/PKZip 与 FixedHeaderReader<0\|8>/ContentsHeaderReader 符号双证 | E3 gzip 扫描 + E2 模板符号 | 观察 | PKZip 分布、帧字段[?] |
| R17 | **BasicFileInfo=TLV 布局**（revision=14）：u32 rev+10B 保留 + u32字符数+UTF-16 串（版本/构建/路径/workshared/GUID/语言/CMVer/…）+ 文本 Part2 摘要（键名↔BmBasicFileInfo.h getter 全表对应）；纯 Go 可只读识别 | E3 双样例逐字段走读 + E1 BmBasicFileInfo.h + E2 getBFIPart2 符号；rvt-basicfileinfo.md | 观察→推断（个别字段语义） | tag=3、保留区、协同样例、尾部混合编码[?] |
| R18 | TransmissionData = u32 字符数 + UTF-16 XML（`<TransmissionData version="5">` 外部引用：Keynote/Assembly 表路径）；ProjectInformation = 标准 ZIP | E3 双样例 | 观察 | ZIP 内部条目[?] |

## 实现（M1/M2/M3 落地）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R19 | **M1 元数据层落地**：`internal/cfb`+`bfi`+`rvt.Open`+`cmd/rvtinfo`；双真实样例端到端绿 | go-rvt 代码+测试 | 观察 | Summary 混合编码 best-effort |
| R19b | **BFI 布局勘误**：u8 标记×2；Increments=UTF-16 串；Part2=ANSI CRLF+UTF-16LE | 双样例走读 | 观察 | u8 语义[?] |
| R20 | **M2 表层落地**：`internal/zg`+Global/*、Formats/* 解压+Contents 帧+工作集名；2023 全 7 表 | 代码+测试 | 观察 | 尾部语义[?] |
| R20b | **压缩体系勘误**：单成员+尾部（非 4KB 链）；大流 gzip footer 非标准（宽容校验）；Contents u32 非长度 | python+Go 双证 | 观察 | footer 真语义[?] |
| R21 | 2022 样例 Global/ElemTable deflate 真实损坏；**2023 样例 Partitions/13 页@519413 同类损坏、2022 Partitions/8 页@453435 同类** —— 两官方样例大二进制区均有局部损坏（"(1)"/"(2)" 重复下载件），**外部依赖：重新获取干净样例** | 页链走读+trial 恢复失败 | 观察 | 损坏来源（下载 vs 官方）[?] |
| R22 | 工作集名任意字符串 | 双样例 | 观察 | — |
| R23 | **M3 分区容器落地（REA+样例双证，M3.5 修订）**：`Partitions/<n>` = **12B 固定头 + 32B 页记录 ×(页+1) + gzip 页成员链 + 签名块 + 终结器**（旧"44B 头"= 12B + 首页记录）。真实页数：2023=1060（792 验证）/2022=972（767 验证）——早期"22/24 页"系 walker 在首个损坏区提前截断 | REA: readPartitions@0x1a6e62（openStorage"Partitions"+foreachStream+wcstol 流名→分区号）；PageReader@0x1d85ea（N 字节原始读+verifyAndCutCRC）；verifyAndCutCRC@0x197058 | 观察 | 头@4/@8 字段语义（869/817 亦见于签名块）[?] |
| R24 | **分区逻辑负载首区=103B deposit 记录**（双 counter 戳；Field91=91 恒定，Field1398=1398(2023)/1340(2022)——疑似按版本类索引）。★勘误（M3.5）：2022 **同样有** deposit（2 条，counter 99585/1708，不按 +1 递增）——旧"2022 无 deposit 布局"系截断+整流拼接伪象；记录内 A/B 块内距因版本微异（counter 复现于 @66 或 @70），但 103B 步长与 91@4/91@99 恒定 | 双样例子流实测 | 观察→推断 | 对象信封全长=ObjectPtrReader（外部库）待追 |
| R25 | 对象流读取栈（REA）：PagedStream(≤0xFEE1 块)→拼接→CompressedStream(FixedHeaderReader\<8\>+GZip)→ObjectPtrReader(外部)→FixedFooterReader=u32 UnitSystem 尾；表流按 9 条路径名分派（readStream@0x1a19ee 字符串表）。★M3.5 对照：分区页 ownLen(=压缩长+8) 上限吻合 0xFEE1=65249（u16 量级），页解压 ~131KB 无碍（65249 限制的是**原始页读长 N**，非解压尺寸） | REA ObjectStreamReader 栈分析 + 样例对照 | 观察 | ObjectPtrReader 内部（TB_Database?）未追 |
| R26 | **页记录=内联索引（M3.5 定案）**：每页 gzip 成员前紧贴 32B 记录 `[u16 markA][u32 prevLen][u16 markB][u32 kind][u32 counter][u32 ownLen][u32 payload][u32 class][u32 flag]`（LE）；ownLen=本页压缩长+8（+8 对应 verifyAndCutCRC 切除的 8B CRC 记录），prevLen=前页 ownLen；markA/markB 每文件恒定（2023: 0e47/0e4e；2022: 0de2/0de9，语义[?]）；**class(101/102/103)=子流标签——分区含 3 条交错分页的逻辑子流**（2023: 8.7/38.0/17.6MB；2022: 6.8/30.0/17.2MB）；kind 4=常规、5=小页、6/7=128K 对齐页[?]；counter 全程递减[?] | 双样例 170+ 页记录逐字段对照 + 索引链行走模拟（793/767 页 ownLen 精确吻合） | 观察 | payload(~90% 解压尺寸)、counter、kind 精确语义[?] |
| R27 | **签名记录与终结器（M3.5 定案）**：链中穿插 `[32B 签名记录(kind=0,ownLen=0)][156B 载荷=64B 哈希+u32 长度+UTF-16"Data generated by Autodesk® Revit®"+尾部(含 header@8 值与其-1)][32B 下一页记录(前 6B 不透明,余字段有效)]`（合计 220B 间隔；2023:158 处、2022:179 处，多跟随 class-103 页）；流尾=终结记录(prevLen=末页长)+64B 哈希+同款签名串+[header@8][0][0xFFFFFFFF]+零填充 | 双样例 220B 间隔逐字节走读 | 观察 | 哈希算法（64B，疑似 SHA-512/Ed25519）、签名记录触发规律（约每 2 个 class-103 页）[?] |
| R28 | **子流逻辑层（M3.5）**：class-101 子流头部=deposit 记录区；class-102 头=同 counter 戳异型记录（field2=0xf77f22cb/87[?]）；class-103 头=目录表记录（[类索引 0x821(2023)/0x7df(2022)] + 递增小 id 表项 4..N）；页数据=对象记录流（[u32 counter][u32 size][u32 1398/1340]…反复，1398 出现 600-1000 次/页） | 双样例子流拆分实测 | 观察→推断 | 记录区对象信封=ObjectPtrReader 领域 |
| R29 | **★R21 重大修正：分区"损坏"呈系统性 +353**——全部可解压异常页的索引长度差恒为 -353（2023: 168 页 -353 + 1 页 -706=2×353；2022: 137 页全部 -353），页脚 CRC/ISIZE 同步陈旧（ISIZE=旧尺寸），而解码输出与正常页结构完全一致（记录流形态、1398 计数正常）且解码不报错。结论：这些页被**原地重写（+353 压缩字节/+2082 解压字节）而索引/页脚未更新**（疑似 Revit 增量保存/deposit 追加行为或写出器缺陷），非随机下载损坏；另 97/68 页 inflate 失败（真损坏或魔数丢失）。解析器保守处理：仅验证页入子流数据，异常页显式报错（含差值） | 双样例全部异常页差值分布统计 + 损坏页数据结构对照 | 观察 | +353 字节内容定位、是否可在干净样例上消除[?] |

## 关键未知（队列）

- U-R1 `62 19 22 05` 帧头字段语义
- U-R2 Global/* 8B 前缀 u32（0/1/5）语义
- ~~U-R3 Partitions/N 头与块布局~~ ✅ R23/R26/R27 定案（12B 头+页记录链+签名块+终结器）；N 编号规则仍[?]
- U-R4 Format{Year} class Index 全表（可脚本从 Classes 模块提取；91/1398/1340/0x821/0x7df 已见实例）
- U-R5 协同（workshared）样例
- U-R6 ~~gzip 链逐成员边界与 65249 分页常数~~ ✅ R25/R26：页成员边界=页记录 ownLen-8；65249=原始页读长上限（含 +8 CRC 记账），非解压尺寸
