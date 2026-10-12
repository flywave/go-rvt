# 对象信封与值解码链（P0 #1/#2）— TB_LoaderBase 反编译定案

证据：REA/Ghidra 12.1.4 对 `TB_LoaderBase.tx`（sha256=1a8bf7f5…）逐层反编译；
dossier 存于 opencode tool-output（会话内）；类索引表 `tables/class-index-{2022,2023}.csv`。
地址均为 Ghidra 地址（= nm 地址 + 0x100000）。

## 0. 调用链总览（自顶向下）

```
ObjectStreamReader (TB_Loader, 表流/分区共用外层)
  → PagedStream(≤0xFEE1 块) → 拼接 → CompressedStream(FixedHeaderReader<8>+GZip)   [表流]
  → ObjectPtrReader::read @0x283026            # 薄包装：换 vptr → OdBmObjectPtrReader
    → OdBmObjectPtrReader::read @0x2823fc      # 编排器：DynamicQueue + reactor 钩子
      → OdBmObjectPtrInitReader::read @0x281db6 # ★信封：读 i16 类索引 → 工厂
        → ClassesContainer::find(u32)           # i16 符号扩展
        → readersFactory(cls) → per-class 生成 reader（快路径，Format{Y}Readers.tx）
        或 OdBmGenericObjectReader::read @0x2811ba  # 慢路径：属性迭代驱动
          → 每属性: typeId==0x83→u8[] ；否则按 getLoadingMode:
             0/6→ItemModeProcessorReader<OdTfVariant>::read @0x2876fe（标量）
             1→定长数组(getSize) / 5→变长数组(count=-1) → CollectionReader @0x206ba6
             1/2/4(CondInt16 语境)→ItemModeProcessorReader<OdBmCondInt16> @0x286e3c
             3→ItemModeProcessorReader<int> @0x287074
          标量叶子 = OdBmPropertySingleValueReader::read @0x28358c（getFieldType 分发）
      ← OdBmDynamicQueue::initReferences(bool, obj) @0x275c20   # 引用两阶段回填
      ← OdBmDynamicQueue::readDynamicProperties(stream) @0x2775e2
  → FixedFooterReader = u32 UnitSystem 尾（表流）
```

## 1. 信封关键定案

| # | 结论 | 证据 |
|---|---|---|
| E1 | **类索引 = i16（2B signed LE）**，非 u32！读取器 `OdInt16Reader`；符号扩展为 u32 后 `ClassesContainer::find`；查不到抛 OdError(0x128) | InitReader@0x281e4e-0x281e64 |
| E2 | 仅当 context 缓存类指针（this+0x48）为 null 时才读类索引——**嵌套子对象可携带预解析类** | InitReader@0x281dd0 |
| E3 | 类索引→对象体读取 = **每类生成 reader**（`__ODA_BimRv__internal_read_function<CustomDirectReader<…Index4Enum,LjN…>>`，在 TB_Format{Y}Readers.tx，2023 年 2845 类）；无生成 reader 时走 GenericObjectReader 慢路径 | readersFactory@0x1e1f34 + Readers 模块 nm |
| E4 | 对象体 = **属性序列驱动**（类描述符迭代器 done/next），无 per-attribute 磁盘头——属性集与顺序完全由类元数据（编译期）决定，盘上无属性名/编号 | GenericObjectReader@0x2811ba |
| E5 | 基类 OdBmObject 的属性**跳过不序列化**；typeId==0x83 的属性读长度前缀 u8[] | 同上 |
| E6 | 引用/子对象 = fieldType 14 → **递归 ObjectPtrInitReader**（内含 i16 类索引）；动态队列两阶段：先挂 IdReference，读完后 `initReferences` 按 DataKey 回填 | PropertySingleValueReader case14 + DynamicQueue 析构签名(list\<IdReference\>) |
| E7 | 引用命名空间：`c_valueReader` 值类型 = **ElementId**(OdBmObjectId)、**ElementIdPlusIndex**(pair\<ObjectId,int\>)、**GUIDvalue**(OdGUID) —— CompileTimeString 提取 | nm TB_LoaderBase |

## 2. fieldType 分发表（属性值磁盘格式核心表）

分发键 = `OdBmRuntimeProperty::getFieldType()`（u32），跳转表 @0x2b78c8（15 项 rel32）：

| fieldType | 磁盘读取 | 宽度 | 装配 | 备注 |
|---|---|---|---|---|
| 1 | boolReader | 1B | bool | |
| 2 | OdInt8Reader | i8 1B | int8 | |
| 3 | OdInt16Reader | i16 2B LE | int16 | 与 10 共用 reader（语义差=符号/枚举 [?]） |
| 4 | OdInt32Reader | i32 4B LE | int32 | 与 5 共用 reader |
| 5 | OdInt32Reader | i32 4B LE | int32 | 同上 |
| 6 | floatReader | f32 4B → 提升 f64 | double | |
| 7 | doubleReader | f64 8B LE | double | |
| 8 | OdStringReader | 长度前缀+编码 [?]（reader 在外部库） | string | |
| 9 | OdGUIDReader | 16B [字节序?] | guid | 唯一不经 OdVariant 直构 |
| 10 | OdInt16Reader | i16 2B | int16 | 见 3 |
| 13 | ItemMode 路径 | 条件/数组（itemMode 0/1/5/6 调度，见 §3） | variant | getRefPropertyInfo 参与 |
| 14 | ObjectPtrInitReader | i16 类索引 + 对象体（递归 §1） | object ref | 可选 wrapValue 成员 "value" |
| 0/11/12/≥15 | — | throw OdError(3) | | |

itemMode（13 的子调度 @0x2876fe）：0/6=标量（ctx vcall+0x10）、1=定长数组（prop.getSize()）、
5=变长数组（count=0xFFFFFFFF）、2/3/4 在 variant 语境 throw。

## 3. 校勘：与样例字节的对齐

- 分区对象区实测 `[u32?][u32?][76 05 00 00 …]`：`76 05`=i16 1398=OdBmElementHeader ✓（2023）；
  后随 `00 00` 属 ElementHeader 对象体首属性，**不是 u32 类索引的高半字**。旧"u32 类索引"解读作废（R30）。
- 记录外层 [counter][size] 包装仍在分区子流层（PartitionStreamDataReader 语境），非本信封——待 #10 续追。

## 4. 未知挂账

- OdStringReader/OdGUIDReader 磁盘布局（外部库实现，E 头文件或样例对拍可解）
- getFieldType 枚举名（实现外部；按行为命名表 §2 为准）
- case 3/10、4/5 语义差；vtable+0x68 返回 9 的 wrap 条件语义
- [counter][size] 记录包装的消费点（分区子流循环）
- UnitSystem 枚举全表（#4 未做）
