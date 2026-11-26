# 考核方案配置

本文档定义考核方案的配置结构、版本记录、核算时的选择方式和公式规则。产品结构、方案配置页和界面交互见 `docs/product-redesign.md` 的 6.8.11 与 6.8.12。默认值的唯一来源是 `docs/requirements-analysis.md` 的口径确认表；当前实现的数据模型与模块结构见 `docs/system-design.md`，后续目标模型见 `docs/design-roadmap.md`。

## 实现状态

| 能力 | 当前状态 | 说明 |
|---|---|---|
| 内置默认方案 | 已实现 | 首次启动写入并发布版本 1 |
| 严格 JSON 解析与语义校验 | 已实现 | 未知字段拒绝，公式编译和指标依赖检查 |
| 发布与停用版本 | 已实现 | 派生方案可以发布和停用，正式核算只接受已发布版本 |
| 内置方案更新 | 已实现 | 更新时追加已发布版本，旧版本保留，内置方案不能停用 |
| 派生方案数据模型 | 已实现 | `store.CreatePlan` 写入完整配置，版本 1 为草稿 |
| 派生方案新建界面 | 已实现 | 方案页填写完成后选择「保存为新的方案」 |
| 方案配置页 | 已实现 | 全部配置项以分节表单呈现，顶部下拉切换方案，底部一个保存按钮；保存前执行完整校验 |
| 版本记录 | 已实现 | 只供存储层和已有核算结果引用，界面不展示 |
| 核算时选择版本 | 已实现 | 本次考核详情操作区的方案版本下拉框 |
| 考核级按科分档边界微调 | 已实现 | 工作台「调整分档边界」弹窗按方案默认预填，支持行内编辑与重置、参评组数变化阻断（`BoundaryStaleError`）、结果快照留痕（`result_run_boundaries`）与「计算依据」导出 |
| 核算摘要与范围提示 | 未实现 | 计划在版本选择区域补充 |

## 目的与范围

考核方案是全部计算规则的集合。学校调整指标分值、档位、并列规则、合并组算法或者零目标处理时，修改方案即可，计算代码不另写默认值。

学校当前只维护一套方案，所有年级统一使用。默认方案覆盖全部年级和考试类型，高考专项通过方案内的 `programs` 条件规则表达，不单独建立方案。个别批次需要特殊处理时，从默认方案派生新方案，在核算时显式选择，结果记录选用的版本。

方案配置的是「怎么算」，批次数据配置的是「拿什么算」。这条边界必须保持：

- 方案配置：指标分值、公式、封顶、档位权重、名次边界、并列规则、合并组算法、个人得分规则、零目标处理、校验规则。
- 批次数据：学生、班级、教学班、统计组成员、基数、目标、成绩、任课关系。

并列名次、分档边界、零目标处理和合并组算法都属于方案配置项。第 23 项的合并规则属于方案，哪些教学班合并成一个统计组、统计组目标是多少属于批次数据。这样同一个方案可以同时用于期中、期末和高考，而每次考试的走班名单和统计组目标各自保存在批次里。

## 方案库

系统管理的是一组方案，初始只有内置默认方案。

| 概念 | 含义 | 可变性 |
|---|---|---|
| 方案 | 一组考核规则的集合，带名称和说明 | 名称和说明可以修改 |
| 方案版本 | 某一时刻的完整配置快照 | 发布后只读 |
| 内置默认方案 | 首次启动写入，`kind = system` | 不能删除、不能停用、不能失效，可以更新为新版本 |
| 派生方案 | 从某个方案版本复制出来 | 独立演进，不影响来源方案 |

内置默认方案首次启动写入版本 1，状态为已发布。修改默认方案时生成版本 2，版本 1 保留在数据库中供已有结果引用。恢复默认操作从出厂配置生成一个新版本。

内置方案始终至少保留一个已发布版本。更新内置方案时，保存动作在一个事务里追加新的已发布版本，旧版本保留。应用启动时如果发现内置方案没有已发布版本，读取最新版本配置并发布一个新版本；内置方案没有任何版本时从 `assessment.DefaultConfig()` 恢复。

派生方案从默认方案或者其他方案的某个版本复制。复制时保存完整配置，不保存相对来源方案的差异；来源方案以后变化，派生方案不受影响。

常见派生方案：

- 「并列不跳名次」：从默认方案复制，把 `ranking.tieMode` 改为 `dense`，用于学校确认并列按 1、1、2 处理的批次。
- 「零目标班级不参与」：从默认方案复制，把 `validation.zeroTarget.action` 改为 `pause_and_exclude`，用于个别班级没有完成率目标的批次。
- 「人工分档边界」：从默认方案复制，把 `ranking.boundary.mode` 改为 `manual`，用于需要沿用某张名次区间表的批次。

## 版本状态

数据库里每个方案保留版本记录，状态为 `draft`、`published` 或 `retired`，只有 `published` 可以用于正式核算。版本记录供存储层和已有核算结果引用，界面不展示，也没有停用、回退和版本对比操作。

方案页顶部用下拉框切换要修改的方案，下面是全部分区。用户只看到一个保存按钮，点击后询问本次保存的目的：

| 选择 | 写入结果 | 版本状态 |
|---|---|---|
| 保存为新的方案 | 新建派生方案，来源版本指向当前打开的版本 | 保存后直接发布 |
| 更新当前方案 | 在当前方案下追加版本 | 保存后直接发布 |

两种选择都执行完整配置校验和公式编译。保存失败时不写入任何版本，当前已发布版本继续有效。版本号与版本状态对用户隐藏，界面只显示当前方案，历史版本只由核算结果引用。

## 核算时选择方案

核算入口按批次打开，当前实现在批次工作台顶部选择方案版本：

- 新批次默认选中内置默认方案的最新已发布版本。
- 已经核算过的批次默认选中上次使用的版本。
- 下拉框按「方案名称 + 版本号」列出全部已发布版本，状态固定为已发布。
- 开始核算后，结果记录 `plan_version_id`。
- 已有结果时更换方案，旧结果保留；导出前发现输入摘要不一致时要求重新核算。

方案摘要、适用范围提示和两个版本的并排对比属于后续界面。当前同一批次重新核算会保留旧运行记录，最新运行标记为正式结果。

## 可配置项清单

方案配置页覆盖下面全部内容。每项都有出厂默认值，用户可以修改。公式字段直接显示在页面上，不放在隐藏的高级页签里。

| 分区 | 配置键 | 页面控件 | 保存时校验 |
|---|---|---|---|
| 方案信息 | `name`、`description` | 文本输入 | 名称非空，同一方案名称唯一 |
| 适用范围 | `appliesTo.grades`、`appliesTo.examTypes` | 多选 | 至少选择一项，取值在年级与考试类型枚举内 |
| 数据口径 | `data.validStudent`、`excludeFlags`、`excludeStatus`、`baselineSource`、`gradeAverage` | 下拉框、多选 | 取值在枚举内，剔除清单不重复 |
| 走班口径 | `data.walking.countIn`、`adminClassDoubleCount`、`japanese.englishStatistics`、`japanese.totalScore`、`japanese.rankGroup`、`artSports.rankGroup` | 下拉框、开关 | 取值在枚举内 |
| 指标列表 | `indicators[].key`、`indicators[].points`、`indicators[].cap` | 指标列表、数字输入、可选上下限 | 指标键唯一，分值合计为 100，上限大于下限 |
| 指标公式 | `indicators[].formula` | 公式选项表单（形状选择、句子预览，不匹配时只读回显自定义表达式） | 语法、变量、类型、依赖、字面除零和节点上限 |
| 合并组算法 | `indicators[].merge.numerator`、`denominator`、`resultRow`、`leadClass` | 下拉框 | 取值在枚举内，合并组目标存在 |
| 档位 | `tiers[].key`、`label`、`weight` | 可增删、可排序的表格 | 档位键唯一，权重为大于零的有限数值 |
| 排名分组 | `ranking.groupBy`、`ranking.tieMode` | 多选、下拉框 | 至少选择一个分组字段，并列规则在枚举内 |
| 分档模式 | `ranking.boundary.mode` | 下拉框 | `ratio` 或 `manual` |
| 分档公式 | `ranking.boundary.first`、`ranking.boundary.second` | 公式选项表单（均等份数数字框、句子预览，不匹配时只读回显自定义表达式） | 返回整数名次，边界递增，覆盖 1 到参评数量 |
| 学科例外 | `ranking.boundary.overrides[]` | 可增删的表格 | 学科存在，参评数量为正整数，边界连续 |
| 人工区间 | `ranking.boundary.manual[]` | 可增删的表格 | 学科存在，区间连续且覆盖全部名次 |
| 赋分 | `ranking.points[]` | 可增删的数字列表 | 每档一个分值，数量与档次一致 |
| 个人得分 | `personal.mode`、`personal.scope` | 下拉框 | 取值在枚举内 |
| 专项规则 | `programs[].when`、`indicatorOverrides` | 可增删的条件行、指标选择和公式选项表单 | 条件取值合法，覆盖的指标存在，覆盖公式通过编译 |
| 校验规则 | `validation.blocking`、`validation.warning` | 多选 | 取值在问题代码枚举内 |
| 零目标 | `validation.zeroTarget.action`、`allowNotParticipating`、`recomputeAfterFill`、`audit`、`recheckBeforeExport` | 下拉框、开关 | 动作在枚举内 |

公式字段共有三类：

1. 指标公式 `indicators[].formula`，计算每项指标的原始比值。
2. 分档边界公式 `ranking.boundary.first` 与 `ranking.boundary.second`，计算每个学科排名组的一档与二档结束名次。
3. 专项覆盖公式 `programs[].indicatorOverrides[key].formula`，在条件匹配时替换指标公式。

三类公式使用同一个公式引擎、同一套变量和同一套函数。页面为每个公式显示可用变量、函数、编译结果和试算值。封顶、分值、权重、赋分和区间属于数值参数，使用数字输入或者表格编辑。

## 历史数据反推的规则

下面这些规则已经通过 2026 年春季高一、高二、高三的期中、期末工作簿和高考专项工作簿确认。它们属于方案，配置页逐项展示，计算代码和导入器不另存一套默认值。

| 已确认规则 | 配置键与默认值 | 说明 |
|---|---|---|
| 应考名单与成绩单按考号取交集 | `data.validStudent = registered_and_scored` | 报名表与成绩单同时存在才进入统计 |
| 缺考、违纪、作弊、无效成绩剔除 | `data.excludeFlags = absent, violation, cheating, invalid` | 标记清单同时决定有效考生 |
| 转学、休学、借读学生剔除 | `data.excludeStatus = transferred, suspended, borrowed` | 学生变化后按当前名单重算 |
| 单科 0 分按未参加该科处理 | `data.zeroScore = absent` | 该科不计人数、均分和达线人数，总分保留 |
| 单科本科线按物理类与历史类分别取线 | `data.cutoffTrack = by_track` | 同一科目不同类别使用不同分数线 |
| 基数取中考成绩 | `data.baselineSource = zhongkao` | 三个年级统一 |
| 年级均分取年级总计行 | `data.gradeAverage = total_row` | 包含该年级全部参评班级 |
| 走班学生按实际授课教学班统计 | `data.walking.countIn = teaching_group` | 行政班不重复计人 |
| 日语考生不进入英语统计，总分保留 | `data.walking.japanese.englishStatistics = exclude`、`totalScore = include` | 排名随教学班 |
| 艺体考生随教学班排名 | `data.walking.artSports.rankGroup = teaching_group` | 不单独成组 |
| 均分提升率 40 分，不封顶 | `indicators.improvement.points = 40`、`cap = null` | 公式结果乘以满分 |
| 单科达线率 40 分，不封顶 | `indicators.pass.points = 40`、`cap = null` | 得分可以超过 40 |
| 班级完成率 20 分，不封顶 | `indicators.completion.points = 20`、`cap = null` | 得分可以超过 20 |
| 档位权重 | `tiers[].weight = 50, 40, 30, 20, 10, 8` | 实际完成分与目标基础分共用 |
| 三项指标直接相加 | `composite.mode = sum` | 不做组内标准分 |
| 班级完成率由各科共用 | `composite.completionShared = true` | 每个学科教师的量化成绩都包含该班完成率得分 |
| 班主任不单独产出个人得分 | `personal.includeHeadTeacher = false` | 个人得分表只列学科教师 |
| 排名对象是统计组 | `ranking.unit = stat_group` | 合并组只占一个名次 |
| 排名依据是教学量化成绩 | `ranking.score = total` | 从高到低 |
| 排名组为同年级同学科 | `ranking.groupBy = grade, subject` | 参评对象是该学科开设的统计组 |
| 并列默认跳过后一名 | `ranking.tieMode = competition` | 当前口径，等待学校确认 |
| 并列跨越边界时整组进入较高档 | `ranking.tieBoundary = higher_tier` | 下一档从并列组之后开始 |
| 三档赋分 | `ranking.points = 70, 67, 64` | 第 1 个元素对应第 1 档 |
| 默认分档边界按比例生成 | `ranking.boundary.mode = ratio` | 一档止 `floor(n/3)`，二档止 `round(2n/3)` |
| 个别学科边界例外 | `ranking.boundary.overrides` | 记录学科、参评数量和实际边界 |
| 合并组分子为成员达线人数之和 | `indicators.pass.merge.numerator = sum_members` | 成员教学班达线人数相加 |
| 合并组分母取统计组目标 | `indicators.pass.merge.denominator = group_target` | 统计组目标单独维护 |
| 合并组结果写在承担统计职责的班级行 | `indicators.pass.merge.resultRow = lead_class` | 合并结果只出现一次 |
| 合并组成员行保留任课教师，不重复得分 | `indicators.pass.merge.memberRows = keep_teacher` | 其他成员行不重复显示赋分 |
| 教师个人得分取最高两个统计组平均 | `personal.mode = top_two_average` | 只教一个组时取该组赋分 |
| 跨年级教师按年级分别记录 | `personal.scope = per_grade` | 每个年级一条记录 |
| 高考完成率按 60% 与 40% 合成 | `programs[gaokao].indicatorOverrides.completion` | 目标任务完成率 0.6 加本科达线完成率 0.4 |
| 目标基础分为 0 时暂停并补齐 | `validation.zeroTarget.action = pause_and_prompt` | 允许补录或者本班不参与 |
| 导出前再次检查零目标 | `validation.zeroTarget.recheckBeforeExport = true` | 仍有零目标时不允许导出 |
| 阻断项与提醒项 | `validation.blocking`、`validation.warning` | 校验结果按文件、工作表、行号和列名列出 |

配置结构版本 1 已经包含大部分字段。`data.zeroScore`、`data.cutoffTrack`、`composite`、`ranking.unit`、`ranking.score`、`ranking.tieBoundary`、`indicators.pass.merge.memberRows` 和 `personal.includeHeadTeacher` 加入配置结构时，`schemaVersion` 升为 2。读取版本 1 的配置时在内存里补齐这些字段的默认值，保存新版本时写入当前结构。

方案保存计算规则。文件格式、工作表识别、表头关键词、列名同义词和旧版 `.xls` 转换属于导入规则；学生名单、成绩、分数线、目标和合并组成员属于批次数据。

## 配置结构

配置保存为 JSON，顶层字段固定。下面的结构是版本 1：

```json
{
  "schemaVersion": 1,
  "name": "教学成绩量化考核方案",
  "description": "学校内置默认方案，覆盖全部年级和考试类型",
  "appliesTo": {
    "grades": ["g1", "g2", "g3"],
    "examTypes": ["midterm", "final", "gaokao"]
  },
  "data": {
    "validStudent": "registered_and_scored",
    "excludeFlags": ["absent", "violation", "cheating", "invalid"],
    "excludeStatus": ["transferred", "suspended", "borrowed"],
    "baselineSource": "zhongkao",
    "gradeAverage": "total_row",
    "walking": {
      "countIn": "teaching_group",
      "adminClassDoubleCount": false,
      "japanese": {
        "englishStatistics": "exclude",
        "totalScore": "include",
        "rankGroup": "teaching_group"
      },
      "artSports": {
        "rankGroup": "teaching_group"
      }
    }
  },
  "indicators": [
    {
      "key": "improvement",
      "points": 40,
      "cap": null,
      "formula": "class.average[subject] / grade.average[subject] / baseline.ratio[subject]"
    },
    {
      "key": "pass",
      "points": 40,
      "cap": null,
      "formula": "statGroup.passCount / statGroup.target",
      "merge": {
        "numerator": "sum_members",
        "denominator": "group_target",
        "resultRow": "lead_class",
        "leadClass": "designated"
      }
    },
    {
      "key": "completion",
      "points": 20,
      "cap": null,
      "formula": "tier.actual / tier.base"
    }
  ],
  "tiers": [
    {"key": "qingbei", "label": "清北", "weight": 50},
    {"key": "c9", "label": "C9", "weight": 40},
    {"key": "project985", "label": "985", "weight": 30},
    {"key": "project211", "label": "211", "weight": 20},
    {"key": "special_control", "label": "特控", "weight": 10},
    {"key": "undergraduate", "label": "本科", "weight": 8}
  ],
  "ranking": {
    "groupBy": ["grade", "subject"],
    "tieMode": "competition",
    "boundary": {
      "mode": "ratio",
      "first": "floor(n / 3)",
      "second": "round(2 * n / 3)",
      "overrides": [
        {"subject": "chinese", "n": 17, "first": 6, "second": 11}
      ]
    },
    "points": [70, 67, 64]
  },
  "personal": {
    "mode": "top_two_average",
    "scope": "per_grade"
  },
  "programs": [
    {
      "when": {
        "examType": "gaokao"
      },
      "indicatorOverrides": {
        "completion": {
          "formula": "program.targetCompletion * 0.6 + program.undergraduatePass * 0.4"
        }
      }
    }
  ],
  "validation": {
    "blocking": [
      "missing_required_column",
      "duplicate_student",
      "zero_target_base"
    ],
    "warning": [
      "count_mismatch",
      "missing_score_without_flag"
    ],
    "zeroTarget": {
      "action": "pause_and_prompt",
      "allowNotParticipating": true,
      "recomputeAfterFill": true,
      "audit": true,
      "recheckBeforeExport": true
    }
  }
}
```

`grades` 使用 `g1`、`g2`、`g3`，`examTypes` 使用 `midterm`、`final`、`gaokao`。学科使用 `subjects` 表里的稳定代码，例如 `chinese`、`math`、`english`、`physics`、`chemistry`、`biology`、`history`、`geography`、`politics`、`japanese`。`外语`和`英语`使用同一个 `english` 代码，界面标签按年级显示。界面显示中文名称，配置保存代码，改名不影响历史数据。

目标结构版本 2 在上面的版本 1 上增加以下字段：

```json
{
  "schemaVersion": 2,
  "data": {
    "zeroScore": "absent",
    "cutoffTrack": "by_track"
  },
  "indicators": [
    {
      "key": "pass",
      "merge": {
        "numerator": "sum_members",
        "denominator": "group_target",
        "resultRow": "lead_class",
        "leadClass": "designated",
        "memberRows": "keep_teacher"
      }
    }
  ],
  "ranking": {
    "unit": "stat_group",
    "score": "total",
    "tieBoundary": "higher_tier"
  },
  "composite": {
    "mode": "sum",
    "completionShared": true
  },
  "personal": {
    "includeHeadTeacher": false
  }
}
```

### 数据口径

| 字段 | 取值 | 默认 | 说明 |
|---|---|---|---|
| `validStudent` | `registered_and_scored`、`registered`、`scored` | `registered_and_scored` | 有效考生的定义 |
| `excludeFlags` | `absent`、`violation`、`cheating`、`invalid`、`not_selected` | `absent`、`violation`、`cheating`、`invalid` | 成绩标记的剔除清单 |
| `excludeStatus` | `transferred`、`suspended`、`borrowed`、`left_school` | `transferred`、`suspended`、`borrowed` | 学生状态的剔除清单 |
| `zeroScore` | `absent`、`valid` | `absent` | 单科 0 分按未参加该科处理或者作为有效分数 |
| `cutoffTrack` | `by_track`、`single` | `by_track` | 单科本科线是否按物理类与历史类分别取值 |
| `baselineSource` | `zhongkao`、`placement`、`previous_exam`、`imported` | `zhongkao` | 均分提升率的基数来源 |
| `gradeAverage` | `total_row`、`class_row` | `total_row` | 年级均分取总计行或者指定班级 |
| `walking.countIn` | `teaching_group`、`admin_class` | `teaching_group` | 走班学生计入哪个单位 |
| `walking.japanese.englishStatistics` | `exclude`、`include` | `exclude` | 日语考生是否进入英语统计 |
| `walking.japanese.totalScore` | `include`、`exclude` | `include` | 日语考生的总分是否进入总分统计 |
| `walking.japanese.rankGroup` | `teaching_group`、`separate` | `teaching_group` | 日语考生的排名组 |
| `walking.artSports.rankGroup` | `teaching_group`、`separate` | `teaching_group` | 艺体考生的排名组 |

单科成绩为 0 时按未参加该科考试处理。导入阶段把这种成绩记为没有该科成绩，不计入该科人数、均分和达线人数，总分仍然保留。这个规则来自已经核对过的历史工作簿，属于数据读取口径，不随方案版本变化。

### 指标

每个指标有固定的键、满分、封顶和公式。公式返回比值或者原始比例，引擎把公式结果乘以 `points` 得到该项得分。三项默认指标如下：

| 键 | 满分 | 公式 | 封顶 |
|---|---|---|---|
| `improvement` | 40 | `class.average[subject] / grade.average[subject] / baseline.ratio[subject]` | 不封顶 |
| `pass` | 40 | `statGroup.passCount / statGroup.target` | 不封顶 |
| `completion` | 20 | `tier.actual / tier.base` | 不封顶 |

`points` 之和必须等于 100。`cap` 为 `null` 表示不封顶；设置上下限时，计算先得到公式结果的得分，再按上下限截断，结果明细同时保存原值和截断后的值。

### 档位权重

`tiers` 保存六个档位的代码、显示名称和权重。班级完成率的实际完成分和目标基础分都按这张表加权：

```
实际完成分 = 清北人数 × 50 + C9 人数 × 40 + 985 人数 × 30
           + 211 人数 × 20 + 特控人数 × 10 + 本科人数 × 8
目标基础分 = 同上，人数换成目标人数
```

档位代码保持英文，`label` 用于界面显示。增加或者减少档位时修改 `tiers`；排名档次和赋分由 `ranking.points` 单独定义，两者互不影响。

### 排名、分档与赋分

| 字段 | 取值 | 默认 |
|---|---|---|
| `groupBy` | `["grade","subject"]`、`["grade"]`、`["school"]` | `["grade","subject"]` |
| `tieMode` | `competition`、`dense`、`ordinal` | `competition` |
| `boundary.mode` | `ratio`、`manual` | `ratio` |
| `boundary.first` | 一档结束名次的表达式 | `floor(n / 3)` |
| `boundary.second` | 二档结束名次的表达式 | `round(2 * n / 3)` |
| `boundary.overrides` | 按学科、参评数量覆盖边界 | 空 |
| `points` | 排名档次到赋分，第 1 个元素对应第 1 档 | 第 1 档 70、第 2 档 67、第 3 档 64 |

并列处理：

- `competition`：同分同名次，下一名跳过，例如 1、1、3。当前默认，已由学校确认。
- `dense`：同分同名次，下一名不跳过，例如 1、1、2。
- `ordinal`：按排序顺序给唯一名次，同分时按班级标识稳定排序。

分档模式：

- `ratio`：按参评统计组数量计算边界，适合作为各年级各科的默认初始参考。`overrides` 记录个别学科或者参评数量的例外，例如 `{"subject":"chinese","n":17,"first":6,"second":11}`。
- `manual`：每个学科维护一张名次区间表，贴近学校按学期、按科人工调整的实际做法；工作台后续支持基于默认比例自动预填后，仅对有请假或人数变动的学科做本次考核级微调。

两种模式都按名次判定档次，并列的统计组进入同一档。并列跨越边界时，整个并列组进入较高的档次，下一档从并列组最后一个名次之后开始。保存时校验边界连续、覆盖 1 到参评数量、赋分覆盖全部档次。第 16 项排名并列与第 17 项分档边界均已由学校确认。

### 合并统计组

`indicators.pass.merge` 定义合并组的算法：

| 字段 | 取值 | 默认 | 说明 |
|---|---|---|---|
| `numerator` | `sum_members` | `sum_members` | 成员教学班达线人数相加 |
| `denominator` | `sum_member_targets`、`lead_class_target`、`group_target` | `group_target` | 分母取成员目标之和、承担统计职责班级的目标或者统计组直接录入的目标 |
| `resultRow` | `lead_class`、`each_member`、`new_row` | `lead_class` | 结果写在承担统计职责的班级行、每个成员行或者单独一行 |
| `leadClass` | `designated`、`largest_target`、`first_by_name` | `designated` | 承担统计职责的班级如何确定 |

统计组的成员名单、承担统计职责的班级和统计组目标保存在批次数据里。统计组目标单独维护，成员教学班目标之和可以与它不同。高一期中地理 `(4+16)/(14+4)`、政治 `(1+15)/(0+14)`、历史 `(5+3+1)/6`，高一期末政治 `(1+16)/(1+14)`，都按成员达线人数相加、分母取统计组目标计算。高一期中历史的统计组目标是 6，成员教学班目标之和是 8，以统计组目标为准。高一期末 539、542、555 的历史是三个独立教学班，按各班目标 3、2、3 分别计算。哪些教学班合并、统计组目标是多少属于批次数据，合并算法属于方案。

### 零目标处理

`validation.zeroTarget` 定义目标基础分为 0 时的行为：

| 字段 | 取值 | 默认 |
|---|---|---|
| `action` | `pause_and_prompt`、`pause_and_exclude` | `pause_and_prompt` |
| `allowNotParticipating` | 布尔值 | `true` |
| `recomputeAfterFill` | 布尔值 | `true` |
| `audit` | 布尔值 | `true` |
| `recheckBeforeExport` | 布尔值 | `true` |

默认流程：

1. 计算前的校验阶段扫描目标。目标人数为空白、六档全为 0、或者加权后目标基础分为 0 时，暂停计算并打开补齐窗口。
2. 弹窗按班级列出缺失的档位和当前值。
3. 用户可以逐项补录，也可以选择「本班不参与本次完成率计算」并填写原因。
4. 补录完成后重新计算该批次，得分和排名一起更新。
5. 记录补录人、时间、原值、新值和原因，差异报告列出这次补录。
6. 导出前再次校验，仍有 0 目标时不允许导出。

空白和明确填写的 0 分开处理：空白属于缺失，明确填写的 0 属于学校设置的有效目标。整班不参与的选择单独记录，不把六档目标改成 0。

### 个人得分

| 字段 | 取值 | 默认 |
|---|---|---|
| `mode` | `top_two_average`、`weighted_by_count`、`average_all`、`top_one`、`lowest_one` | `top_two_average` |
| `scope` | `per_grade` | `per_grade` |

`top_two_average` 取所教统计组赋分中最高的两个组求平均；只教一个组时取该组赋分。跨年级教师按年级分别生成记录，每个年级一条。

### 专项规则

`programs` 按条件覆盖指标参数。高考默认使用：

```
完成率 = 目标任务完成率 × 0.6 + 本科达线完成率 × 0.4
```

这个公式返回比值，引擎再乘以 `completion` 的 20 分。条件支持 `examType`、`grade`、`cohort`。条件匹配时，覆盖字段替换默认值；没有匹配时使用默认值。试算结果会显示实际生效的规则。

## 公式

每个指标有一个 `formula` 字段，存放受限表达式。表达式只允许：

- 数字、字符串、布尔值和空值。
- 变量路径和方括号索引，例如 `class.average[subject]`。
- 四则运算、比较、布尔运算、条件表达式和括号。
- 白名单函数 `min`、`max`、`abs`、`round`、`floor`、`ceil`、`if`、`coalesce`、`sum`、`avg`、`count`。

不允许循环、函数定义、任意函数调用、文件访问和网络访问。

### 变量

| 变量 | 含义 |
|---|---|
| `class.average[subject]` | 本次班级均分 |
| `class.count[subject]` | 本次班级有效人数 |
| `class.totalAverage` | 本次班级总分均分 |
| `grade.average[subject]` | 年级总计行均分 |
| `grade.totalAverage` | 年级总分均分 |
| `baseline.average[subject]` | 统计组基数均分 |
| `baseline.ratio[subject]` | 班级基数比值 |
| `statGroup.passCount` | 统计组达线人数 |
| `statGroup.target` | 统计组目标 |
| `statGroup.memberCount` | 统计组成员教学班数量 |
| `tier.actual` | 六档实际完成分 |
| `tier.base` | 六档目标基础分 |
| `tier.count[tier]` | 某一档的实际人数 |
| `tier.target[tier]` | 某一档的目标人数 |
| `indicator[key]` | 其他指标计算后的最终得分 |
| `program.targetCompletion` | 目标任务完成率 |
| `program.undergraduatePass` | 本科达线完成率 |

### 校验

保存方案版本时依次执行：

1. 结构校验：字段齐全，未知字段拒绝，枚举值合法。
2. 语义校验：`points` 之和为 100，指标键唯一，档位权重存在，赋分覆盖全部档次，名次边界连续，合并组目标存在。
3. 公式编译：解析语法树，检查变量存在、类型匹配、没有自引用、没有字面除零，节点数量不超过上限。

只有状态为 `published` 的版本可以正式核算。运行时除零、变量缺失或者类型不符都返回带行号和变量值的错误，不生成无穷大或者空值。

### 结果明细

每个指标的计算明细写入 `result_rows.indicator_values_json`：

```json
{
  "improvement": {
    "classAverage": 95.2,
    "gradeAverage": 88.1,
    "baselineRatio": 1.08,
    "ratio": 1.003,
    "raw": 40.12,
    "capped": 40.12
  },
  "pass": {
    "passCount": 49,
    "target": 11,
    "ratio": 4.4545,
    "raw": 178.18
  },
  "completion": {
    "actual": 558,
    "base": 910,
    "ratio": 0.6132,
    "raw": 12.26
  }
}
```

界面点开一个数字时显示公式、代入值、计算过程和方案版本。参与计算的学生名单按输入快照在查看时重新计算，不重复写入结果表。

## 配置项扩展

配置结构由应用定义，用户选择取值，不能自行增加引擎不认识的字段。扩展分三种情况：

| 需求 | 是否需要更新应用 | 做法 |
|---|---|---|
| 调整指标分值、档位、并列规则、零目标处理 | 不需要 | 新建方案版本或者派生方案 |
| 用新的数学公式计算某个指标 | 不需要 | 在 `formula` 里使用已有变量和函数 |
| 新增变量、新增指标类型、新增排名方式 | 需要 | 应用升级时增加 `schemaVersion` 和迁移 |

每个方案版本记录写入时的 `schemaVersion`。应用升级后读取旧版本时在内存里升级到当前结构，新字段填入默认值；发布新版本时写成当前结构。旧版本的计算结果不重写，仍按当时的版本解释。遇到无法自动决定含义的变化时，打开迁移对话框，由用户确认后再转换。

## 界面

当前已经实现的界面：

- 方案页顶部用下拉框列出内置方案和派生方案，右侧显示方案类型。
- 下面是全部分区的可视化配置表单，底部固定一个保存按钮；保存前执行严格解析、语义校验和公式编译。
- 本次考核详情的操作区可以选择已发布版本，默认选中上次使用的版本，不可用时回退到第一个已发布版本。

下面是完整的方案配置界面设计，页面、交互和视觉规范以 `docs/product-redesign.md` 的 6.8.11 与 6.8.12 为准。

方案库页面：

- 顶部下拉框列出全部方案，右侧标出内置默认方案或者派生方案。
- 下拉框下面直接是全部配置项，没有任何版本列表。
- 未选择年级时侧栏不显示年级切换。

方案配置页面：

- 默认打开内置方案当前生效的版本。
- 按「方案信息、适用范围、数据口径、走班口径、指标、档位、排名与分档、个人得分、专项规则、校验」分节。
- 每个配置项使用下拉框、多选、开关、数字输入或者文本输入。
- 指标、分档边界和专项覆盖公式使用单行输入框。

保存动作：

- 底部只有一个「保存」按钮，点击后询问「保存为新的方案」还是「更新当前方案」。
- 「保存为新的方案」使用表单里的名称创建派生方案，来源版本指向当前打开的版本。
- 「更新当前方案」在当前方案下追加版本并立即发布，旧版本保留。
- 两种保存都在数据库事务里完成。保存失败时不写入版本，错误显示在保存动作旁边。

核算页面：

- 顶部显示当前方案和版本。
- 更换方案时显示摘要和受影响的结果数量。
- 试算结果单独存放，不进入正式导出。

## 数据模型

Schema v1（见 `docs/system-design.md` 与 `internal/store/schema.go`）已移除旧版 `batches` 与 `target_adjustments`，方案与核算使用 `plans`、`plan_versions`、`assessments.plan_version_id`、`assessment_stat_groups`、`assessment_target_adjustments` 与 `result_runs.plan_version_id`：

| 表 | 字段 | 说明 |
|---|---|---|
| `plans` | `is_system` | `1` 为内置默认方案，`0` 为派生方案 |
| `plans` | `scope` | 方案适用范围说明 |
| `plans` | `created_at` | 创建时间 |
| `assessment_stat_groups` | `lead_class_id` | 承担统计职责的班级，未合并时等于唯一成员班级 |
| `plan_versions` | `config_hash` | 配置内容的哈希 |
| `plan_versions` | `status` | `draft`、`published`、`retired` |
| `plan_versions` | `parent_version_id` | 编辑前的版本 |
| `plan_versions` | `note` / `change_summary` | 版本说明与变更摘要 |
| `plan_versions` | `created_at` | 创建时间 |
| `plan_versions` | `created_by` | 创建人 |
| `plan_versions` | `published_at` | 发布时间 |
| `result_rows` | `indicator_values_json` | 每个指标的分子、分母、基数、原值和封顶后的值 |

`assessments.plan_version_id` 记录本次考核当前选用的版本。核算时如果用户选了另一个版本，先更新该字段，再生成新的 `result_runs` 记录。同一考核只能有一个正式结果，由 `result_runs.is_official` 约束。

内置方案的数据不变量：

- `plans.is_system = 1` 的方案在 `plan_versions` 中始终至少有一条 `status = 'published'` 的记录。
- `Retire` 先读取方案类型，内置方案返回错误，不执行更新。
- 更新内置方案时，读取最新已发布版本作为 `parent_version_id`，追加版本号最大的新记录，状态直接写入 `published`。
- 启动时调用 `EnsureDefaultPlan`。没有内置方案时从 `assessment.DefaultConfig()` 建立；有内置方案但没有已发布版本时，读取最新版本的配置并追加一条已发布版本；没有任何版本时从出厂配置追加版本。
- 恢复失败返回具体错误，不写入空配置，不把错误转换成默认值。

`assessment_target_adjustments` 表（`id`、`assessment_id`、`class_id`、`tier_key`、`adjusted_target`、`excluded`、`reason`、`updated_at`）只记录零目标补录和本班不参与的选择，不改写原始导入数据。

## 实施顺序

1. 方案库与配置内核：内置默认方案、方案与版本表、配置加载与校验、公式引擎、三个指标、排名分档、个人得分、结果明细。用 2026 年春三个年级的期中、期末和高考材料逐项比对。
2. 方案选择与保存：核算时选择版本、保存为新的方案、更新当前方案、旧结果保留。
3. 方案配置页面：分节表单、指标与公式编辑、`schemaVersion` 迁移、零目标弹窗。

第一阶段完成后，系统可以按内置默认方案算出正确结果；第二阶段完成后，教务处可以保存新的方案、更新本方案并在核算时选择；第三阶段完成后，可以在界面里修改全部规则和公式。

## 关键口径与配置位置

| 项 | 配置键 | 当前默认 | 状态 |
|---|---|---|---|
| 16 排名并列 | `ranking.tieMode` | `competition`，同名次并跳过后续名次，例如 1、1、3 | 已确认 |
| 17 分档边界 | `ranking.boundary.mode` | 默认按 `ratio`（一档止 `floor(n/3)`，二档止 `round(2n/3)`）提供参考，支持 `manual` / 考核级按科人工微调 | 已确认 |
| 18 小语种 | `data.walking.japanese` | 小语种不参与常规计算与量化排名 | 已确认 |
| 19 艺体生 | `data.walking.artSports` | 平时不参与统计，仅高考专项参与 | 已确认 |
| 20 零目标 | `validation.zeroTarget` | 计算前暂停、弹窗补齐、允许不参与、重算、审计、导出前复检 | 待细则答复 |
| 21 跨年级教师 | `personal.scope` | 按年级分别记录 | 已确认 |
| 22 年级均分分母 | `data.gradeAverage` | 年级总计行 | 已确认 |
| 23 合并组分子分母 | `indicators.pass.merge.denominator` | 分子相加，分母取统计组目标 | 已确认 |
| 24 合并组归属行 | `indicators.pass.merge.resultRow` | 承担统计职责的班级行 | 已确认 |
| 26 单科成绩为 0 | 导入规则 | 按未参加该科考试处理，不计入该科人数、均分和达线人数 | 已确认 |

第 16、17 项已由学校确认；目前仅第 20 项零目标具体细则待学校答复（维持安全拦截与补齐策略），详细说明见 `docs/requirements-analysis.md` 的「已确认与待确认项说明」。
