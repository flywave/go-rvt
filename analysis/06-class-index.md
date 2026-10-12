# 类索引全表（P1 #5）— Index4Enum 提取

## 方法

`TB_Format{2022,2023}Readers.tx` 的 per-class 生成 reader 符号内嵌
`OdBm::Format::version{Y}::classes::Index4Enum` 模板值 + 目标类名：
`…Index4EnumELS7_<N>EE10OdSmartPtrI<len><OdBmClassName>E…`
nm 全量提取 + 去重 → `tables/class-index-{2022,2023}.csv`（index,class）。

## 结果

| 版本 | 唯一类数 | 锚点验证 |
|---|---|---|
| 2023 | 2845 | **1398=OdBmElementHeader** ✓（分区对象区反复出现的 0x0576）、1340=OdBmElectricalDomainDesignPropertyManager、1481=OdBmEntityInfoData、2081=OdBmGFilter、87=OdBmAbsCurveDriver |
| 2022 | 2761 | **1340=OdBmElementHeader** ✓（跨版本同类不同号）、1398=OdBmEnergyAnalysisSchedule、87=OdBmCurveDriver |

**类索引是按版本重编号的**——跨版本比较必须经类名，不得拿数字对数字。

## 校勘（对 rea-extract-plan #3/#8/#9 假设的修正）

- **91/102/103 不在 Index4Enum 全表**（2023/2022 Readers 均无）。它们是**分区页记录/子流的标签号**（R26/R28 的 class 字段），与序列化类索引是不同编号空间。deposit 记录 Field91=91 的语义仍 [?]（非类索引，或属未导出内部类）。
- 计划中"0x821(2023)/0x7df(2022)=class-103 目录表"的类索引假设需按 csv 复核：2081/2015 附近的值须以类名确认后再解释目录表项。

## 用法

- 解释对象区 i16 类索引：`csv[year][index] → OdBm* 类名`
- 下一步（#6 ElemTable、#8/#9 目录表）：先查 csv 再解字段
