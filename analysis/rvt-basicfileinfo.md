# BasicFileInfo 流布局（C2）— 样例实测 + SDK 头文件对映

## 证据源

- 样例：2023 racbasic（2339B）/ 2022 rstadvanced（2401B），`~/Downloads/`
- E1：`BimRv/Include/Database/Entities/BmBasicFileInfo.h`（getter 全表）
- E2：TB_Loader.tx 符号 `BasicFileInfoReader/Writer`、`createBimRvBasicFileInfoPart2`、
  `getBFIPart2`、`getBFIStreamRevision(FileVersion)`

## 布局（两样例一致，revision=14）★实测定案（含勘误）

```
u32  revision            # 实测=14；SDK: getBasicFileInfoStreamRevision / BFIStreamRevisionMap
u64  zero                # [?] 保留
u16  zero                # [?] 保留
--- TLV 字符串区：u32 字符数 + UTF-16LE（无 NUL）---
str  SavedInVersion      # "2023" / "2022"
str  Build               # "20220401_1515(x64)" / "20210129_1515(x64)"
str  LastSavePath        # 完整 .rvt 路径
u32  tag=3               # [?]（两样例恒3）
u8   worksharedFlag      # 0=非工作共享；SDK: getIsWorkshared/getWorksharedFlags
u32  36
str  CentralModelIdentity GUID   # 非工作共享时全零 GUID
str  Language            # "ENU"；SDK: getLanguageWhenSaved
u8   marker1=0           # ★勘误新增：Language 与 CMVer 之间恒 0 字节 [语义?]
u32  CMVerNumCorrToTheLRelLatest # 2023=14 / 2022=9（与文本文本段吻合）
u32  36
str  CMEpGUIDCorrToTheLRelLatest
u32  36
str  UniqueDocumentGuid  # 2023: e20d7047-90b6-4dd5-a092-73b36e9d365a
str  UniqueDocumentIncrements    # ★勘误：非 u32！u32字符数+UTF-16 十进制串（"14"/"9"，值==CMVerNum）
u32  36
str  ModelIdentity       # 样例全零
u8   marker2=0           # ★勘误新增：ModelIdentity 与 Author 之间恒 0 字节 [语义?]
str  Author              # "Autodesk Revit"（14）
str  ClientAppName       # "RevitApplication"（16）
--- 文本 Part2（getBFIPart2 / createBimRvBasicFileInfoPart2）---
0d 0a                    # ANSI CRLF 前缀
UTF-16LE 人类可读摘要，行式 "Key: Value\r\n"，键名与 BmBasicFileInfo.h getter 一一对应：
  Worksharing / Username / Central Model Path / Format / Build / Last Save Path /
  Open Workset Default / Project Spark File / Central Model Identity /
  Locale when saved / All Local Changes Saved To Central /
  Central model's version number … / Central model's episodic GUID … /
  Unique Document GUID / Unique Document Increments / Model Identity /
  IsSingleUserCloudModel / Author / ClientAppName
注意：摘要内偶发嵌入 ANSI 段（如 "False" 原始字节），纯 UTF-16 解码局部乱码
（go-rvt Summary 为 best-effort；结构化字段不受影响）。尾部以 0d 0a 结束。
```

## 字段 ↔ SDK getter 对映（E1）

| 样例字段 | `BmBasicFileInfo.h` getter |
|---|---|
| SavedInVersion | `getSavedInVersion` / `getFileVersion`（enum 2011–2026） |
| Build | （文本段 Build；二进制段即 build 串） |
| LastSavePath | `getLastSavePath` |
| worksharedFlag | `getIsWorkshared` / `getWorksharedFlags` |
| CentralModelIdentity | `getCentralModelIdentity`（OdGUID） |
| Language | `getLanguageWhenSaved` |
| CMVerNum… | `getCMVerNumCorrToTheLRelLatest` |
| CMEpGUID… | `getCMEpGUIDCorrToTheLRelLatest` |
| UniqueDocumentGuid | `getUniqueDocumentGuid` |
| UniqueDocumentIncrements | `getUniqueDocumentIncrements` |
| ModelIdentity | `getModelIdentity` |
| Author | `getAuthor` |
| ClientAppName | `getClientAppName` |
| 文本段各项 | `getUserName/getCentralModelPath/getFormat/getOpenWorksetDefault/getIsProjectSparkFile/getIsAllLocalSavedToCentral/getIsSingleUserCloudModel/…` |

## 识别用途（M1 即可交付）

- `revision`（14）+ `SavedInVersion`（"2023"）→ 版本识别与门控（R5）
- Build 串 → 精确到 Revit build
- GUID 族 → 协同模型身份（中心/本地/唯一文档）
- **纯 Go 最小可用**：读 CFB → 取 BasicFileInfo → TLV 走读即可输出 JSON 元数据

## 未知

- 前导 u64/u16 保留区语义；tag=3 语义
- 尾部 8-bit 段与 UTF-16 段并存原因（Part2 双写？）
- revision >14 的未来布局（BFIStreamRevisionMap 按 FileVersion 维护 [推断]）
- 协同（workshared=1）样例未覆盖 → Username/CentralModelPath 非空路径待样例
