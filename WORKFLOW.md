# gkill 代码工作流程说明书

本文档旨在详细解释 `gkill` 工具的核心工作流程，帮助您理解其内部的并发模型和UI架构。

## 核心架构概览

`gkill` 的核心架构可以分为三个主要部分：

1.  **并发数据处理层 (`internal/process`)**: 负责高效地从操作系统获取所有进程的详细信息。
2.  **UI 交互层 (`internal/tui`)**: 基于 `Bubble Tea` 框架，采用 **Model-View-Update (M-V-U)** 架构，负责界面的显示和用户交互。
3.  **缓存层 (`internal/process/cache.go`)**: 负责将进程列表缓存到本地文件，以加快后续启动速度。

## 整体工作流程图

```mermaid
graph TD
    subgraph "启动阶段"
        A[用户执行 gkill 命令] --> B{main.go};
        B --> C[tui.Start(filter)];
        C --> D[tea.NewProgram(InitialModel)];
        D --> E[加载缓存数据 process.Load()];
        D --> F[执行 Init 命令];
    end

    subgraph "并发数据获取 (process.GetProcesses)"
        F --> G[创建 Jobs 和 Results Channels];
        G --> H[启动多个 Worker Goroutines];
        F --> I[主 Goroutine: 分发任务到 Jobs Channel];
        H --> J{处理单个进程};
        J -->|成功| K[Item 发送到 Results Channel];
        J -->|失败| K_WARN[error 发送到 Warnings Channel];
        I --> L[关闭 Jobs Channel];
        L --> M[等待所有 Workers 完成 (wg.Wait)];
        M --> N[关闭 Results 和 Warnings Channels];
    end

    subgraph "UI 事件循环 (M-V-U)"
        N --> O[发送 processesLoadedMsg 消息];
        O --> P{Update 函数};
        P -- 更新状态 --> Q[更新 Model (包括 warnings)];
        Q -- 返回新命令 --> R[执行新命令 Cmd];
        R -- 产生新消息 --> P;
        U[用户按键] -- 产生 KeyMsg --> P;
    end
    
    subgraph "UI 渲染"
       Q -- 数据 --> T[View 函数];
       T --> V[渲染UI到终端];
    end

    subgraph "异步缓存"
        P -- 返回 Save 命令 --> S[异步保存新列表 process.Save()];
    end

    style H fill:#f9f,stroke:#333,stroke-width:2px
    style J fill:#f9f,stroke:#333,stroke-width:2px
```

## 详细步骤分解

### 1. 启动流程

1.  **入口 (`main.go`)**: 用户在命令行执行 `gkill`。`main` 函数捕获所有命令行参数作为初始的过滤条件。
2.  **启动TUI (`tui.Start`)**: `main` 函数调用 [`tui.Start()`](internal/tui/model.go:120)，这是UI模块的入口。
3.  **创建程序实例**: `Start` 函数内部通过 `tea.NewProgram(InitialModel(filter))` 创建一个 `Bubble Tea` 程序实例。
4.  **初始化Model (`InitialModel`)**:
    *   [`InitialModel`](internal/tui/model.go:55) 函数负责创建应用的初始状态 `model`。
    *   它会尝试从缓存文件加载上一次的进程列表 ([`process.Load()`](internal/process/cache.go:34))，这样即使用户网络慢或进程多，UI也能立即显示一些数据，避免空白。
    *   同时，它初始化了搜索框等UI组件。
5.  **执行初始化命令 (`Init`)**:
    *   `Bubble Tea` 运行时调用 [`model.Init()`](internal/tui/update.go:24) 方法。
    *   此方法返回一个批处理命令 `tea.Batch`，该命令会同时执行两个操作：
        *   让搜索框获得焦点。
        *   执行 `getProcesses` 命令，开始异步获取最新的进程列表。

### 2. 并发数据获取 (`process.GetProcesses`)

这是项目的性能核心，位于 [`internal/process/process.go`](internal/process/process.go:62)。它采用 **扇出/扇入 (Fan-out/Fan-in)** 的并发模式来加速I/O密集型操作。

1.  **扇出 (Fan-out)**:
    *   获取到系统所有原始进程列表后，程序会根据CPU核心数创建一组 **Worker Goroutines**。
    *   主 Goroutine 将每一个进程作为一个“任务”发送到一个带缓冲的 `jobs` channel 中。
2.  **并行处理**:
    *   每一个 Worker Goroutine 都在循环地从 `jobs` channel 中取出任务。
    *   获取进程的详细信息（如用户名、启动时间、端口等）是耗时操作，但由于多个 Worker 并行执行，总耗时被大大缩短。
3.  **扇入 (Fan-in)**:
    *   每个 Worker 完成信息处理后，会将结果发送到对应的 Channel 中：
        *   **成功**: 将 `process.Item` 结构体发送到 `results` channel。
        *   **失败**: 如果在获取进程信息时遇到非致命错误（例如权限不足），它不会再被忽略，而是将一个 `error` 对象发送到 `warnings` channel。
4.  **同步与收集**:
    *   主 Goroutine 在分发完所有任务后，会使用 `sync.WaitGroup` 等待所有 Worker 完成工作。
    *   所有 Worker 都结束后，主 Goroutine 会同时从 `results` channel 和 `warnings` channel 中收集所有的数据。
5.  **返回结果**: 最终，函数将返回三个值：成功获取的进程列表、一个包含所有警告的 `error` 切片，以及一个表示是否发生致命错误的 `error`。

### 3. UI 事件循环 (M-V-U 架构)

`Bubble Tea` 的核心是 **Model-View-Update** 架构，逻辑主要分布在：

*   **Model**: [`model`](internal/tui/model.go) 结构体，存储应用的整体状态（完整进程列表、过滤后的列表、光标位置、错误信息等），以及依赖树视图状态 `dep depViewState`（见 [`internal/tui/dependency.go`](internal/tui/dependency.go)）。
*   **View**: [`View()`](internal/tui/view.go) 方法，根据当前的 `model` 状态生成要在终端上显示的字符串。它是一个“纯函数”，只负责渲染：
    *   普通列表视图、端口面板、详情视图、错误/确认/帮助覆盖层；
    *   依赖树视图（T 模式）的实际渲染逻辑。
*   **Update**: [`Update()`](internal/tui/update.go) 方法，是整个应用的“状态机”。它接收 **消息 (Message)**，根据消息类型更新 `model`，并可以返回一个 **命令 (Command)** 来执行新的异步操作。按键更新进一步拆分为若干辅助函数，例如 `updateHelpKey` / `updateConfirmKey` / `updateDepModeKey` / `updateMainListKey` 等，便于独立理解各模式的行为。

**事件处理流程**:

1.  当 `getProcesses` 命令完成后，它会返回一个新的 `processesLoadedMsg` 消息。这个消息同时包含了进程列表和在获取过程中产生的警告列表。
2.  `Update` 函数接收到这个 `processesLoadedMsg`，然后：
    *   用最新的进程列表更新 `model.processes`。
    *   将警告列表存入新增的 `model.warnings` 字段。
    *   根据当前搜索词重新过滤，更新 `model.filtered`。
3.  `View` 函数在渲染时会检查 `model.warnings`。如果其中有内容，就会在UI上显示一个警告计数，告知用户部分信息可能不完整。
4.  当用户按下按键（如 `j`, `k`, `/`），`Bubble Tea` 运行时会生成一个 `tea.KeyMsg` 消息。
4.  `Update` 函数接收到 `tea.KeyMsg`，判断按键类型：
    *   如果是 `up/down`（或 `j/k`），则修改 `model.cursor` 或 `model.dep.cursor` 的值来移动光标（取决于是否处于 T 模式）。
    *   如果是 `/`，则激活搜索框或在 T 模式中激活树过滤。
    *   如果是 `enter/p/r`，则返回一个 `sendSignalWithStatus` 命令向选中进程发送信号；仅当命令成功返回后，才会接收一条 `signalOKMsg` 来更新 UI 中该进程的 `Status`。
    *   如果是 `P`（大写），进入“仅显示占用端口的进程”模式（ports-only）；按 `Esc` 退出该模式，用于快速巡视当前端口占用情况。
5.  每次 `Update` 函数返回后，`Bubble Tea` 都会自动调用 `View` 函数，使用更新后的 `model` 来重绘界面。

这个循环不断重复，构成了整个应用的交互逻辑。

### 4. 缓存机制

为了提升体验，`gkill` 设计了简单的缓存机制，位于 [`internal/process/cache.go`](internal/process/cache.go)。

1.  **加载 (`Load`)**: 程序启动时，`InitialModel` 会调用 `Load()` 尝试从 `~/.cache/gkill_cache.json` 文件加载上一次的进程列表。
2.  **保存 (`Save`)**: 当 `Update` 函数收到新的进程列表 (`processesLoadedMsg`) 时，它会返回一个命令，该命令在后台异步调用 `Save()`，将新的列表写入缓存文件，为下一次启动做准备。

这个机制确保了即使在获取实时数据需要几秒钟的情况下，用户也能立即看到一个可交互的界面。

## 新增与可配置点

### A. Warnings 聚合避免阻塞

在并发采集进程信息时，每个进程可能产生多条非致命警告。为了避免 `warnings` 通道因容量不足导致 worker 阻塞：

- 使用小容量 `warnings` 通道并启动后台聚合 goroutine 持续读取；
- 在完成后关闭通道并 `wait` 聚合器，统一返回 `[]error`；
- 相关实现：
  - `internal/process/process.go:95-111` 创建与聚合；
  - `internal/process/process.go:188-199` 关闭并等待；
  - 返回结果：`internal/process/process.go:206-207`。

### B. 端口扫描可开关

全量扫描端口性能开销大，且在部分系统需要更高权限。现通过环境变量控制：

- `GOKILL_SCAN_PORTS` 为空/`1`/`true`/`yes` 时启用扫描；其他值禁用；
- 列表与详情视图都受该开关控制；
- 相关实现：
  - 列表采集时是否扫描：`internal/process/process.go:126-156`；
  - 详情视图端口：`internal/process/process.go:264-268`；
  - 开关函数：`internal/process/process.go:324-333`。

同时，端口扫描为 I/O 密集型操作，已做两项强化：

- 提升并发（最多 CPU×2，且不超过进程数）：`internal/process/process.go`（worker 数计算）；
- 为每个进程的连接采集设置短超时（默认 300ms，可通过 `GOKILL_PORT_TIMEOUT_MS` 覆盖）：
  - 采集：`internal/process/process.go:getProcessPortsCtx`，`GetProcesses` 与 `GetProcessDetails` 中使用 `context.WithTimeout` 调用。

### C. 信号成功后再更新 UI 状态

避免信号发送失败导致 UI 状态错误：

- 新增 `signalOKMsg`，仅在 `SendSignal` 成功后回传，`Update` 收到后更新 `Item.Status`；
- 新增 `sendSignalWithStatus` 命令封装（代替直接改状态）；
- 相关实现：
  - 新消息与处理：`internal/tui/model.go:38-42,156-165`；
  - 命令函数：`internal/tui/model.go:318-326`；
  - 按键处理：`internal/tui/model.go:204-219`。

### D. CPU 百分比二次采样

`gopsutil` 的 `CPUPercent()` 首次采样常为 0。为更可信的展示：

- 若首次采样为 0，则 `sleep 200ms` 后再采样一次；
- 相关实现：`internal/process/process.go:231-240`。

### E. 依赖树视图 (T 模式)

依赖树视图用于展示基于 PPID 关系构建的进程树，并提供按需展开/分页、过滤和信号操作能力。其实现分为三部分：

1.  **状态结构 (`depViewState`)**  
    为避免在主 `model` 上散落过多与 T 模式相关的字段，依赖树状态被聚合在 [`depViewState`](internal/tui/dependency.go) 中，并作为 `model.dep` 挂载：

    - `mode bool`：是否处于 T 模式。
    - `rootPID int32`：当前树的根进程 PID。
    - `expanded map[int32]depNodeState`：每个节点的展开/分页状态。
    - `cursor int`：当前选中行（在扁平化后的依赖树行列表中的索引）。
    - `showAncestors bool`：是否显示祖先进程链。
    - `aliveOnly bool`：是否仅显示存活进程。
    - `portsOnly bool`：是否仅显示有监听端口的进程。

2.  **结构构建与过滤 (`dependency.go`)**  
    [`internal/tui/dependency.go`](internal/tui/dependency.go) 负责把原始进程列表转换为可渲染的树形行：

    - `buildChildrenMap`：基于 `PPid` 构建父 PID → 子进程列表的映射。
    - `buildDepLines`：从 `dep.rootPID` 出发，结合 `dep.expanded`、`dependencyTreeDepth` 等配置，将树形结构扁平化为 `[]depLine`：
        - 支持每个父节点的分页 (`page`) 与深度扩展 (`depthExtend`)；
        - 在达到深度限制时插入 `… (deeper)` 行，在超出子数量限制时插入 `… (N more)` 行。
    - `applyDepFilters`：在 `buildDepLines` 的结果上，按照当前的过滤条件筛选：
        - 文本/数字过滤（进程名、文本行内容或 PID）；
        - `aliveOnly` / `portsOnly` 开关。

3.  **按键交互与渲染 (`update.go` / `view.go`)**  

    - 进入/退出 T 模式：
        - 在主列表中按 `T`，由 [`updateMainListKey`](internal/tui/update.go) 将 `dep.mode` 置为 `true`，并以当前选中进程作为 `dep.rootPID`；
        - 在 T 模式中按 `esc`，由 [`updateDepModeKey`](internal/tui/update.go) 将 `dep.mode` 置为 `false`，并清理 `dep.expanded` 等状态。
    - T 模式按键处理：
        - [`updateDepModeKey`](internal/tui/update.go) 负责处理 T 模式下的所有键：方向键、`space` 折叠/展开、`S`/`L` 过滤、`u`/`a` 根和祖先链、`i`/`x`/`p`/`r` 细节与信号操作等；
        - 未处理的按键会返回 `handled=false`，由外层逻辑回退到主列表的通用处理。
    - 渲染：
        - [`renderDependencyView`](internal/tui/view.go) 使用 `depViewState` 与 `buildDepLines` 的结果绘制依赖树视图：
            - 根据 `dep.cursor` 计算可视窗口（viewport），只渲染当前需要显示的行；
            - 根据进程状态（存活/已杀/已暂停）选择不同样式，并在有隐藏子依赖时在行尾追加一个淡色的 `+` 提示；
            - 在顶部可选显示祖先链（由 `buildAncestorLines` 提供），底部附带当前过滤状态的提示（`[filter: ...] [alive-only] [listening-only]`）。
        - [`renderHelpView`](internal/tui/view.go) 在 `dep.mode` 为 `true` 时展示 T 模式专属的帮助文案，列出可用的按键和操作说明。
