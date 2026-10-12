# ElemTable 布局（P1 #6）— 样例字节定案

证据：E3 `Global/ElemTable` 解压后 231753B（2023 样例）逐字节走读；类索引对照 `tables/class-index-2023.csv`。

## 布局

```
头部 6B: 5a 05 54 20 00 00        # magic 0x2054055a (u32) + u16 0  [语义?]
记录 ×N: 28B = 7×u32 LE
  [0] id            # 元素标识；非严格连续（实测 26→29 跳号=删除痕迹）
  [1] id2           # 恒 == id（双写校验?）
  [2] 0             # 恒 0（保留/flags?）
  [3] depA          # 语义 [?]：早期条目 767/551/822；偶见与 depB 不同
  [4] depB          # 多数 == depA；实测一例 (822,565)
  [5] 0             # 恒 0
  [6] 0xffffffff    # 哨兵（-1：无后续/无父?）
尾部 ~19B           # footer [?]
```

- 可整除性：(231753−6−19)/28 = 8276 条（约）——**元素量级 ~8K**（该样例）
- depA 取值样例：767(OdBmFamGeomRefHandles@2023)、551(OdBmBoundaryConditionsUserData)、822、565——**与 csv 类名对照不合"元素自身类"语义**，更像类型组/依赖槽编号；**勿当类索引直接解释**（R35）
- 2022 ElemTable 本地损坏（R21），布局待干净样例复核

## 与分区路由的关系（未定 [?]）

ElemTable **不含**分区号/页号/偏移字段（7 字段内无大数值）——ElementId→分区数据定位应经
**分区 deposit 记录**（103B 区，R24）与 class-103 目录表（R28）：ElemTable 给元素清单，
deposit/目录表给"哪个分区的哪条对象记录"。串联键 = id ↔ deposit counter（待对拍）。

## 未竟

- 头部 magic 0x2054055a 与尾部 19B 语义
- depA/depB 精确语义（REA `ElemTable2011..2023Reader::read`，TB_LoaderBase）
- id↔deposit counter 对拍（需全分区对象记录解码后做）
