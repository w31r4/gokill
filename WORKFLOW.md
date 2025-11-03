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
2.  **启动TUI (`tui.Start`)**: `main` 函数调用 [`tui.Start()`](internal/tui/model.go:447)，这是UI模块的入口。
3.  **创建程序实例**: `Start` 函数内部通过 `tea.NewProgram(InitialModel(filter))` 创建一个 `Bubble Tea` 程序实例。
4.  **初始化Model (`InitialModel`)**:
    *   [`InitialModel`](internal/tui/model.go:55) 函数负责创建应用的初始状态 `model`。
    *   它会尝试从缓存文件加载上一次的进程列表 ([`process.Load()`](internal/process/cache.go:34))，这样即使用户网络慢或进程多，UI也能立即显示一些数据，避免空白。
    *   同时，它初始化了搜索框等UI组件。
5.  **执行初始化命令 (`Init`)**:
    *   `Bubble Tea` 运行时调用 [`model.Init()`](internal/tui/model.go:75) 方法。
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

`Bubble Tea` 的核心是 **Model-View-Update** 架构，所有逻辑都在 [`internal/tui/model.go`](internal/tui/model.go) 中。

*   **Model**: [`model`](internal/tui/model.go:36) 结构体，存储了应用的所有状态（完整进程列表、过滤后的列表、光标位置、错误信息等）。
*   **View**: [`View()`](internal/tui/model.go:300) 方法，根据当前的 `model` 状态，生成要在终端上显示的字符串。它是一个“纯函数”，只负责渲染。
*   **Update**: [`Update()`](internal/tui/model.go:112) 方法，是整个应用的“状态机”。它接收 **消息 (Message)**，根据消息类型更新 `model`，并可以返回一个 **命令 (Command)** 来执行新的异步操作。

**事件处理流程**:

1.  当 `getProcesses` 命令完成后，它会返回一个新的 `processesLoadedMsg` 消息。这个消息同时包含了进程列表和在获取过程中产生的警告列表。
2.  `Update` 函数接收到这个 `processesLoadedMsg`，然后：
    *   用最新的进程列表更新 `model.processes`。
    *   将警告列表存入新增的 `model.warnings` 字段。
    *   根据当前搜索词重新过滤，更新 `model.filtered`。
3.  `View` 函数在渲染时会检查 `model.warnings`。如果其中有内容，就会在UI上显示一个警告计数，告知用户部分信息可能不完整。
4.  当用户按下按键（如 `j`, `k`, `/`），`Bubble Tea` 运行时会生成一个 `tea.KeyMsg` 消息。
4.  `Update` 函数接收到 `tea.KeyMsg`，判断按键类型：
    *   如果是 `j`/`k`，则修改 `model.cursor` 的值来移动光标。
    *   如果是 `/`，则激活搜索框。
    *   如果是 `enter`，则返回一个 `sendSignal` 命令来杀死进程。
5.  每次 `Update` 函数返回后，`Bubble Tea` 都会自动调用 `View` 函数，使用更新后的 `model` 来重绘界面。

这个循环不断重复，构成了整个应用的交互逻辑。

### 4. 缓存机制

为了提升体验，`gkill` 设计了简单的缓存机制，位于 [`internal/process/cache.go`](internal/process/cache.go)。

1.  **加载 (`Load`)**: 程序启动时，`InitialModel` 会调用 `Load()` 尝试从 `~/.cache/gkill_cache.json` 文件加载上一次的进程列表。
2.  **保存 (`Save`)**: 当 `Update` 函数收到新的进程列表 (`processesLoadedMsg`) 时，它会返回一个命令，该命令在后台异步调用 `Save()`，将新的列表写入缓存文件，为下一次启动做准备。

这个机制确保了即使在获取实时数据需要几秒钟的情况下，用户也能立即看到一个可交互的界面。