# ODA BimRv SDK 模块地图与 Loader 管线（C0 盘点）

## 证据源

- SDK：`go-dgn/ODA-Parse-DWGs-main/archives/BimRv_lnxX64_11dll.lic/`  
  （Linux x64 ELF，2026 版权头，`bin/lnxX64_11dll/` 83 模块，**not stripped**）
- E1：`BimRv/Include/Common/BmModuleNames.h`、`Base/BmFileVersion.h`、  
  `Database/Entities/BmBasicFileInfo.h`、`Loader/BmHostAppServices.h`
- E2：`nm -C` / `strings` 于 `TB_Loader.tx`（2.0MB）、`TB_LoaderBase.tx`、  
  `TB_Format2026Readers.tx`（40MB）、`TB_FormatCommonReaders.tx`

## §1 模块地图（BmModuleNames.h，E1 观察级）

| 模块                                   | 职责                                                                                              |
| ------------------------------------ | ----------------------------------------------------------------------------------------------- |
| `TB_Loader`                          | **文件加载主控**：OdBmFileLoader、Ole2(CFB) 封装、BasicFileInfo、压缩/分页流                                     |
| `TB_LoaderBase`                      | ElemTable 等基础记录（ElemTable0Writer；**ElemTable2011–2023 / 2024–2026 分档 Reader**）                  |
| `TB_Format{2011..2026}Classes`       | 各版本序列化类定义（**class Index4Enum 每版本独立编号**）                                                         |
| `TB_Format{2011..2026}Readers`       | 各版本读取器（2026 例：DocumentIncrement=idx1338、ElemTable=1481、PartitionTable=3200、ElementHistory=933…） |
| `TB_Format2026Writers`               | 写回（仅最新版本配 Writers）                                                                              |
| `TB_FormatCommon{Classes,Readers}`   | 跨版本公共记录（Workset 族、DocumentHistory、PartitionTableInterface…）                                     |
| `TB_ModelerGeometry`                 | BRep 几何内核（85KB 壳 + 上游库）；三角化                                                                     |
| `TB_Database/TB_Main/TB_Essential/…` | 对象模型（Domain 层，58MB Database）                                                                    |
| FileVersion enum（E1）                 | **2011–2026 十六档**，`ODBM_LATEST_FV=2026`                                                         |

⇒ **版本门控 = 按 FileVersion 加载对应 Format{Year}Classes+Readers 模块**  
（R5 的模块级机制）。ElemTable 读取器按 2011–2023 / 2024–2026 两档切换  
（内部格式断点在 2023/2024 之间）。

## §2 读取管线（TB_Loader.tx 符号，E2 观察级）

```
OdBmFileLoader::readFileInternal(OdBmOle2RootStorage, OdBmDatabase, …, LoadingReactor[])
  ├─ runWithCFL(...)                     # CFL 跟踪（写覆盖/协同？）
  ├─ readNonPartitions(db, Ole2Storage, StreamData)   # Global/* 等表（可 MT：OdBmMTRunner/STRunner）
  └─ readPartitions(db, Ole2Storage, PartitionData)   # Partitions/N（可 MT）
       经 OdBmLoadingManager + OdBmLoadingReactor 回调
```

### 流读取栈（模板符号还原的分层管道）

```
OdBmOle2Storage / OdBmOle2Stream        # CFB（BmOle2.cpp）
  → FixedHeaderReader<0|8> | ContentsHeaderReader   # 头前缀（8B=Global/*；0=Formats；魔数帧=Contents）
  → OdBmGZipCompressor | OdBmPKZipCompressor        # 双压缩编解码
  → CompressedStreamImplReader<OdBmCompression<…>>
  → PagedStreamImplReader<OdBmPagedStreamContext<PageReader, Reader, 65249u>>
                                        # 分页；常数 65249 [语义?]
  → BasicStreamReader | PartitionStreamDataReader | ObjectStreamReader
  → OdBmZipChainReader<PartitionStreamVersion::{6,7,8,9}>   # 分区流版本链
  → OdBmHistoryPostProcessor
```

其他符号证据：

- `compressBlock/decompressBlock`（自定义块压缩，独立于 gzip）
- `createElementStream`、`readHandleOrIdx`、`getSchemas`
- 源文件名：BmOle2.cpp、BmCompressors.cpp、BmStreamReaders/Writers.cpp、  
  BmAggregationStream.cpp、BmConstrainedStream.cpp、BmCommonStreamIO.cpp
- `getBFIStreamRevision(FileVersion)`：BFI 修订号按版本查表

## §3 与 go-rvt 实现的映射

| SDK 层                            | go-rvt 目标                                           |
| -------------------------------- | --------------------------------------------------- |
| OdBmOle2Storage                  | `internal/cfb`（可参考 go-dgn 实现重写；需支持子存储递归）            |
| BasicFileInfoReader + Part2      | M1 `bfi.go`（见 rvt-basicfileinfo.md 布局）              |
| FixedHeader + GZip 链             | M2 `gzchain.go`（8B 前缀 + 多成员 gzip）                   |
| ContentsHeader 帧（魔数 62192205）    | M2 `frame.go`                                       |
| Format{Year}Classes 的 Index4Enum | M3 前置：class index ↔ 类名表（可从 Classes 模块 RTTI/nm 批量提取） |
| PartitionStreamVersion ZipChain  | M3 按需                                               |
| ModelerGeometry                  | **不实现**（几何黑盒；纯 Go 只到元素枚举/包围盒）                       |

## 未知

- PageReader 65249 常数与样例 4078B gzip 块的对应关系
- PKZip vs GZip 两类流在容器中的分布（样例只见 gzip）
- Format 各年 Class Index 全表（可脚本批量提取，排队）
- OdBmLoadingReactor/CFL 事件语义（对只读解析非必需）
