# RVT 容器结构（C1）— 真实样例实测

## 证据源

- 样例：`~/Downloads/2023 racbasicsampleproject (2).rvt`（18MB，Autodesk 官方样例）、  
  `~/Downloads/2022 rstadvancedsampleproject (1).rvt`（14MB）
- 工具：Python olefile（venv `/tmp/olevenv`）；go-dgn `internal/cfb` 曾对拍通过  
  根级流（迁库时该测试已随 RVT 移出；go-rvt `internal/cfb` 落地后补回）
- SDK 交叉证据：`TB_Loader.tx` 符号（见 `bim-sdk-modules.md`）

## 结论

**RVT = OLE/CFB（Composite Document File V2）复合文档** —— C1 假设 CONFIRMED。  
根 CLSID 为空；流/存储清单两样例完全同构（13 流 / 8 顶层名）。

## 流目录（2023 样例，olefile 全量）

|       大小 | 路径                              | 编码（实测）                                                                                                              | SDK 对应（推断，见 bim-sdk-modules）                                         |
| -------: | ------------------------------- | ------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
|     2339 | `BasicFileInfo`                 | **TLV：u32 修订号 + 定长头 + u32字符数+UTF-16 串序列 + 文本 Part2**（见 `rvt-basicfileinfo.md`）                                      | `BasicFileInfoReader`（TB_Loader）                                     |
|      276 | `Contents`                      | 帧魔数 `62 19 22 05` + 头 + **gzip 成员**；解压得240B 含 UTF-16 `"GLOBAL"` + UUID —— 顶层目录索引                                    | `ContentsHeaderReader`+gzip（TB_Loader）                               |
|   165042 | `Formats/Latest`                | **从0偏移起整段 gzip**（FixedHeaderReader<0>）；解压4796B 含 C 串 `A3PartyAImage/A3PartyObject/A3PartySECImage`（ACIS 第三方几何格式注册表） | CompressedStream(GZip, hdr0)                                         |
|   533587 | `Global/ContentDocuments`       | **8B 前缀（u32=1）+ 多成员 gzip 链**（单成员压缩≈4078B）                                                                           | CompressedStream(GZip, hdr8)                                         |
|      764 | `Global/DocumentIncrementTable` | 8B 前缀 + gzip；解压含 UTF-16 用户名（保存历史）                                                                                   | OdBmDocumentIncrementTable reader                                    |
|    42732 | `Global/ElemTable`              | 8B 前缀 + gzip 链；解压22078B+（元素表）                                                                                       | `ElemTable2011..2023/2024..2026Reader`（版本分档！）                        |
|    14081 | `Global/History`                | 8B 前缀（u32=1）+ gzip；解压含 FILETIME×3                                                                                   | OdBmElementHistory/DocumentHistory                                   |
|    89046 | `Global/Latest`                 | 8B 前缀（u32=5）+ gzip 链                                                                                                | 主"latest"数据                                                          |
|      137 | `Global/PartitionTable`         | 8B 前缀 + gzip；解压87B 含 UTF-16 `"Workset1"` + FILETIME                                                                 | `OdBmPartitionTable` reader（class index 3200@2026）                   |
| 17848032 | `Partitions/13`                 | **12B 固定头 + 32B 页记录内联索引 + gzip 页成员链 + 签名块 + 终结器**（M3.5 定案，见下节）；**元素主体全部在此**                     | `readPartitions` + `OdBmZipChainReader<PartitionStreamVersion 6..9>` |
|     1000 | `ProjectInformation`            | **标准 ZIP**（`PK\x03\x04`）—— 项目参数                                                                                     | 独立路径                                                                 |
|    12880 | `RevitPreview4.0`               | 同 `62 19 22 05` 帧魔数 + gzip（预览图）                                                                                     | Contents 同族帧                                                         |
|     2792 | `TransmissionData`              | **u32 字符数(1394) + UTF-16 XML**：`<TransmissionData version="5">` 外部引用清单                                              | DocumentIncrement 的 stream revision 跟踪                               |

2022 样例差异：`Partitions/8`（vs 13）、各表大小不同；**流名集合一致**。

## 帧/压缩体系（样例实测 + TB_Loader 符号双证）★M2 定案（含勘误）

1. **魔数 `62 19 22 05`**：Contents/RevitPreview 的帧头。Contents = 若干
   `magic + u32(语义[?]，非载荷长度！2023=181/2022=260 均大于实际剩余) + gzip 成员(+尾部至下一 magic)` 记录；
   首条记录为外层头（2023:26B 域），第二条解压得 "GLOBAL" 索引（名称+GUID+build 串）。
   ★勘误：早期"u32=载荷长度"模型错误（2022 上会越界）——解析改为解压优先。
2. **Global/* / Formats/\* = 头(0 或 8B) + 单个 gzip 成员 + 变长尾部(39–677B，前导零+高熵块，语义[?])**。
   ★勘误：早期"≈4KB 多成员 gzip 链"系 4096B 读截断假象——实为单成员。
   头部 u32 观测值：0(ElemTable/PartitionTable)/1(History/ContentDocuments/IncTable)/5(Latest)。
3. **gzip footer 非标准（重要）**：大流（Latest、Formats/Latest、ContentDocuments，
   2022+2023 双文件一致）的 CRC32/ISIZE 与内容不符（python gzip 与 Go 同报错）——
   Revit 写出器不更新/写错 footer；**deflate 本体有效**。解析：接受数据，
   记录 FooterCRCOK/FooterSizeOK。小流（ElemTable/History/PartitionTable/IncTable）footer 合法。
4. **PartitionTable**（解压后）：GUID + u32 组 + **工作集名**（len 前缀 UTF-16；
   2023="Workset1"，2022="Project Standards"——名称任意，勿按前缀过滤）。
5. **双压缩器**：符号见 `OdBmGZipCompressor` 与 `OdBmPKZipCompressor` 两条模板路径
   （样例只见 gzip；PKZip 分布未知）。
6. **PartitionStreamVersion 6/7/8/9**：分区流格式带版本号的链式读取器
   （`OdBmZipChainReader<…>`），跨 Revit 版本演进。

## 分区流布局（M3.5 定案，双样例逐字节 + REA 双证）

`Partitions/<n>`（两样例均 v9）：

```
12B 固定头      u32 版本(9) + u32 0 + u32 field8(869/817，亦现于签名块尾部)
32B 页记录 ×N   [u16 markA][u32 prevLen][u16 markB][u32 kind][u32 counter]
                [u32 ownLen][u32 payload][u32 class][u32 flag]  (LE)
gzip 页成员     ownLen = 成员压缩长 + 8（verifyAndCutCRC 切除 8B CRC 记账）
签名块(220B)    [32B 签名记录 kind=0/ownLen=0][156B 载荷：64B 哈希 +
                u32 长度 + UTF-16 "Data generated by Autodesk® Revit®" +
                尾部(含 field8 与 field8-1)][32B 下一页记录(前 6B 不透明)]
终结器          [32B 终结记录 prevLen=末页长][64B 哈希][签名串][field8][0]
                [0xFFFFFFFF][零填充至流尾]
```

- 页记录紧贴其成员**之前**；markA/markB 每文件恒定（2023: 0e47/0e4e；
  2022: 0de2/0de9）[?]；**class=子流标签**（101/102/103）——分区含 3 条
  交错分页的逻辑子流，按类重组后各自独立（class-101 头部=deposit 记录区，
  class-103 头部=目录表）。
- 真实页数 2023:1060 / 2022:972；验证页（页记录 ownLen 精确吻合 + gzip
  footer 校验通过）792/767；其余为 +353 系统性索引陈旧页（R29）或
  inflate 失败页——解析器三态分类并显式报错。
- 签名块 2023:158 处 / 2022:179 处（多跟随 class-103 页，约每 2 页一处）[?]。

## 未知（C1 残留）

- `62 19 22 05` 帧头各字段语义
- Global/* 8B 前缀 u32 语义（0/1/5）
- `Partitions/N` N 编号规则（分区计数？）；头@4/@8 与 markA/markB、页记录
  kind/counter/payload 字段语义
- +353 系统性页重写的机理与内容（R29）
- ProjectInformation ZIP 内部条目清单（未解包）
- Formats/Latest 的 ACIS 格式注册表逐项语义
