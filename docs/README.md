# 文档入口与交接

## 文档地图

| 文档 | 内容 | 什么时候读 |
|---|---|---|
| `docs/requirements-analysis.md` | 业务流程、计算链路、口径确认表、导入与导出要求 | 开始需求、计算、导入导出工作前 |
| `docs/system-design.md` | 当前实现的系统结构、模块职责、运行流程、真实数据库结构、界面状态、错误处理与设计决策 | 修改代码、审查实现、接手当前阶段前 |
| `docs/assessment-plan.md` | 考核方案库、版本记录、完整配置结构、公式规则、核算时选择方案 | 设计考核方案、实现计算引擎、调整默认值前 |
| `docs/product-redesign.md` | 教学数据工作台的产品结构、级与阶段基础数据、引导式导入、考核流程、界面与视觉规范 | 讨论产品重构方向、确认第 11 节决策、调整导航与数据模型前 |
| `docs/design-roadmap.md` | 后续阶段划分、目标数据模型、历史数据、同步与迁移 | 规划后续阶段、同步、历史分析前 |
| `docs/historical-data-checklist.md` | 真实历史数据的目录层级、工作簿与工作表角色、读取规则和说明模板 | 整理或者导入历史数据前 |
| `docs/testing.md` | 界面双轨测试流程、View 与 Modal 测试矩阵、双色快照规范与 mygo-automation 插件驱动 | 编写、修改界面测试或执行自动化前 |
| `docs/release.md` | 发布流程 | 发布前 |
| `docs/commits.md` | 提交规范 | 提交前 |
| `AGENTS.md` | 工程规则和技能路由 | 每个新会话开始时 |
| `DESIGN.md` | Sunbeam 设计系统 Token、材质与组件规范 | 调整配色、排版、圆角、材质与动效前 |
| `.agents/skills/README.md` | 项目技能索引和锁定范围 | 选择技能或者新增技能时 |

## 当前状态

- 阶段：产品与数据架构重构**阶段 1（Schema v1 单一真理源与存储引擎重构）已完成**。彻底移除旧版 `batches` / `terms` / `cohort_stages` / `class_stage_info` / `legacy_*` 表及旧库迁移代码，建立全新 `SchemaVersion = 1` SQLite 模型（见 `internal/store/schema.go`），并通过阶段 1 界面兼容层保障现有全量 UI 与 697 个计算核对点 0 差异回归通过；后续阶段将基于已落地的全校师资占用、升学流转与师生三年档案接口推进阶段 2+ 全校统一导航与「档案即分析」界面重构。
- 代码状态：
  - 根包界面：`shell.go`、`overview.go`、`cohort_state.go`、`ui_theme.go`、`ui_components.go`、`ui_icons.go`、`data_state.go`、`data_pages.go`、`exam_state.go`、`exam_pages.go`、`quality_page.go`、`import_wizard.go`、`import_center.go`、`prep_wizard.go`、`plan_page.go`、`main.go`、`workbench.go`、`analysis_page.go`、`analysis_state.go`、`settings_school.go`、`settings_sync.go`、`settings_window.go`；字体与 Sunbeam 主题由 `internal/fonts` 与 `github.com/AstraFoundry/design/packages/mygo/sunbeam` 提供。
  - 核心存储与业务引擎：`internal/store` 已完成 Schema v1 模块化拆分（`schema.go`、`store.go`、`types.go`、`cohort.go`、`faculty.go`、`students.go`、`exams.go`、`assessments.go`、`profiles.go`、`imports.go`、`imports_apply.go`、`analysis.go`、`workspace.go`、`saved_views.go`、`appearance.go`、`sync_settings.go`、`backup.go`）；`internal/school`、`internal/assessment`、`internal/formula`、`internal/calc`、`internal/importer`、`internal/excel`、`internal/pipeline`、`internal/exporter`、`internal/sync` 保持单向依赖与计算引擎红线。当前实现细节见 `docs/system-design.md`。
- 阶段 1 已落地能力：
  - **全新 Schema v1 单一真理源**：首次启动检测到旧库或包含 `batches` 等旧表时直接返回 `ErrIncompatibleDatabase` 提示新建干净库，不做旧库迁移；初始化预置 10 个学科（含日语）、2 个选科类别（物理类、历史类）与内置默认考核方案。
  - **年级三年生命周期与升学流转**：`cohorts` 以 `enrollment_year` 唯一并持久化 `current_phase`（`g1 | g2 | g3 | graduated`）；`PromoteCohort` 原子支持模式 A「原班直升」（建立 `continued_from_class_id` 继承链并延续班主任、常规教学班与任课）与模式 B「选科重分班」（旧阶段归档、`continued_from_class_id = NULL`、等待重分班）；`PreviewGraduateCohort` 与 `GraduateCohort` 支持毕业封存并自动释放仅在该届任教的教师占用。
  - **全校师资实时占用与跨级兼任提示**：`ListTeachersWithOccupancy` 实时由 `teaching_assignments(status='active')` 与 `class_head_teachers(status='active')` 汇总教师占用状态（`idle | active_single | active_multi | inactive`）；`DetectCrossGradeWarnings`、`AssignTeacherToGroup` 与 `SetClassHeadTeacher` 在跨在读年级兼任时返回非阻断提示；`EndTeachingAssignment` 支持期中/手动释放任课。
  - **行政班阶段实例与传承链**：`classes` 按 `(cohort_id, phase, name)` 唯一并支持 `continued_from_class_id` 递归串联；`QueryClassTrendChain` 沿继承链串联同一班集体走势，选科重分班后自动断开避免不同学生群体均分误连。
  - **复读生前溯与多级标识匹配**：`students` 按 `(cohort_id, student_no)` 建档，支持 `previous_student_id` 前溯往届档案（`SetStudentPreviousLink`）、同届重名档案合并（`MergeStudents`）、`student_no > candidate_code > name+class_name` 多级匹配（`MatchStudent`）及 `student_entry_scores` / `cohort_baseline_overrides` / `student_events`。
  - **考试快照与师生完整档案查询**：`exams` / `exam_cohorts` / `exam_participants` / `exam_scores` / `exam_cutoffs` 完整固化考时班级、类别、外语、教学班快照与各科/总分排名（`RecomputeExamRanks`）；`GetStudentProfile` 与 `GetTeacherProfile` 为阶段 2+「档案即分析」提供一站式三年学习轨迹与跨届执教履历数据。
- 产品重构设计：`docs/product-redesign.md` 已同步三项核心业务决策（教师跨年级兼任与结届释放、行政班 `(cohort_id, phase)` 实例与继承链、复读生独立建档与前溯）、Schema v1 目标模型及全校统一导航与档案即分析规格。
- 已经确认的口径：三项指标分值 40/40/20 且都不封顶、档位权重 50/40/30/20/10/8、三档赋分 70/67/64、个人得分取最高两个统计组、高考完成率 60/40、基数取中考、走班按教学班统计并按统计组合并；除第 16、17 项外的当前口径都已经确认。
- 待确认的口径：第 16 项排名并列、第 17 项分档边界，详细说明见 `docs/requirements-analysis.md` 的「待确认项说明」。
- 外部依赖：`private-data/history/` 下已有历次期中、期末及高考专项量化材料（不入库，不在文档中出现真实名称）。开发环境已安装 LibreOffice，`soffice` 位于 `/opt/homebrew/bin/soffice`；旧版 `.xls` 已实际转换并读取。
- 下一步：推进阶段 2+ 界面重构（全校统一导航、年级档案 6 页签、教师档案与跨级占用徽章、学生三年档案与复读前溯跳转、升学与结届向导）及高二、高三、高考历史数据复算验收。
- 阻塞项：导出模板的最终样式；第 16、17 项的学校确认。
- 当前打包体积：macOS 应用包低于 40 MB 上限（内嵌 Allo 与 zstd 压缩的霞鹜文楷屏幕阅读版字体）。
- 真实材料验收：高一期中工作簿黄金样例导入 965 名有效考生、135 个统计组，核算 135 条结果和 73 条个人得分，导出的工作簿可以被 excelize 重新打开；年级语文总均分、实考缺考标记、走班合并归属及各科均分提升率、单科达线率、班级完成率全部对齐，697 个计算核对点差异为 0。

## 当前风险

- 应用包体积上限是 40 MB（已计入内嵌 Allo 与压缩后的霞鹜文楷屏幕阅读版字体）。后续新增依赖前先测量体积。
- 主要运行环境是 Windows，开发环境是 macOS。每个阶段都要做 Windows 交叉编译，条件允许时在 Windows 上实际运行。
- WebDAV 服务器存在实现差异，至少验证坚果云、Nextcloud 和群晖。
- 真实工作簿存在多行表头、合并单元格、同名多版本工作表和默认 `Sheet` 名称，导入器必须按表头与用户指定识别，不能按工作表顺序猜测。
- 旧版 `.xls` 依赖 LibreOffice 无界面转换。不同版本的 LibreOffice 在公式、合并单元格和样式上的转换结果可能有差异，导入前必须校验工作表、合并单元格、公式和抽样数值；校验失败时阻止导入。
- 考核方案的 JSON 结构和公式引擎需要长期兼容。每次修改配置结构都要增加 `schemaVersion` 并保留旧版本的解释能力，旧结果按当时的版本读取。
- 真实文件名和目录名包含日期与年级，批次识别以目录层级和文件名中的学期、考试类型、年级为准，具体日期只作为版本参考。

## 新会话接手顺序

1. 读 `AGENTS.md`，按技能表选技能。
2. 读本文档，确认当前状态和阻塞项。
3. 读 `docs/requirements-analysis.md`，确认口径和待确认项。
4. 读 `docs/system-design.md`，确认当前模块、数据流、表结构和限制。
5. 读 `docs/assessment-plan.md`，确认方案结构、默认值、版本状态和核算时选择方式。
6. 需要讨论产品重构方向时读 `docs/product-redesign.md`，确认级、届、阶段、基础数据、导入引导和界面规范。
7. 读 `docs/design-roadmap.md`，确认后续阶段和目标数据模型。
8. 读 `docs/historical-data-checklist.md`，确认历史数据的目录层级、工作表角色和读取规则。
9. Excel 任务先读 `.agents/skills/excelize/SKILL.md`。

## 口径规则

- `docs/requirements-analysis.md` 的口径确认表是默认值的唯一来源。
- 未确认项按“当前口径”实现，不得在代码里固定另一套默认值。
- 修改默认值时，只改口径确认表，并且同步更新 `docs/assessment-plan.md` 和 `docs/design-roadmap.md` 的默认值。
- 考核方案是一组可版本化的配置。系统初始只有内置默认方案，所有年级统一使用；方案配置页展示并允许修改全部规则和公式；方案可以保存为新的方案或者更新当前方案，版本记录只供已有核算结果引用。
- 第 16 项排名并列和第 23 项合并组算法属于方案配置项。第 23 项已经确认，第 16 项仍待确认。
- 走班学生按学科计入实际授课的教学班，行政班不重复计人；三项指标折算为 40/40/20 分后相加；完成率默认不封顶。
- 单科达线率、均分提升率、完成率都不封顶，40、40、20 是该项满分，得分可以超过满分。
- 单科达线率按统计组计算；走班合并时分子相加，分母取统计组目标，结果只写一条并归到承担统计职责的班级行。
- 目标基础分为 0 时，计算前暂停并弹窗补齐；可以逐项补录，也可以选择本班不参与并填写原因；补齐后重算，记录审计，导出前复检。
- 考核方案的 Word、PDF、图片只用于人工阅读；系统内通过方案页的表单录入。
- 历史数据分完整复算和结果归档两种模式，必需文件不同。
- 真实工作簿按「学期 → 考试批次 → 年级或专项」识别，一个工作簿内的多个工作表按名称、表头和用户指定判定角色。
- `.xls` 文件由 LibreOffice 无界面转换为 `.xlsx` 后导入，转换失败时阻止导入；`.DS_Store` 和 `~$` 开头的临时文件直接忽略。

## 数据模型规则

以下为已落地的 Schema v1（`SchemaVersion = 1`，见 `internal/store/schema.go`）核心数据模型规则：

- **单一真理源与不兼容旧库拒绝**：数据库版本固定为 `1`；`store.Open` 检测到旧版 `batches` / `terms` / `cohort_stages` / `legacy_*` 等表或非 `v1` 版本号时直接返回 `ErrIncompatibleDatabase` 提示新建干净数据库，不做旧库迁移。
- **级（`cohorts`）与三年生命周期**：`cohorts` 以 `enrollment_year` 唯一，默认 `graduation_year = enrollment_year + 3`，持久化 `current_phase`（`g1 | g2 | g3 | graduated`）与 `current_academic_year_start`；界面展示名称由入学年份格式化为 `2026级`。阶段 1 界面通过 `EncodeStageID` / `DecodeStageID` 保持对 `(cohort_id, phase)` 的兼容视图映射。
- **行政班（`classes`）按阶段建实例与继承链**：`classes` 以 `(cohort_id, phase, name)` 唯一，并包含可选自引用外键 `continued_from_class_id`。升级模式为「原班直升」（`PromoteDirect`）时，自动在新阶段生成对应班级并填充 `continued_from_class_id`；升级模式为「选科重分班」（`PromoteReclassify`）时，旧阶段班级归档，新阶段班级 `continued_from_class_id = NULL`，防止不同学生群体均分被误连成一条趋势线。
- **教师（`teachers`）与跨级实时占用**：`teachers` 为全校级师资库（有工号按 `teacher_no` 唯一，无工号按 `(name, primary_subject)` 辅助识别）；占用状态（`idle | active_single | active_multi | inactive`）不存冗余锁布尔列，而由未毕业年级的活跃任课（`teaching_assignments.status = 'active'`）与活跃班主任（`class_head_teachers.status = 'active'`）实时聚合计算。跨在读年级分配任课或班主任时返回非阻断提示（`CrossGradeWarning`）；年级执行毕业封存（`GraduateCohort`）时，该届活跃任课与班主任自动闭合为 `ended`（`end_reason = 'graduated'`），仅在该届任教的教师即时释放为空闲。
- **教学班（`teaching_groups`）与走班映射（`teaching_group_students`）**：常规教学班（`kind = 'admin'`）关联 `admin_class_id`，学生通过 `students.current_class_id` 自动进入；仅跨行政班走班（`kind = 'walking'`）在 `teaching_group_students` 中维护显式学生映射，并通过部分唯一索引保证同学生同阶段同科至多存在一条活跃（`ended_at IS NULL`）走班记录。
- **学生（`students`）、入口基数与异动事件**：`students` 在级内按 `(cohort_id, student_no)` 建档（允许同批复读/留级生在新届独立建档以保持各届人数与基数统计纯净），通过可选外键 `previous_student_id` 前溯关联往届档案；学生单科入学成绩保存在 `student_entry_scores`，手工或导入的班级/年级汇总基数覆盖保存在 `cohort_baseline_overrides`；学籍、分班、升学、合并、毕业异动记入 `student_events`。
- **考试与不可变快照（`exams` / `exam_cohorts` / `exam_participants` / `exam_scores` / `exam_cutoffs`）**：一场考试通过 `exam_cohorts` 覆盖一个或多个级并固化 `phase_at_exam` 与 `subjects_json`；`exam_participants` 固化参考时的 `class_id`、`class_name_snapshot`、`track_snapshot`、`elective_snapshot`、`foreign_lang_snapshot`、`status_snapshot` 与各级排名；`exam_scores` 固化 `teaching_group_id_snapshot` 与单科排名；后续升学或转班不改变历史考试快照。
- **考核与结果（`assessments` / `result_runs` / `result_rows` / `result_row_teachers` / `personal_scores`）**：每个 `exam_cohort_id` 至多对应一条 `assessments` 记录；班级六档目标保存在 `assessment_class_targets`（含 `actual_count`），补录与不参与保存在 `assessment_target_adjustments`，统计组保存在 `assessment_stat_groups` 与 `assessment_stat_group_members`。每次核算写入一条 `result_runs`（`is_official` 区分正式与试算，保存 `input_hash` 与 `input_summary_json`），结果行保存 `participants_json` 与 `indicator_values_json`，并通过 `result_row_teachers` 与 `personal_scores` 绑定 `teacher_id` 与快照姓名，导出路径与哈希记入 `export_records`。
- **考核方案与版本**：`plans` 与 `plan_versions` 保存内置默认方案与派生方案，版本状态为 `draft | published | retired`；内置方案不能停用，始终保留至少一个已发布版本，旧结果继续引用历史方案版本。
- **文档同步规则**：修改数据模型时，必须同步更新 `docs/system-design.md` 的 ER 图与表结构、`docs/product-redesign.md` 的目标模型说明与 `docs/README.md`。

## 术语

| 中文 | 英文 | 说明 |
|---|---|---|
| 级 | cohort | 入学年份相同的一批学生，例如 2026级 |
| 届 | graduating class | 毕业年份相同的一批学生，例如 2029届 |
| 年级阶段 | grade level / phase | 高一（`g1`）、高二（`g2`）、高三（`g3`）及已毕业（`graduated`） |
| 升学流转 | cohort promotion | 年级跨阶段推进，分原班直升（`direct`）与选科重分班（`reclassify`） |
| 毕业封存 | cohort graduation | 将届满高三的年级归档为已毕业，同步闭合活跃任课并释放教师占用 |
| 学年 | academic year | 例如 2026-2027 学年，从 9 月到次年 8 月 |
| 行政班 | admin class | 按 `(cohort_id, phase, name)` 建实例的日常管理班级，可带 `continued_from_class_id` 继承链 |
| 教学班 | teaching group | 按学科组织的授课单位，分常规（`admin`）与走班（`walking`） |
| 教师占用状态 | teacher occupancy | 实时计算的在任状态：空闲（`idle`）、单届在任（`active_single`）、跨届兼任（`active_multi`）、已停用（`inactive`） |
| 复读前溯 | previous student link | 新届学生通过 `previous_student_id` 指向往届独立档案以查看历史成绩 |
| 考核批次 / 本次考核 | assessment | 一场考试中一个级的一次量化考核计算单位 |
| 考核方案 | assessment plan | 全部计算规则的集合 |
| 方案版本 | plan version | 某一时刻的完整配置快照 |
| 内置默认方案 | built-in default plan | 首次启动写入的方案，所有年级统一使用 |
| 派生方案 | derived plan | 从某个方案版本复制出来的方案 |
| 基数 | baseline | 均分提升率的比较基准 |
| 目标 | target | 班级完成率的分母来源 |
| 赋分 | points | 档位映射后的分数 |
| 均分提升率 | average improvement rate | 本次平均分相对基数的变化 |
| 本科达线率 | pass rate | 单科达线人数占单科目标任务的比例，折算为 40 分满分 |
| 班级完成率 | completion rate | 各档实际完成分占目标基础分的比例，折算为 20 分满分 |
| 统计组 | stat group | 单科达线率的计算单位，可以包含一个或者多个教学班 |
| 承担统计职责的班级 | lead class | 合并统计组的结果和得分所在的班级行 |

界面文字使用 `locales/en.json` 和 `locales/zh-Hans.json`，英文名称以上表为准。

## 当前不做

- 排课、学籍管理、家长端和移动端不在当前阶段。
- 考务系统接口对接不在阶段一，先读取考务系统导出的文件。
- 多端修改不做自动合并，冲突由用户选择保留哪一边。
- 不解析 Word、PDF 和图片格式的考核方案，方案通过界面表单录入。
- 不在安装包里嵌入 LibreOffice，旧版文件转换使用本机安装的 LibreOffice。

## 文档维护规则

- 规则和清单放在 `docs/`；`private-data/` 只放原始 Excel、Word、PDF 和考试目录的说明文件。
- `private-data/` 不进入版本库。文档里只写逻辑名称和目录层级，不写真实文件名、教师姓名、学生姓名、学号、身份证号、考生号和成绩。
- 考核方案的默认值以 `docs/requirements-analysis.md` 的口径确认表为准，配置结构和版本规则以 `docs/assessment-plan.md` 为准。
- 新增技能时，同时更新 `AGENTS.md` 技能表和 `.agents/skills/README.md`；需要锁定的技能更新 `skills-lock.json`。
- 完成一个阶段或者解除一个阻塞项后，更新本文档的当前状态。
- 文档修改后运行 Mermaid 解析检查和 Markdown 链接检查。
- 图表、表格和代码块中的标识符保持英文原名。

## 测试策略

- 计算引擎使用历史数据做黄金样例，输入和输出都保存为测试数据，逐项比对均分、提升率、达线率、完成率、综合得分、排名、档位和赋分。
- 考核方案使用配置样例覆盖默认方案、派生方案、版本发布、核算时选择版本、并列处理、分档边界覆盖、零目标补齐和合并组归属。
- 导入器为每一种真实文件格式保存一个脱敏样例，覆盖表头识别、合并单元格、缺考标记、前导零和多工作表。
- 旧版转换使用脱敏的 `.xls` 样例，覆盖工作表名称与数量、合并单元格、公式和抽样数值的转换校验。
- 同步使用两个临时目录模拟两台电脑，覆盖无更新、单向更新和冲突三种情况。
- 界面交互使用 `ui.NewTester`，测试固定语言。
- 性能测试使用单个一万行成绩文件，记录机器型号和冷热启动状态。

真实姓名、学号和成绩只放在 `private-data/`。测试数据使用脱敏副本或者合成数据，不提交到版本库。

## 新会话检查清单

- [ ] 已经读 `AGENTS.md` 和本文档。
- [ ] 已经从口径确认表读取默认值，未确认项没有固定另一套默认值。
- [ ] 已经读 `docs/assessment-plan.md`，方案默认值与口径确认表一致。
- [ ] Excel 任务已经读 `excelize` 技能。
- [ ] 数据模型修改同时更新了 ER 图、核心表字段和数据库迁移。
- [ ] 历史数据按 `docs/historical-data-checklist.md` 读取。
- [ ] 已经确认 `.xls` 通过 LibreOffice 转换，并校验工作表、合并单元格、公式和抽样数值；转换失败时阻止导入，没有把失败当成空数据。
- [ ] 多工作表工作簿已经按角色索引指定工作表，没有按工作表顺序猜测。
- [ ] 测试使用脱敏数据，没有把真实学生信息写进仓库。
- [ ] 没有把规则复制到 `private-data/`。
- [ ] 文档修改后 Mermaid 和链接检查通过。

## 必须遵守的约定

- 走班学生按实际授课的教学班统计；需要调整时改口径表，不在计算代码里另写默认值。
- 行政班用于学生日常管理，教学班按学科划分。
- 批次可以有多条计算结果，每条结果引用一个方案版本。
- 计算规则、公式、分值、档位、并列规则、分档边界、合并组算法和零目标处理都从方案版本读取，计算代码不写另一套默认值。
- 第 16、17 项仍待学校确认；未确认前按当前默认值试算，不在界面或者导出文件里宣称最终口径。
- 计算和存储默认在本地完成；WebDAV 是可选同步，只上传加密数据包。
- 同步数据包和原始文件的加密参数写入 `latest.json` 和 `manifest.json`。
- 备份关闭 SQLite 连接后复制文件，不使用 `VACUUM INTO`。
- 历史数据的 `说明.md` 默认放在年级目录；多年级文件放在考试目录。
- 真实文件只从 `private-data/` 读取，导入记录保存相对路径；导出和日志不写身份证号、考生号、家庭住址和电话。
- 同一工作簿存在多个版本时，带「新」「修改」「核对后」的作为当前版本，带「旧」的保留为历史版本，无法判断时由用户确认。
- `skills-lock.json` 只记录从 `transmit-bug/mygo-skills` 安装的技能；MyGo 模板技能和项目本地技能不在其中。
