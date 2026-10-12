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

## P0/P1 提取轮（rea-extract-plan 驱动）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R30 | **★类索引勘误：i16 非 u32**——对象信封类索引 = `OdInt16Reader`（2B signed LE）符号扩展后查 `ClassesContainer::find`；分区实测 `76 05 00 00` 中 `76 05`=1398(OdBmElementHeader)，后两 `00 00` 属对象体首属性 | REA InitReader@0x281db6（0x281e4e 读 i16）+ csv 锚点 | 观察 | — |
| R31 | **对象信封调用链定案**（01-object-envelope.md）：ObjectPtrReader(薄)→OdBmObjectPtrReader(编排+DynamicQueue)→InitReader(i16 类索引→工厂)→per-class 生成 reader（快）/GenericObjectReader(慢：属性迭代+fieldType 分发)；引用两阶段=IdReference 挂起+initReferences 回填；动态属性段 readDynamicProperties | REA 六层 dossier | 观察 | [counter][size] 记录包装消费点仍在分区子流层[?] |
| R32 | **fieldType 磁盘格式表**（01-object-envelope.md §2）：1=bool/2=i8/3=i16/4=i32/5=i32/6=f32→f64/7=f64/8=string[布局?]/9=guid16B/10=i16/13=itemMode(0/6 标量,1 定长数组,5 变长)/14=对象引用(递归 i16+体)；跳转表@0x2b78c8；属性无磁盘头（顺序=类元数据）；typeId0x83=u8[] | REA PropertySingleValueReader@0x28358c + GenericObjectReader@0x2811ba | 观察 | String/Guid reader 内部布局（外部库）[?] |
| R33 | **类索引全表落地**（06-class-index.md + tables/*.csv）：2023=2845 类、2022=2761 类，index↔OdBm* 类名；1398(2023)/1340(2022)=OdBmElementHeader 跨版本重编号证实；**91/102/103 不在类索引空间**（系分区页/子流标签，修正计划 #3 假设）；值类型命名=ElementId/ElementIdPlusIndex/GUIDvalue | nm 生成 reader 符号批量提取 | 观察 | Classes 模块注册表未穷举（Readers 覆盖序列化类） |
| R34 | **几何存储模型定案（#13 决策）**：文件同时存参数化定义+**BRep 几何缓存树**——`Element.getGeometry`→`OdBmGElement(geometry cache)→GRep→GBrep/GEdge/GLine/GPoint…`；G 族全在类索引空间（GElement=2073、GBrep=2044、GEdge=1301、GLine=1798、GPoint=2098、GeomTable=2155、Geometry=2160）；A3Party*=外部导入容器；go-topo=**转换器读缓存树**，非再生引擎 | E1 BmElement.h:126/648、BmGElement.h、BmGeometry.h、BmGRep.h + csv | 观察→推断（决策） | G* 字段级序列化=下轮 REA |
| R35 | **ElemTable 布局定案**（02-elemtable.md）：6B 头（magic 0x2054055a）+ 28B×N 记录 `[id,id,0,depA,depB,0,0xffffffff]`；id 可跳号（删除）；depA/depB **非元素类索引**（551/767/822 对 csv 类名不合语义）——路由经 deposit/目录表串联 [?] | 样例 26 条连续+跳号实测 | 观察 | depA/B 语义、头尾字段[?] |
| R36 | **UnitSystem 定案（#4）**：Metric=0/Imperial=1（E1 BmUnitSystem.h）——表流 FixedFooter u32∈{0,1}；**实例变换**=Instance.getTransform()→OdGeMatrix3d（#15 头文件层）；坐标基准=BasePoint(Project/Shared)+GeoLocation（#16，与 oda-bim-clues R7 三坐标系一致） | E1 三头文件 | 观察 | 盘上字段布局随类信封[?] |
| R37 | **G 节点类型系统定案（#14a）**：TB_Geometry.tx 每类 propertiesConstructor 内 UTF-32 属性名按注册序提取（=序列化序）——GLine=OriginPoint+DirectionVector、GArc=X/YDir+Radius+Center+IsFilled、GEllipse=Center+X/YDir+RadiusX/Y、GNurbSpline=Degree+Closed+Rational+ControlPoints+Knots、GBrep=Faces、GFace→GEdgeLoop→GEdge 双向链表（InteriorEdgePnts/FirstAndLastEdgePnts）、GRep=GeomBBox/ObjectType/GRepFlags/GTightBBox/ElementId(i64)、GInstance=InstanceInfo+EmbeddedSymbolGRep+…、GRvtLink=InstanceInfo；产物 tables/g-node-properties.txt | objdump lea→UTF-32 串批量提取（29 构造器） | 观察 | 各属性 fieldType/itemMode[?]；GElement 壳构造器属性在别处[?] |
| R38 | **ElementId 值编码=经 DynamicQueue 的 objectReader**（非裸 u64）：c_valueReader<"ElementId"> lambda 调 objectReader<"ElementId">（队列 IdReference）→OdDbStub→OdBmObjectId；裸 id 宽度在 objectReader/IdReference 内[?] | E2 objdump 0x10b0d9（TB_LoaderBase） | 观察 | 裸 id 宽度、IdReference 磁盘格式[?] |
| R39 | **总纲落盘**：rvt-format.md（全景+信封契约+几何面+未知优先级）——R1–R38 收口索引 | 本文档集 | 观察 | — |
| R40 | **+353 页回收规则实用定案**：next_rec=member_start+ownLen−8；失败 span+=353 重试+32KB dict 链+0x0E47 重同步——2023 回收 133 页/14.8MB（+7.6MB）、2022 151 页/19.0MB；逻辑流 /tmp/rvt-partition-*.bin | 双样例恢复 walker | 观察 | 全 1060 页终结器回链[?] |
| R41 | 签名 64B=**非对称签名块**（非 sha 系内容哈希；RSA-512 尺寸；公钥未见）；串长34 校验 ✓ | 多候选哈希扫描 | 观察→推断 | 算法/公钥[?] |
| R42 | **ElementId 路由=OdBmDepositId(u64 Handle)×3 三元组**（splitElements/populatePartition/BmIODepositPE）；[counter][size] 字节循环在 PE 平台扩展内 | E1 BmDepositId.h + E2 nm/objdump | 观察 | PE 字节层[?] |
| R43 | **String=i32 长度+raw 字节；Guid=raw 16B 无前缀**（TB_Common.tx） | E2 objdump 0x29f58/0x29b54 | 观察 | String 编码[?] |
| R44 | **Variant::Type 用法表**（2=bool/5=i32/6=i64/7=f64/9=对象/13=ElementId/14=托管对象/263=f64[]/265=obj[]/270=链[]）；GInfoFlags 位域全表（E1）；**Formats/Latest=类型反射注册表**（非 ACIS 库）；**ProjectInformation=ZIP→Atom 项目参数 XML** | 构造器批量提取+E1 头+样例 | 观察 | NurbSpline 权重位置[?]；Variant→fieldType 映射[?] |

## M4 实现轮（报告落地 + 字节级新证）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R45 | **记录环字节层定案（R42 挂账收口）**：分区子流记录 = `[u32 counter][u32 size][body: size 字节][u32 size echo]`，stride=12+size。**仅 class-101（deposit 件流）成立**：2023 走出 77,251 条（99.95%=1398 OdBmElementHeader）、2022 58,819 条，回收后 walk 覆盖 100%；class-102/103 无此框式（102 头=counter 戳异型块、103=纯目录表，R28/R44 维持）；class-101 亦含少量 **class=0 记录**（无类 deposit 件，counter 正常递增）。真实记录 ≥91B——minRecordSize=16 门 + 同类链门抑制规律数据假链 | Go walker 双样例 100% 覆盖实测（objects.go） | 观察 | class-0 记录语义；偶发 resync 伪记录（39/77251 中约数条）[?] |
| R46 | **+353 回收落地（R40）**：stale-index 重分类（index 差为 353 倍数且解压成功→StaleIndex，数据入子流）——2023 逻辑流 64.4→**84.7MB**（169 页回收，corrupt 仅剩 2）、2022 53.9→**69.3MB**（137 页，corrupt 0）；unreadable 页不变（97/68，真损坏或魔数丢失） | rvt/partition.go + 双样例测试 | 观察 | 终结器回链扫描（unreadable 区）仍未做 |
| R47 | **ElemTable↔分区串联键实锤（R35 升级）**：ElemTable 尾部 19B footer 内嵌**分区末条 deposit counter**（2023=1098316，bytes.Contains 验证 ✓）；ElemTable id 域实测 1..1098316（首 id=1，非 R35 所记 26 起——前条目属页内局部序）；8276 条、id/id2 零失配 | m4_real_test 双测试 + footer 字节 | 观察→推断 | id↔counter 全表对拍（待对象信封全解码）；footer [u32 0x8d5] 段语义[?] |
| R48 | **M4 代码落地**：`rvt/classindex`（两版本 CSV 嵌入+Lookup）、`rvt/objects.go`（记录环 walker+类普查）、`rvt/elemtable.go`（6B 头+28B 记录）、`rvt/partition.go`（stale 回收+walk 集成+ReadPartitionWithLookup）、`rvt/rvt.go`（ElemTable/UnitSystem/ProjectEntries 元数据）；rvtinfo JSON 全暴露；合成+真样例测试全绿 | 本仓库 M4 提交 | 观察 | — |
| R49 | **★ElementHeader 静态 schema 全量定案（第 5 轮 REA，反汇编生成 reader）**：记录体 = `[i16 1398][i32 count][count×([i64][EID][i16][i32])][ElementId×6][i32 viewRules][i32 abFlags][i16 classDef][i32][i32][CondInt16×2][动态段]`—— setter 序：RegenHistory→Category/Family/OwnerView/DesignOption/UnplacedOwner/Misc 六个 ElementId→RejectedViewRules→AbFlags4Bytes→ClassDefinitionRef→2×CondInt16；**ElementId=裸 i32（null=-1，R38 裸宽收口）**；viewRules=视图类型位掩码（0x81a 系实测）；**快路径嵌套对象无 i16 类前缀**（InitReader 前缀仅慢路径）；CondInt16=[i32 v][i16]，v==0→6B / v≠0→≥8B 分支 | objdump CustomDirectReader<1398>::read@0xe8914a + OdBmCondInt16Reader@0x17474a + ElementId201120260Reader@0xeaf08 + StaticIntegerReader@0x174936（=i32） | 观察 | CondInt16 v≠0 分支精确长度[?] |
| R50 | **★元素类身份解锁（全量验证）**：ClassDefRef i16 = 元素真实类（经 classindex）——2023 普查 77,213/77,251 解出（GStyleElem=11964、CategoryElem=10500、CurveElem=7066、LinearDimString=6036、FontElem=4788、DBViewType=2775、SketchGrid/SketchPlane…）；2022 58,815/58,819（GStyleElem=11338、CategoryElem=10157…）。API：`ElementClassCensus` + SubStreamInfo.ElementClasses | Go 解码器 77K 记录全量跑通（elements.go + 测试） | 观察 | 4 条类名未解 + 34 条短体（损坏页残留） |
| R51 | ElementHeader 静态头合计 60–64B；每条记录其后跟**变长动态属性段**（实测 27–165B）——readDynamicProperties→readProperties（readProperty\<readPropertyToken\> 循环+resolveReferences 队列回填） | readProperties@0x1770b8 调用图 | 观察 | 段内格式=R52 |
| R52 | ~~动态段 token 解码器=位压缩 token 流~~ **已作废（见 R57）**：readPropertyToken@0x176468 的 2.8KB 内联体实为 DynamicQueue 队首弹出逻辑 | 并行会话 Ghidra dossier（R57 修正） | 观察 | — |

## M5 几何战役轮（第 6 轮 REA）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R53 | **★class-103=几何 deposit 流定案**：头部=18B 定形目录记录 `[u32 counter][i16 class][u16][u32 k][u16][u32 k]`（k=2 双样例恒定；检测用形状匹配而非类匹配）——**目录 counter 与 class-101 deposit/记录 counter 同域且 100% 联动**（2023: 15/15，含 1098306–1098316 deposit counters + 312/313/314/315 记录 counters；2022: 2/2 = 99585/1708）；目录类按版本/条目混合（2023: GeomStep(97)×15；2022: AnalyticalLinkGStep(251)×2——⚠️ 2022 csv 的 96=GeomStep 与流内 251 并存，Readers 模块双证 96/251 各归其类）；目录后=**G 对象区**（慢路径 i16 信封），2023 标记普查：GLine×17803、GElement×5868、Geometry×1652、GPoint×149、GNode×119、GEdge×67、GEdgeLoop×46、GRep×41、GBrep×33、GeomTable×30 | rvt/geometry.go + 双样例测试（联动 100%） | 观察 | G 对象间的分帧=各自 schema 自定界（未解） |
| R54 | **G* 生成 reader schema 链定案**（TB_Format2023Readers CustomDirectReader::read 逐类反汇编）：GNode(1303)←GInfo(obj)；GRep(2074)←GNode+GeomBBox(obj)+GTightBBox(obj)+ObjectType(i32)+GRepFlags(u32)(+ElementId)；GElement(2073)←GRep 基类内联；GPoint(2098)←GNode+Coord(obj)+Size(i32)+BorderSize(i32)+PointFlags(i32)（**读序≠propertiesConstructor 注册序**——BorderSize 在 PointFlags 前）；GLine(1798)←GCurve(1797)+OriginPoint(obj)+DirectionVector(obj)；GBrep(2044)←GNode+Faces(collection)；GEdge(1301)←GNode+Faces/Next/Prev(idref-i32)+InteriorEdgePnts(array\<OdBmEdgePnt\>)+FirstAndLastEdgePnts(array\<,2\>)+Flags(i32)；GEdgeLoop(1312)←GNode+CondInt16+idrefs+Envelope(obj)+Open(bool)。基类 read 内联在前（无 i16 前缀） | objdump 调用序列 ×6 类 + 2 基类 | 观察 | 各 OBJREF 目标类（Coord/OriginPoint/GeomBBox/GInfo）schema 待提取 |
| R55 | **G 对象=慢路径信封**：G 节点以 [i16 class][fields] 形式存在于 class-103 数据区（快路径无前缀规则不适用于跨类引用）——与 R49 快/慢路径二分一致 | 流内 i16 标记对齐扫描（GLine 首现 @492 等） | 观察 | OBJREF 递归解码后才能逐对象定界 |

## M5.5 G 数据库目录轮（第 7 轮探针）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R56 | **class-103 G 区内含 G 数据库目录层**：6B 像目 `[i16 class][u32 id]`（id 连续递增），2023 扫出 2,238 个 ≥4 条像目——含 GLine/GFilter 序列与 **BRep 拓扑目录（OdBmFace(1649)/OdBmEdge(1300) 交替 id 序列**，Faces 135–139 + Edges 135–145 = 每个 BRep 的面/边清单）；GElement 组按 deposit counter 键控（`[u32 counter][8B][i16 2073][u32 counter]…`）；**Geometry(2160) 对象内联 f64 三元组对**（bbox min/max，实测 -0.0038/0.0088 等英尺值）；2040=OdBmGArc 证实 | 字节探针 + ScanGCatalogRuns（rvt/geometry.go，id<2^20 噪音门） | 观察→推断 | 目录与对象 blob 的指向关系；GArc=2040 修正早期 04-g-classes 的类号假设 |

## 并行会话合并轮（第 8 轮；并行编号 R53–R57 与本台账撞号，重编号如下）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R57 | **★动态段定性修正（作废 R52 位压缩假设）**：readPropertyToken 实为 DynamicQueue 队首弹出器（节点延迟标志触发 ObjectPtrInitReader::read）；动态段=按队列序连续的**标准对象信封**（与静态 schema 同规则） | 并行会话 Ghidra dossier | 观察 | **字节框架仍未解**：本库实证（77,251 条 ElementHeader，静态头 60B 处首 i16 仅 74 条解析为类）表明动态段起点由 addProperty 队列序决定，非静态头定长偏移——需队列语义（DataKey/disposition）才能逐字节定位 |
| R58 | **BFI 接受表**：2011/12→{3}、2013/14→{6}、2015→{9}、2016→{9,10}、2017–19→{10}、2020/21→{13}、2022→{13,14}、2023–26→{14}（双样例 rev=14 ✓）；已实现 `bfi.CheckRevision`（未知版本放行） | 并行会话 + bfi_test | 观察 | rev≠14 布局差异仍需协同样例 |
| R59 | **NurbSpline 权重关闭**：ControlPoints = OdBmWPoint[] = {f64 weight, Point3d} = 4×f64 内联（无独立 Weights 属性） | 并行会话 | 观察 | — |
| R60 | **签名收口**：全 SDK 无验证路径（零字符串命中）——ODA 跳过签名块；算法属 Revit 侧（外部依赖坐实，升级 R41） | 并行会话全 SDK 字符串扫描 | 观察 | — |
| R61 | 样例边界坐实：机上仅两件官方样例；干净/协同样例为持续外部依赖 | 并行会话 | 观察 | — |
| R62 | **交叉印证 + ElemRec schema**：并行会话构造器属性序与本库 R49 字节 schema 完全一致；**ElemRec(1366@2023) reader schema = ObjectId(EID)→ElementHistory(871 嵌套)→PartitionId→OwningElementId(EID)**（PartitionId 字段存在——修正 R35 的"不含分区路由"假设）；但 ElemTable 28B 表项 depA/depB 取值离散（767/551/822/565，单分区文件），**28B 项更可能=ElemRecPointers(1367) 表而非 ElemRec 对象**——对应关系挂账 | 本库 objdump ElemRec::read@0xe53478 + ElemTable 实测 | 观察 | 28B 项 ↔ ElemRec 映射；PartitionId 取值域 |

## M6 动态段框架轮（第 9 轮；07-dynamic-queue.md 指导下的字节对齐）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R63 | **★ElementHeader 动态段字节框架破解**：body = `[静态头 60B][n×4B 动态值][3B 零垫][echo]`，n=(size−63)/4——77,251 条中 76,006（98.4%）纯 4B 框架吻合（2022: 92.3%）；**违例记录=含懒对象信封的动态段**（[i16 class][fields]，与 R57 懒节点模型自洽）；动态值=混合 i32/f32 原始 4B（GStyleElem 实测 0x3F6F0002=0.934 风格参数、零默认值、id 引用），n 按元素类型聚类（GStyle 6–9、Category 7–10、Curve 11–40，max 1940）；**每类 item 数呈 4B 步进离散谱**（尾部直方图 31/35/39/43…全奇数 ≡3 mod4）。API：DecodeElementHeader 返回 DynamicValues/EnvelopeTail | 统计直方图（尾部按偏移值分布 + 按类尾长谱）+ 全量框架验证（elements.go/测试） | 观察→推断 | 各 4B 值的属性归属（队列序 ↔ 属性名映射）；信封尾记录的逐对象解析 |

## M6.5 剩余边界收敛轮（第 10 轮探针）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R64 | **动态段扩展尾定性与分类器**：R63 的"违例"记录**不是对象信封**——是按元素类型变化的扩展数据：① DBViewPlan(229B 记录)尾部=**元素 id 序列**（312/313/375/9481/49552…前有 2B 短前缀，id 域与 ElemTable 吻合）=**视图→成员映射白拿**；② FamilySymbol(153/177B)尾部=**f64 坐标数组**（-0.0038/0.0088，插入点/bbox）；③ 内联 id 与动态值无关（内联=真值，动态 n=6 记录的值全 0=默认值）。API：TailKind 分类器（scalars/ids/floats32/floats64/mixed；id 扫描尝试 +2B 偏移、f64 扫描尝试 4 种对齐）+ ExtendedTail 结构标记；实测 DBViewPlan ids×1、FamilySymbol floats64×2 | 尾长按类分布谱（全奇数 ≡3 mod4）+ 分类测试 | 观察→推断 | 各类扩展尾的逐字段 schema（按需）；f32/f64 双关值（小整数与浮点位型重叠）标注 |

| R65 | **G 对象 blob 定界=下一战役入口（边界探察）**：G 标记在 class-102（GLine×13158/51MB）与 class-103（×17803/23.6MB）均出现——真对象 blob 与目录像目交错（@492 一带的 [i16 class][u32 id] 即目录项而非对象头），指向规则未解；GInfo 疑似=3282（GeomStep 目录 field12 恒值，未证实）。定位真 blob 需先解目录→对象指向 | 双子流标记普查 + 头字节走读 | 观察 | 目录→blob 指向、blob 内 base 类嵌套序 |

## M7 G 树战役轮（第 11 轮）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R67 | **★class-103 = 按元素 G 树流（M7 定案）**：G 区 = GeomStep 目录 + **按 deposit 键控的 G 树块** `[u32 counter][8B key][i16 2073 GElement][u32 counter]…对象序列…`（块体延至下一严格组起点；2023: 10,537 组/10,477 唯一 counter，89% 与 class-101 记录 counter 联动）。块内对象序列混合：**内联 GRep 族对象**（`[i16 class][GeomBBox 48B][GTightBBox 48B]`——**双 bbox 字节相同**为强判据；ObjectType/GRepFlags i32 对随后的 `ffffffff 00000000 ffffffff`+GInfoFlags(0x88004=DefaultModel) 尾）与**类型化子引用**（`[i16 class][u32 child-id]`，id 按类连续）。扫描收紧后：925 个 bbox **100% 合理**（实测样本 (-0.0475,-0.0197,0)..(0.0082,0.0278,0) 英尺），829 元素带世界包围盒。API：ScanGTrees/GTree/GRef + geom.Element.BBox + Model.Trees | rvt/geometry.go ScanGTrees + TestGTreesRealSamples | 观察→推断 | 双 bbox 相同=紧致时相等（非紧致可能不同——潜在漏检）；GArc=2040 修正记录；曲线/网格 payload 在 class-102（下一战役） |

## M9 几何 payload 轮（第 12 轮）

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R68 | **class-102 对象存储部分解码（M9 首收）**：存储 = deposit 键控块（头部 `[counter][0xf77f22cb][87=AbsCurveDriver][3107]`）+ 两类条目：**引用** `[i16 class][u32 id≥1]` 与 **内联实例** `[i16 class][u32 0][payload]`。**GLine 内联 payload = 6×f64（Origin 点 + 第二点）**——109 条提取成功（样例 (0,0,0)→(0.336,0.246,0) 英尺），合理性门（有限/幅值/非退化）；`Model.Curves` + `ScanGLines` 落地。GArc(2040) 假设布局（固定偏移 XDir/YDir/Radius/Center）不成立——reader 反汇编证实调用序 = GCurve 基类 → XDirection/YDirection（vcall OBJREF）→ f64 Radius → Center（OBJREF）→ bool IsFilled，payload 前段含嵌套对象前缀 | rvt/geometry.go ScanGLines + TestCurvesRealSamples + GArc reader 反汇编 | 观察→推断 | GArc/GPoint/PolyMesh/NurbSpline payload 布局（需 OBJREF 类 schema）；引用 id ↔ 内联实例的绑定（块框架） |

| R69 | **OBJREF 点/向量目标类线上形态定案**：Coord/OriginPoint/DirectionVector/XDirection/YDirection/Center 的目标 reader = `OdGePoint3dReader<double>::read`（TB_Common @0x2a068）= **3×doubleReader = 24B（x,y,z）内联、无类前缀**（float 变体 @0x2a0e2 同构）——快路径点/向量不写 [i16 类索引]。GPoint[0] 块（3 处全同字节）= PointElementGStep(3098/3099) 引用列表而非坐标点——GPoint 实例坐标在其 G 树/其他块。GArc payload 前段含逐实例可变的 GCurve/GNode 前缀（EndParams/GInfo），需锚定前缀对照轮 | objdump GPoint::read（setCoord 前 callq OdGePoint3dReader::read@plt）+ TB_Common 反汇编 + GPoint[0] 三处字节比对 | 观察 | GArc/GPoint 完整布局需锚定前缀分析 |

| R70 | **GArc 内容锚定解码（M9 二收）**：[GLine/GArc][0] 式头不能单独锚定（大量整数数据误配）——改用**内容锚定**：10×f64 窗口 = XDir 单位向量 + YDir 单位向量（dot≈0）+ Radius>0 + Center(3×f64)。`ScanGArcs` 落地：**2023 提取 3,972 条模型空间圆弧**（R=74.27/75.55 ft、C=(-39.86,22.33,-63.25) 等；单位 XDir/YDir 正交），`Model.Arcs` + ModelSummary.Arcs。NurbSpline(2094) 对象定位（degree=4 标志 `04 00 08 01`、knot 计数 69 可见、WPoint 4×f64 数组在尾）——数组帧解码待专项；PolyMesh(2104) 头已见 | rvt/geometry.go ScanGArcs + TestArcsRealSamples + NurbSpline/PolyMesh 探针 | 观察→推断 | NurbSpline/PolyMesh 数组帧；曲线/弧↔元素绑定（deposit 块键） |

| R71 | **deposit 块键与绑定边界（M9 收口）**：class-102 头部存在 deposit 块 `[counter][marker][87][3123]`（412B 小块，仅头部 3 块，counter=deposit 1098306/1098310/1098314 步进 4；3123 为 R68 误读修正）；主体 51MB 用 **G 目录 id 空间**（class-103 树引用的 [class][id]）而非 deposit counter。绑定尝试：`[GLine][idN][GLine][0][payload]` 对儿索引仅 21 项/覆盖树引用 4%——**ref→object 的 id 关联语义 = M10 边界**（需 deposit 写出器级分析或同内容差分扩大样本）。头字段勘误：3123（R68 的 3107 误读修正） | 块扫描 + 对儿索引探针 | 观察 | id 关联语义；主体存储组织 |

| R72 | **M10 探察（绑定语法复杂度定性）**：① class-102 存在**按元素 counter 键控的块**——counter 1049 四次出现中 @5536 为真键（前置 GInfo 尾 `ffffffff 00000000 0000`）、@1465320/@5679464 为他块数据（id 引用 [1354][3][2] 序列）、@2058480 为前块内（[1047][1492][123][5] 后）；② 块内容含 id 列表（476/467/888/690/694/680）+ f64 数据 + [GLine][0] 内联标记；③ GPoint[0] 块三处字节全同 = PointElementGStep(3098/3099) 引用列表（非坐标点）；④ NurbSpline(2094) 权重锚扫描不确定（[2094][0] 精确样本仅 3 个）。块文法（键 vs 数据区分）需写出器级分析 | 四点 pre16 上下文走读 + GPoint[0] 字节比对 + 权重锚扫描 | 观察 | 键/数据区分文法；NurbSpline 数组帧 |

## M8 对象模型声明轮

| ID | 结论 | 证据 | 置信度 | 边界/未知 |
|---|---|---|---|---|
| R66 | **对象模型层落地（go-topo 契约）**：`rvt/geom` 包声明全部对象类型——已填充：Element（id+类身份+六个 id 槽+flags）、View（视图成员表）、BRep（Face/Edge 拓扑骨架）、BBox/WPoint；声明待填（M7 blob 战役后填充）：Line/Arc/Ellipse/NurbSpline/PolyLine/PolyMesh/Instance（按 R54 reader schema 定形，WPoint=weight+Point3d 内联）。`Partition.Model(version)` 装配器 + Metadata.Model 计数摘要——2023 实测：**77,217 元素（100% 命名）+ 1 视图 37 成员 + 493 BRep 骨架**（如 42 面/103 边）；2022：58,819 元素 + 495 BRep | rvt/geom + rvt/model.go + TestModelRealSamples | 观察 | BRep 骨架分组为启发式（≤1024B 窗口聚合 Face/Edge 序列）；几何实例待 M7 |
| R53 | **BFI 修订接受表**（getBFIStreamRevision@0xa04ca，TB_Loader）：FileVersion→可接受 BFI revision 向量：2011/12→{3}、2013/14→{6}、2015→{9}、2016→{9,10}、2017–19→{10}、2020/21→{13}、2022→{13,14}、2023–26→{14}；双样例 rev=14 ✓；各 rev 字节布局待老样例 | E2 objdump 初始化器立即数 | 观察 | rev3/6/9/10/13 布局[外部样例] |
| R54 | **NurbSpline 权重关闭**：ControlPoints=OdArray<**OdBmWPoint**>；WPoint={f64 m_weight, OdGePoint3d}=**4×f64 内联**（非 OdBmObject，无独立信封） | E1 BmWPoint.h | 观察 | 数组 count 前缀=推 u32（itemMode） |
| R55 | **签名块=SDK 跳过**：全 .tx UTF-16 检索 "Data generated by Autodesk" 零命中——ODA 读取器不验证（与 walker 一致）；算法/公钥属 Revit 侧（外部） | E2 全 bin 扫描 | 观察（否定性） | 算法[外部] |
| R56 | **属性构造器名表与 R49 字节 schema 交叉印证**：ElementHeader setter 序（RegenHistory→六 ElementId→ViewRules→AbFlags→ClassDef→…）与 R49 反汇编序一致；ElemRec=ObjectId/History/**PartitionId(i32)**/OwningElementId/ElemIdSet/MRU；ElemTable=Elements[]/Graveyard[]/LastHandle(i64)…；机上无更多 .rvt 样例（干净/协同=外部确认） | E2 构造器提取 + mdfind | 观察 | — |
| R57 | **★U-R7 关闭：动态属性段=队列驱动的标准对象读，非位压缩 token 流**——readPropertyToken 实为 DynamicQueue **队首弹出器**（节点 0x48B：延迟标志 node+0x38≠0 时调 ObjectPtrInitReader::read 从流取值；否则用节点自带值/虚槽+0x80）；分发依据=节点内存字段（无流内判别子、无位运算、无 switch 表）；值回写 values[node+0x40]（idx≤1 非法、-1=不回写）；终止=readProperties 依队列空判定。**动态段格式=按队列序连续的标准对象信封**（慢路径 i16 类索引+属性环 / 快路径直读，同 R49 规则）；R52"位压缩"假设作废 | REA readPropertyToken@0x276468 dossier（51 CALL 全核：唯一流触点=InitReader::read） | 观察 | readProperties 终止判据细节[?]（低价值） |
| R58 | **★DynamicQueue 队列语义全解（07-dynamic-queue.md）**：①队列全景=this+0x28 IdRef链/+0x48 Prop链/+0x60 工作栈/+0x68 map<DataKey,OdArray>/+0xa0/+0xc0 null-key 登记；②**DataKey={u64 k0,u64 k1,u32 kind}=0x18B，map 排序仅用 k0/k1（kind 不参与）**；③ValueDisposition≥0x30B 运行时描述符全字段表（kind/key0/key1/value/idx/has_idx/flags/aux）；④Prop 节点 0x48B+IdRef 节点 0x30B（精简）全字段表；⑤addData/internalAddData=map 桶合并（Either tag@0/payload@8；单值 push_back 步长0x28 / 数组整体接管）；⑥resolveDispositions=盖章{ k0,k1}到登记节点+清表+map kNull 桶移交；⑦initReferences=+0x60 弱指针种子（与登记表无关）。**动态段盘上=弹出序标准对象信封；DataKey/Disposition/kind/flags 均运行时簿记不上盘** | REA 六函数 dossier（0x274608/0x274866/0x274dd0/0x274e68/0x2749ee/0x275c20） | 观察 | DataKey.k0/k1 业务语义随各读者构造点[?] |

## 关键未知（队列）

- U-R1 `62 19 22 05` 帧头字段语义
- U-R2 Global/* 8B 前缀 u32（0/1/5）语义
- ~~U-R3 Partitions/N 头与块布局~~ ✅ R23/R26/R27 定案（12B 头+页记录链+签名块+终结器）；N 编号规则仍[?]
- U-R4 Format{Year} class Index 全表 ✅ R33（tables/class-index-*.csv 嵌入 rvt/classindex）
- ~~U-R7 ElementHeader 动态属性段 token 位格式~~ ✅ **R57 关闭**：动态段=队列驱动标准对象读（非位压缩）；CondInt16 v≠0 分支长度仍[?]
- U-R5 协同（workshared）样例
- U-R6 ~~gzip 链逐成员边界与 65249 分页常数~~ ✅ R25/R26：页成员边界=页记录 ownLen-8；65249=原始页读长上限（含 +8 CRC 记账），非解压尺寸
