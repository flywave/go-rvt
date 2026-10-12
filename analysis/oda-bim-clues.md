# ODA BIM SDK 消费方代码 — RVT 线索分析

## 证据源

- **文件**：`../evidence/external/FileProcessorRvt.cpp`（3D Repo Ltd，AGPL v3，2018；  
  来源=用户提供的 bim-ex/模型转换器源码，头注释 `//ODA [ BIM BIM-EX RX DB ]`）
- **证据等级**：**E1-类外部源码** —— ODA BIM SDK（BimRv 产品线）的**消费方**，  
  非 SDK 本体。角色等同于 DGN 侧的 GDAL `ogrdgnv8layer.cpp`（D55：ODA API 消费方）。
- **边界**：**零字节级格式信息**（无偏移/流名/编码）；价值 = 对象模型语义 +  
  读取管线结构 + 新取证路线的模块名线索。

## 结论台账

| ID  | 结论                                                                                                                                                                                                                                                                                                                                                                   | 证据                                                                  | 置信度      | 边界/未知                                       |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- | -------- | ------------------------------------------- |
| R3  | **ODA 存在独立 BIM SDK 产品线**（BimRv/BIM-EX），API 前缀 `OdBm*`，可 `readFile(.rvt)` → `OdBmDatabasePtr`；与本包 Teigha 25.4 无交集（R1 边界再确认）                                                                                                                                                                                                                                           | FileProcessorRvt.cpp 头注释 + 全部 `#include <…Bm…>` + `svcs.readFile()` | 观察（外部源码） | SDK 本体版本/授权渠道未知                             |
| R4  | **读取管线与 ODA DWG/DGN 模式同构**：`odrxInitialize(services)` → `loadModule(OdBmLoaderModuleName)` → `addX(OdBmSystemServicesPE)` → `readFile`；**几何内核独立成模块**（`OdBmModelerModuleName` / `BmModelerModule`，三角化参数 `wrTriangulationParams`）                                                                                                                                      | readFile() 全流程 + setTessellationParams                              | 观察       | Loader/Modeler 对应的 DLL 文件名与内部结构未知           |
| R5  | **版本门控存在**：`OdError eUnsupportedFileFormat` → 单独错误码 REPOERR_UNSUPPORTED_VERSION —— 新 Revit 版本被旧 SDK 拒读（与社区认知一致：RVT 版本锁定）                                                                                                                                                                                                                                             | readFile() catch 分支                                                 | 观察       | 版本↔SDK 对应表未知                                |
| R6  | **对象模型镜像 Revit API**：ViewTable/`DBView3d`（DisplayStyle 4 值：Wireframe/HiddenLine/Shaded/ShadedWithEdges、DetailLevel、Discipline、`BuiltInParameter.VIEW_PHASE_FILTER`）、`OverrideGraphicSettings`、`ODBM_BUILTIN_CATEGORIES` 宏表（KIND 分类 Annotation/Model/Internal/AnalyticalModel；`OST_*` 枚举名与 Revit API 完全一致）、UnitsTracking→UnitsElem→AUnits→FormatOptions、Transaction 宏 | createDefault3DView / setupViewOverrides / setupUnitsFormat         | 观察       | 枚举全表在 SDK 头文件（`BmBuiltInCategory.h` 等）中，本地无 |
| R7  | **三坐标系模型**：Internal Origin / Project Base Point / Survey Point；模型→共享坐标 = `GeoLocation::getActiveLocationId().getTransform()` 取逆；几何在 model space 用 **Revit 内部单位**；变换含项目单位换算                                                                                                                                                                                           | getModelToWorldMatrix + getModelBounds 注释                           | 观察       | 内部单位的精确标度（英尺×304.8？）未在代码中出现                 |
| R8  | **几何提取 = Graphics Services 矢量化管线**：自定义 `OdGsBaseVectorizeDevice`/`OdGsViewImpl` 子类（VectoriseDeviceRvt/DataProcessorRvt）经 `pDevice->update()` 回调收集；**BRep 三角化封在 Modeler 内部**（surfaceTolerance/normalTolerance LOD 参数）；元素级仅 `getBBox()` 可直读                                                                                                                            | StubDeviceModuleRvt + createDevice/update + LOD_PARAMETERS          | 观察       | 三角化算法/网格格式=SDK 黑盒                           |
| R9  | **大模型内存模型**：`OdBmSystemServicesPE` 提供 unload-to-disk（setUseUnload/useDisk/unloadFilePath/indexingRate）—— RVT 模型可部分卸载到磁盘，暗示文件本身为**可分段懒加载的复合容器**（与路线 C1 的 CFB 假设相容，但本代码不构成 CFB 直接证据）                                                                                                                                                                                   | RepoRvtServices 类                                                   | 推断       | CFB 流布局仍需真实样例（C1 不变）                        |
| R10 | **模块名线索（新路线关键）**：`OdBmLoaderModuleName`、`OdBmModelerModuleName` 为独立可加载模块 —— 若能获取 ODA BIM SDK 的 bin/include，可复刻本项目对 TG_Db 的 REA/Ghidra 分析方法论                                                                                                                                                                                                                          | 两处 loadModule 调用                                                    | 观察→推断    | 未见过实际 DLL；文件名/导出表未知                         |

## §1 读取管线（对照 DGN）

```
odrxInitialize(RepoRvtServices)                    # services = OdExBimHostAppServices + OdBmSystemServicesPE
  → loadModule(OdBmLoaderModuleName)               # 文件 I/O 模块（对应 TG_Db 的角色）
  → addX(OdBmSystemServicesPE)                     # 扩展注册（unload 能力）
  → loadModule(OdBmModelerModuleName)              # 几何内核（三角化；对应 DGN 无此层——BRep 是 Revit 特有）
  → setTriangulationParams(wrTriangulationParams)  # LOD：bNewTess / surfaceTolerance / normalTolerance
  → svcs.readFile(filename) → OdBmDatabasePtr      # 主入口；版本不支持→eUnsupportedFileFormat
  → ViewTable.findViewIdByName / createDefault3DView
  → view.getVisibleDrawableElements → element.getBBox()   # 元素枚举+包围盒
  → OdGs device.update() → 自定义 Vectorize 回调    # 全量几何=三角化网格流
```

与 DGN 对照：DGN 是 `OdDgDatabase::readFile`（TG_Db 内一体）；RVT 把 **Loader 与  
Modeler 拆成两个模块**，且几何不是实体字节解码而是 BRep→三角网格的转换。

## §2 对象模型 → Revit 概念映射（Go 设计参考）

| OdBm API                                                     | Revit 概念                        | 备注                                                          |
| ------------------------------------------------------------ | ------------------------------- | ----------------------------------------------------------- |
| `OdBmDatabase`                                               | Document                        | `getAppInfo(ManagerType::…)` 取各类管理器                         |
| `OdBmViewTable` / `OdBmDBView3d`                             | View/View3D                     | DisplayStyle/DetailLevel/Discipline/PhaseFilter             |
| `OdBmDBDrawing`                                              | 图纸容器 → ViewFamily 实例            | 注释："mostly empty container that points to View Family type" |
| `ODBM_BUILTIN_CATEGORIES` 宏 + `OdBm::BuiltInCategory::OST_*` | BuiltInCategory（与 Revit API 同名） | KIND=Annotation/Model/Internal/AnalyticalModel              |
| `OdBm::BuiltInParameter::VIEW_PHASE_FILTER`                  | BuiltInParameter                | setParam 通用参数访问                                             |
| `OdBmOverrideGraphicSettings`                                | OverrideGraphicSettings         | 可见性/图案覆盖                                                    |
| `OdBmUnitsTracking/UnitsElem/AUnits/FormatOptions`           | 单位与显示精度                         | accuracy/rounding                                           |
| `OdBmElementTrackingData.getElementsByType(catId, …)`        | 按类别枚举元素                         | ProjectBasePoint 即用此法找到                                     |
| `OdBmBasePoint` / `OdBmGeoLocation`                          | 项目基点/地理定位（Survey Point）         | 三坐标系                                                        |
| `ODBM_TRANSACTION_BEGIN/END`                                 | Transaction                     | 任何写操作需事务                                                    |
| `OdBmBasicFileInfo`（include 出现，未调用）                          | 基本文件信息（版本/时间戳）                  | **C2 路线的 SDK 对应物**                                          |

## §3 对 04-rvt 路线图的影响

1. **新增路线 C0（可行性取决于 SDK 获取）**：设法取得 ODA BIM SDK  
   （商业授权或试用）的 include + bin →
   - E1：`Bm*.h` 头文件 = 对象模型全表（类比本项目用 Teigha 头钉 DGN 对象模型）；
   - E2：对 BimLoader/BimModeler DLL 做 REA/Ghidra 反编译（复刻 TG_Db 方法论，  
     含 filer 原语/模块名/导出表）。**这是 RVT 内部格式唯一与 DGN 同深度的取证路径。**
   - 前置线索：3D Repo bim-ex 的构建配置/依赖清单可指出所需模块全名。
2. **C1（CFB 容器）/ C2（BasicFileInfo 元数据）不变**：仍需真实 .rvt 样例；  
   本代码只增加"元数据层 SDK 对应物存在"的旁证（R6 的 BasicFileInfo include）。
3. **C3（模型数据层）定性收紧**：几何 = Modeler 内 BRep 三角化黑盒（R8），  
   纯 Go 重现完整几何**不可行**；纯 Go 现实目标 = 识别 + 版本 + 元数据  
   （+ 视 C0 进展而定的元素级信息）。
4. **版本门控（R5）** 入识别层设计：探版本时需预期"更新版本拒读"语义。

## §4 未知与边界

- ODA BIM SDK 本体（头文件/DLL）未见 —— R10 的 C0 路线待启动条件（获取 SDK）。
- RVT 容器是否 CFB：本代码无直接证据（R9 仅"相容"）；维持 C1 待样例。
- `OdBmBasicFileInfo` 流布局、`PartAtom` 等：C2 待样例。
- 本代码为 2018 年版本，对应 ODA BIM SDK 年代与 Revit 版本范围未知；  
  现行（2024+）SDK API 可能有出入。
