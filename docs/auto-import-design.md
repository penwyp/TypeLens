# 自动导入设计说明

## 1. 背景

TypeLens 当前已经具备词典管理能力，包括：

- 手动新增词条
- 从文本文件导入词条
- 重置词典
- 查看历史记录

现有模型偏人工操作，不适合从本机常用 AI 工具的历史输入中持续提取项目词、专有词、冷门词和中英混合词。为此，需要在词典页右上角新增一个“自动导入”能力，用于从本地 AI 平台工作目录中自动扫描用户输入，提取候选词，完成预览、确认和异步导入。

本次设计的目标不是做一个临时旁路工具，而是在现有词典域内部新增一条完整、可维护、可扩展的自动导入链路。

## 2. 目标

### 2.1 功能目标

- 支持从 `Codex` 与 `Claude` 两个平台的本地工作目录扫描对话数据。
- 仅提取用户输入内容，不提取 assistant 输出内容。
- 自动识别英文专有词、复合词、项目名、中文词和中英混合词。
- 基于现有词典做差集过滤，生成候选预览。
- 支持用户预览、二次确认、最终确认导入。
- 点击确认后立即反馈“已成功导入 XXX”，并让词条在本地词典界面直接可见。
- 实际远端词典上传走后台异步流程，不阻塞前端交互。

### 2.2 架构目标

- 保持当前 Wails + React + Go service 的分层结构。
- 复用现有词典域的导入能力、日志能力和远端词典访问能力。
- 避免在 `frontend/src/App.tsx` 中继续堆叠复杂状态。
- 新功能具备明确的扫描、解析、提取、过滤、暂存、同步边界。

## 3. 非目标

以下内容不属于本次范围：

- 不做 assistant 回复内容的语义分析。
- 不做重量级 NLP、外部词典服务或在线分词服务依赖。
- 不做跨设备自动同步。
- 不做全量历史长期索引系统。
- 不做平台无关的“万能 JSONL 自动识别器”。

## 4. 真实输入背景

用户给定的两个默认平台目录为：

- `Codex: ~/.codex`
- `Claude: ~/.claude`

基于当前机器实际目录观察，真实可扫描数据并不是统一命名的 `featured*.jsonl`，而是平台各自的历史与会话 JSONL：

### 4.1 Codex

常见文件形态：

- `~/.codex/history.jsonl`
- `~/.codex/sessions/**/*.jsonl`

### 4.2 Claude

常见文件形态：

- `~/.claude/history.jsonl`
- `~/.claude/projects/**/*.jsonl`

因此，产品上的“featured JSONL”不能在代码实现里硬编码成单一文件名，而应抽象成“平台预设扫描规则 + 工作目录可编辑”的模型。

## 5. 用户需求拆解

原始需求可以拆成五个阶段：

1. 点击“自动导入”打开 Dialog，展示平台和对应工作目录，允许用户编辑。
2. 程序扫描目录下平台相关 JSONL 文件。
3. 从 JSONL 的 user content 中提取候选词。
4. 基于已有词典过滤并展示预览。
5. 用户点击 `Double Check`，再点击确认导入；前端立即反馈成功，本地立刻可见，后台异步批量上传。

这里有两个关键产品约束：

- “即时反馈”与“实际上传完成”必须解耦。
- “本地直接可见”不能只靠 toast，必须有本地暂存词条机制。

## 6. 交互设计

## 6.1 入口

位置：词典页右上角按钮区，新增按钮：

- `自动导入`

该按钮属于词典域核心操作，应与 `重置`、`导入`、`新增` 并列。

## 6.2 Dialog 分步流程

自动导入采用单个 Dialog 的多阶段流转，而不是多个分散弹窗。

### 第一步：来源配置

展示两个来源卡片：

- `Codex`
- `Claude`

每张卡片包含：

- 平台名称
- 工作目录输入框
- 平台扫描规则摘要
- 开关或勾选框，用于启用/禁用该来源

默认值：

- Codex：`~/.codex`
- Claude：`~/.claude`

扫描规则摘要示例：

- Codex：`history.jsonl + sessions/**/*.jsonl`
- Claude：`history.jsonl + projects/**/*.jsonl`

底部主按钮：

- `开始扫描`

### 第二步：预览结果

扫描完成后展示预览：

- 扫描到的文件数
- 解析出的用户输入条数
- 提取到的候选词数
- 过滤后待导入词数

预览列表每项展示：

- 词本身
- 来源平台
- 命中次数
- 示例上下文片段
- 勾选状态

支持操作：

- 搜索词条
- 单项取消
- 全选 / 全不选

底部主按钮：

- `Double Check`

### 第三步：最终确认

进入最终确认态后，不再修改扫描结果，只做确认。

展示内容：

- 即将导入的词数
- 来源平台分布
- 失败后会进入后台重试或保留本地待同步状态的说明

底部按钮：

- `返回预览`
- `确认导入`

## 6.3 导入完成反馈

当用户点击 `确认导入` 后，前端必须立即做两件事：

1. toast：`已成功导入 XXX 个词，后台同步中`
2. 词典列表立即展示这些词

这里“已成功导入”表达的是“已成功导入到本地待同步词典视图”，不是“远端 API 已全部上传完成”。

## 6.4 后台同步反馈

后台异步上传不阻塞用户继续使用。

前端对后台同步的感知方式：

- 词条本身有轻量状态标识：`同步中` / `失败`
- 可选地在 Dialog 或页面状态栏显示后台同步进度

不要求用户停留在 Dialog 中等待所有网络请求完成。

## 7. 领域模型

建议新增以下核心领域对象。

## 7.1 自动导入来源

```text
AutoImportSource
- platform: codex | claude
- enabled: bool
- workdir: string
```

## 7.2 扫描结果

```text
AutoImportScanResult
- scanned_files: int
- parsed_messages: int
- raw_candidates: int
- filtered_candidates: int
- items: []AutoImportCandidate
```

## 7.3 候选词

```text
AutoImportCandidate
- term: string
- normalized_term: string
- platform: codex | claude
- hits: int
- examples: []string
- selected: bool
```

## 7.4 本地暂存词

```text
PendingDictionaryWord
- term: string
- platform: codex | claude
- example: string
- status: pending | syncing | synced | failed
- created_at: string
```

## 8. 前端架构

## 8.1 当前问题

当前 `frontend/src/App.tsx` 已经同时承载：

- 视图切换
- 词典操作
- 历史记录查询
- 多个 Dialog
- 通知
- 日志状态

自动导入如果继续堆叠到同一个文件，状态边界会进一步恶化，后续很难维护。

## 8.2 前端拆分原则

本次不做全局大重构，但应在词典域内完成局部模块化。

建议新增目录：

```text
frontend/src/features/dictionary/
  DictionaryView.tsx
  AutoImportDialog.tsx
  autoImport.types.ts
  useAutoImport.ts
```

## 8.3 前端职责拆分

### `App.tsx`

仅保留：

- 整体布局
- 视图切换
- 顶层 notice
- 词典和历史两个主视图挂载

### `DictionaryView.tsx`

负责：

- 词典列表展示
- 新增 / 删除
- 手工导入 / 重置入口
- 自动导入入口
- 本地待同步词与远端词典合并展示

### `AutoImportDialog.tsx`

负责：

- 来源配置步骤
- 预览步骤
- Double Check 步骤
- 提交确认

### `useAutoImport.ts`

负责：

- Dialog 内部状态机
- 扫描请求
- 预览结果管理
- 最终确认提交

## 8.4 前端状态设计

自动导入至少分为两层状态。

### 预览态

- 来源配置
- 当前步骤
- 是否正在扫描
- 扫描结果
- 勾选项
- 是否完成 double check

### 词典展示态

- 远端词典词条
- 本地暂存未同步词条
- 后台同步状态

最终词典展示列表为：

```text
显示词典 = 远端词典 + 本地暂存词
```

如果只把待同步词放在 Dialog 的局部内存里，Dialog 关闭后就不可见，不满足需求。

## 9. 后端架构

## 9.1 分层原则

遵循当前项目已有分层：

- `app.go`：Wails 暴露接口
- `internal/service`：业务编排
- `pkg/typeless`：底层领域逻辑与工具能力

## 9.2 新增后端模块建议

### `app.go`

新增 Wails 接口：

- `ScanAutoImportSources(request)`
- `ConfirmAutoImport(request)`
- `ListPendingImportedWords()`

### `internal/service/auto_import.go`

负责业务编排：

- 校验来源配置
- 调用平台扫描器
- 调用候选词提取器
- 与现有词典做差集过滤
- 写入本地暂存
- 启动后台异步同步任务

### `pkg/typeless/auto_import_scanner.go`

负责：

- 平台文件发现
- 文件路径过滤
- 扫描上限控制

### `pkg/typeless/auto_import_parser.go`

负责：

- 平台 JSONL 逐行解析
- user content 抽取

### `pkg/typeless/auto_import_extractor.go`

负责：

- 候选词提取
- 归一化
- 打分
- 噪音过滤

### `pkg/typeless/dictionary_staging.go`

负责：

- 本地暂存词读写
- 状态更新
- 后台同步状态持久化

## 9.3 为什么需要本地暂存层

本地暂存层不是可选优化，而是满足需求的必要结构，因为它解决了三个问题：

1. 点击确认后必须立即在词典页可见。
2. 远端上传是异步的，存在失败和重试。
3. 应用关闭后重新打开，未同步词不能丢。

因此需要一个持久化的本地待同步仓库，而不是纯内存队列。

## 10. 平台解析规则

## 10.1 总原则

- 只解析用户输入。
- 不解析 assistant 输出。
- 不依赖平台所有历史格式兼容，只覆盖当前主路径上的主流结构。
- 解析失败的单行记录跳过并计数，不因为单条坏数据中断全量扫描。

## 10.2 Codex 解析规则

### 数据来源

- `~/.codex/history.jsonl`
- `~/.codex/sessions/**/*.jsonl`

### 提取规则

#### `history.jsonl`

按行读取 JSON，提取：

- `text`

该文件实际表现为轻量历史输入记录，可以直接视为用户输入内容。

#### `sessions/**/*.jsonl`

按行读取 JSON，识别当前主结构中的用户消息：

- `type=response_item` 且其中 `payload.role=user`
- 或 `type=event_msg` 且事件为用户消息

只保留真正的用户输入文本字段。

## 10.3 Claude 解析规则

### 数据来源

- `~/.claude/history.jsonl`
- `~/.claude/projects/**/*.jsonl`

### 提取规则

#### `history.jsonl`

按行读取 JSON，提取：

- `display`

该字段反映用户输入展示文本。

#### `projects/**/*.jsonl`

按行读取 JSON，仅接受：

- `type=user`
- 且 `message.role=user`

再从 `message.content` 中抽取文本内容。

需要忽略：

- assistant 消息
- tool use 结果
- local command caveat 等非真实业务输入

## 11. 扫描规则

## 11.1 平台预设扫描器

每个平台由预设扫描器负责，不让用户直接填写 glob 规则。

原因：

- 平台布局是强业务知识，不应泄漏到用户配置。
- 便于后续版本统一调整扫描策略。
- 可以避免误扫过深目录和无关文件。

## 11.2 扫描范围

默认策略：

- 只扫描平台预设路径下的目标 JSONL
- 只递归进入受控目录，如 `sessions/`、`projects/`
- 排除备份目录、插件缓存目录、测试夹具目录、明显无关目录

例如：

- Codex 排除 `merge_backups/`
- Claude 排除 `plugins/cache/`、测试 fixture

## 11.3 限流与上限

为了避免首次扫描极慢，建议增加保护：

- 单次最多扫描 N 个文件
- 单文件最多处理 M 行
- 单条文本最大长度截断

这些限制是性能边界，不改变产品语义。

## 12. 提取算法设计

## 12.1 设计原则

算法目标是“低成本提取高价值词”，不是追求语言学最优。

要求：

- 通用
- 简洁
- 快速
- 对技术语料友好

## 12.2 处理流程

整体流程：

```text
原始文本
-> 归一化
-> token 粗提取
-> 复合词拆解
-> 中文词片提取
-> 候选词打分
-> 噪音过滤
-> 差集过滤
-> 预览输出
```

## 12.3 文本归一化

预处理规则：

- 去除首尾空白
- 合并重复空白
- 保留原文本用于示例展示
- 生成标准化文本用于匹配与去重

标准化目标：

- 英文比较时大小写不敏感
- 连字符、下划线、点号、斜杠可参与复合技术词识别

## 12.4 英文与技术词提取

优先提取以下模式：

```text
[A-Za-z][A-Za-z0-9._/-]*
```

适合捕获：

- 项目名
- 包名
- 模块名
- 复合英文词
- 技术缩写

示例：

- `TypeLens`
- `agent_os`
- `sub2api`
- `TiDB`
- `ClaudeProbe`
- `openai-docs`

## 12.5 复合词拆解

对提取出的英文技术词进一步拆解：

- camelCase / PascalCase
- snake_case
- kebab-case
- dotted.name

同时保留：

- 原复合词整体
- 拆解后的有意义子词

例如：

- `ClaudeProbe` -> `ClaudeProbe`, `Claude`, `Probe`
- `agent_os` -> `agent_os`, `agent`, `os`

这样既能保留专有名，也能辅助召回子词。

## 12.6 中文与中英混合提取

对中文文本，不做重型分词，采用轻量规则：

- 提取长度 2-8 的连续中文片段
- 对中英相邻混合片段保留整体候选

例如：

- `词典导入`
- `自动导入`
- `TiDB集群`

中文提取要严格控噪，否则会把大量常见短语引入词典。

## 12.7 候选词打分

候选词可基于如下因子评分：

- 出现频次
- 跨文件出现次数
- 是否为复合专有词
- 是否包含大写模式或数字组合
- 是否接近路径名、项目名、仓库名

评分不是为了暴露给用户，而是为了排序预览结果，让高价值词优先出现。

## 12.8 噪音过滤

至少过滤掉以下内容：

- 已有词典中已存在的词
- 纯数字
- 过短英文单词
- 常见英文停用词
- URL
- 绝对路径
- 明显命令参数
- 单字中文
- 超长异常 token

## 12.9 词典差集过滤

在展示预览前，必须先读取现有词典并做标准化差集比较：

```text
candidate_terms - existing_dictionary_terms
```

比较时需使用与词典域一致的 term normalize 规则，避免“大小写不同但实际重复”的情况。

## 13. 导入与同步模型

## 13.1 提交确认后的执行顺序

用户点击 `确认导入` 后，执行顺序如下：

1. 将选中候选词写入本地暂存仓库，状态为 `pending`
2. 立即返回前端成功结果
3. 前端将这些词加载到词典展示列表
4. 后台 goroutine 启动异步同步
5. 状态依次流转：
   - `pending`
   - `syncing`
   - `synced` 或 `failed`

## 13.2 为什么不直接同步成功后再展示

如果必须等远端 API 成功再显示，会带来两个问题：

- 与“前端即时反馈”冲突
- 网络抖动会让交互变得不可预测

因此，本地暂存先行是架构必需项，不是视觉优化。

## 13.3 后台同步策略

后台同步可复用现有词典导入客户端：

- 并发受控
- 单词级失败隔离
- 已存在词视作可跳过失败

同步完成后更新本地暂存状态。

## 14. 前后端接口草案

## 14.1 扫描接口

```text
ScanAutoImportSources(request) -> AutoImportScanResult
```

请求包含：

- 来源列表
- 是否启用各来源
- 工作目录

## 14.2 确认导入接口

```text
ConfirmAutoImport(request) -> ConfirmAutoImportResult
```

请求包含：

- 已选候选词列表

返回包含：

- accepted_count
- 本地暂存词列表

## 14.3 本地待同步词接口

```text
ListPendingImportedWords() -> []PendingDictionaryWord
```

用于应用启动和词典刷新时恢复本地可见状态。

## 15. 事件设计

现有项目已经有：

- `typelens:dictionary-log`

自动导入建议新增：

- `typelens:auto-import-progress`
- `typelens:auto-import-finished`

用途：

- 前端感知后台同步进度
- 同步完成时刷新词典或更新本地状态

## 16. 失败处理策略

## 16.1 扫描阶段失败

- 单个平台目录不存在：标记该平台错误，但不阻塞另一个平台
- 单个文件损坏：跳过该文件并计数
- 单条 JSON 损坏：跳过该条并计数

## 16.2 导入阶段失败

- 单词上传失败：保留 `failed`
- 已存在词错误：转为跳过或视为已同步
- 网络中断：保留本地记录，待后续重试

## 17. 测试建议

本次功能风险集中在四类：

- 平台 JSONL 解析正确性
- 词提取与过滤正确性
- 本地暂存与远端词典合并逻辑
- 后台同步状态流转

建议测试覆盖：

### 单元测试

- Codex `history.jsonl` 解析
- Codex `sessions/*.jsonl` 用户消息解析
- Claude `history.jsonl` 解析
- Claude `projects/*.jsonl` 用户消息解析
- 候选词提取
- 差集过滤
- 本地暂存状态流转

### 轻量集成测试

- 扫描 -> 预览 -> 确认 -> 后台同步的服务层流程

## 18. 分阶段实现建议

为了降低一次性改动风险，建议实现顺序如下：

1. 写入本设计文档
2. 抽离词典前端视图模块
3. 实现后端扫描、解析、提取、预览接口
4. 实现前端自动导入 Dialog
5. 实现本地暂存仓库
6. 实现后台异步同步
7. 补单元测试

## 19. 最终设计结论

自动导入的本质不是“再加一个导入按钮”，而是为词典域新增一条独立的半异步数据导入链路。它需要同时满足：

- 平台数据扫描
- 用户输入抽取
- 候选词生成
- 预览与 Double Check
- 本地立即可见
- 后台异步同步

因此，正确实现方式是：

- 前端采用分步 Dialog 和词典域局部模块化
- 后端新增自动导入编排层
- 平台解析按 Codex / Claude 分治
- 使用轻量高效的规则型提取算法
- 通过本地暂存仓库实现“即时反馈 + 最终同步”的统一模型

这份文档作为后续编码的直接依据，后续实现以此为准。
