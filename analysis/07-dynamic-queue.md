# DynamicQueue 队列语义与字段表（动态段字节框架对齐）

证据：REA/Ghidra 对 TB_LoaderBase.tx（sha256=1a8bf7f5…）六函数 dossier：
addProperty@0x274608、addIdReference@0x274866、addData@0x274dd0、
internalAddData@0x274e68、resolveDispositions@0x2749ee、initReferences@0x275c20
（+ 已有 readPropertyToken@0x276468 / R57）。Ghidra 地址 = ELF+0x100000。

## 0. 队列对象全景（this 布局）

```
+0x00..+0x18  上下文四元组（db/storages/reactors/…，读侧装配）
+0x28  list<IdReference>     哨兵 / 尾+0x30 / size+0x38     ← addIdReference 入队
+0x48  list<DynamicProperty> 哨兵 / 尾+0x50 / size+0x58     ← addProperty 入队
+0x60  OdArray<OdTfVariant>  工作栈（元素 0x28B，CoW 头）    ← initReferences 播种
+0x68  map<DataKey, OdArray<OdTfVariant>>  tree/hdr/root = +0x68/+0x70/+0x78
       map 节点: color+0x00 parent+0x08 left+0x10 right+0x18 | k0+0x20 k1+0x28 kind+0x30 | OdArray*+0x38 (0x40B/节点)
+0xa0  list<DynamicProperty*> null-key 登记  size+0xb0
+0xc0  list<IdReference*>     null-key 登记  size+0xd0
+0x110 另一 refcounted 缓冲（ctx 字段）
```

## 1. DataKey

`{u64 k0, u64 k1, u32 kind}` = 0x18B（kNull 符号 size=0x18 印证）。
**map 排序/查找只用 k0/k1**；kind 存于节点 +0x30 但不参与比较（lower_bound 反汇编证实）。
kNull 为全零（.bss 无初值）；key==kNull 的节点进 null-key 登记表。

## 2. ValueDisposition（读者入队描述符，≥0x30B，栈/寄存器传参非盘上）

| 偏移 | 宽度 | 含义 | 消费者 |
|---|---|---|---|
| +0x00 | u32 | kind（→节点+0x20） | Prop/IdRef；addData 不读 |
| +0x08 | u64 | DataKey.k0 | 全部 |
| +0x10 | u64 | DataKey.k1 | 全部 |
| +0x18 | u64 | value/handle（→Prop+0x28） | 仅 addProperty |
| +0x20 | u32 | idx | 仅 bool (idx==0) |
| +0x24 | u8 | has_idx | Prop+0x45 / IdRef+0x2d |
| +0x28 | u8 | flags（→Prop+0x30） | 仅 addProperty |
| +0x2c | u32 | aux（→Prop+0x34） | 仅 addProperty |

## 3. 节点布局（内存态；运行时构造，非盘上）

**DynamicProperty 0x48B**（addProperty）：
```
+0x00/+0x08 list prev/next
+0x10/+0x18 DataKey.k0/k1
+0x20       kind u32
+0x28       value u64
+0x30       flags u8
+0x34       aux u32
+0x38       OdTfClass const*     ← readPropertyToken 延迟判据（≠0 → 从流读对象）
+0x40       idx u32（值表下标；-1=不回写；≤1 非法→OdError 0x128）
+0x44/+0x45 bool(idx==0) / bool(has_idx)
```

**IdReference 0x30B**（addIdReference；**精简无 class*/value/flags/aux**）：
```
+0x00/+0x08 list prev/next
+0x10/+0x18/+0x20  DataKey 三元组
+0x28       idx u32（来自函数第3参，非 disposition.idx）
+0x2c/+0x2d bool(idx==0) / has_idx（has_idx=0 时 +0x2c 为未初始化残留[瑕疵]）
```

## 4. 生命周期（动态段字节框架）

```
静态体读取期间：
 读者（StaticIntegerReader/CondInt16Reader/ESEntity…）按属性遇"动态值"时
   → addIdReference 或 addProperty( ValueDisposition{kind,DataKey,value,idx,…}, classIdx )
   → 队列尾插；DataKey==kNull 的节点 payload 登记入 +0xa0/+0xc0
   → 盘上此处不写特殊标记——动态值的字节留在流中"静态体之后"

OdBmObjectPtrReader::read 编排（R31）：
  ① InitReader::read(stream)          — 静态体（快/慢路径）
  ② initReferences(bool, rootVariant) — 播种 +0x60 工作栈 [null, this+0x18, root?]
                                        （与 null-key 登记表无关，纯弱指针种子）
  ③ readDynamicProperties(stream):
       while 队列非空:
         readPropertyToken → 弹队首节点
         if node+0x38 (class*) != 0:
             ObjectPtrInitReader.read(stream)   ← ★盘上动态段字节 = 标准对象信封
                                                    （慢: i16 类索引+属性环 / 快: 直读，R49）
         else: 取节点自带值（虚槽+0x80）
         values[node+0x40] = 结果（idx 有效时）
       （resolvePairToken/resolveReferenceToken 处理 pair/引用型节点）
  ④ resolveDispositions(key)（收尾/回填）:
       把 {k0,k1} 盖到 +0xa0/+0xc0 登记的每个节点 DataKey（p+0/p+8）
       清空两登记表
       map[kNull].array → 移交 map[key].array，删 kNull 桶
```

**字节框架结论**：动态段盘上格式 = **按队列弹出序排列的标准对象信封序列**（R57）；
ValueDisposition/DataKey/IdReference/kind/flags 全部是**运行时簿记**，不上盘。
盘上唯一可观察的对齐信息 = 弹出顺序（=静态体内 add* 的调用序）与每个对象的信封本身。

## 5. 已知瑕疵/未知

- addIdReference 在 has_idx=0 时写入未初始化 R15B 到 +0x2c（编译器残留，读侧应忽略）
- resolveDispositions(bool)@0x275b8 与 (ValueDisposition&) 版本的关系未查（低价值）
- DataKey.k0/k1 的业务语义（哪个属性/哪个对象的引用槽）由各读者的 ValueDisposition 构造点决定——分布于全 SDK 读者，按需追
