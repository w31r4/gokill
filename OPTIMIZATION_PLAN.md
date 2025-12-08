# gokill 性能优化方案

本文档详细阐述了针对 `gokill` 应用在启动加载和依赖树交互方面的性能优化方案。核心策略是**预计算和缓存数据结构**，将重复的、耗时的计算从高频操作（如UI渲染）中剥离，转移到低频操作（如数据加载）中一次性完成。

## 1. 核心性能瓶颈分析

1.  **依赖树重复计算**: 在依赖树（T）模式下，每次按键（如移动光标、展开/折叠）都会触发 UI 重新渲染。渲染过程中会调用 `buildDepLines`，该函数内部又会调用 `buildChildrenMap`。`buildChildrenMap` 会**完整遍历系统中的所有进程**来构建父子关系图。当系统进程数较多时（例如上千个），每次按键都执行一次全量遍历，造成了严重的性能浪费和交互延迟。

2.  **低效的进程查找**: `findProcess` 函数通过遍历 `m.processes` 列表（时间复杂度 O(n)）来查找指定 PID 的进程。此函数在依赖树构建、祖先链生成等多个环节被频繁调用，进一步加剧了性能问题。

3.  **缓存 I/O**: 当前使用 JSON 作为缓存格式。JSON 可读性好，现阶段缓存读写开销相对整体启动时间并不是主要瓶颈，但可以作为未来的次级优化候选（例如采用更高效的编码或精简缓存内容）。本轮方案暂不修改缓存格式。

## 2. 优化实施计划

### 步骤 0：基线与 Profile（推荐但非强制）

在动手前，建议先通过简单基准或 `pprof` 采样确认瓶颈确实集中在依赖树相关逻辑上：

- 在 T 模式下模拟典型操作序列（上下移动、展开/折叠、过滤），观察 CPU 占用与延迟；
- 使用 `pprof` 或 `bench` 针对 `buildDepLines` / `buildChildrenMap` / `findProcess` 做一次粗略采样；
- 记录一份“优化前”的简单指标（例如：100 次光标移动 + 50 次展开/折叠的总耗时），便于后续对比。

### 步骤 1 & 2: 引入预计算的数据结构

**目标**: 将 O(n) 的查找和 O(n) 的父子关系构建优化为 O(1) 的查找。

**实施**:
在 `internal/tui/model.go` 的 `model` 结构体中增加两个字段：

```go
type model struct {
    // --- 核心数据 ---
    processes []*process.Item
    filtered  []*process.Item
    // 新增: PID到进程指针的快速查找映射
    pidMap map[int32]*process.Item
    // 新增: 预计算的父PID到子进程列表的映射
    childrenMap map[int32][]*process.Item

    // ... 其他字段
}
```

-   `pidMap`: 用于替代 `findProcess` 的线性扫描，实现 O(1) 时间复杂度的进程查找。
-   `childrenMap`: 用于替代在每次渲染时都重新计算的 `buildChildrenMap`，将父子关系预先计算好。

> 备注：内存开销方面，`pidMap` 和 `childrenMap` 只是对已有 `processes` 列表的索引引用，不复制实际数据，在典型场景下完全可以接受。

### 步骤 3: 在数据加载时进行预计算

**目标**: 在进程列表加载完成后，一次性填充 `pidMap` 和 `childrenMap`。

**实施**:
修改 `internal/tui/update.go` 中 `Update` 函数处理 `processesLoadedMsg` 的逻辑：

```go
case processesLoadedMsg:
    m.processes = msg.processes
    m.warnings = msg.warnings

    // --- 新增预计算逻辑 ---
    // 1. 初始化新的 map
    m.pidMap = make(map[int32]*process.Item, len(m.processes))
    m.childrenMap = make(map[int32][]*process.Item)

    // 2. 一次性遍历进程列表，填充两个 map
    for _, p := range m.processes {
        m.pidMap[p.Pid] = p
        m.childrenMap[p.PPid] = append(m.childrenMap[p.PPid], p)
    }
    // --- 预计算结束 ---

    m.filtered = m.filterProcesses(m.textInput.Value())
    return m, func() tea.Msg {
        _ = process.Save(m.processes) // 注意：此处的Save也需要修改
        return nil
    }
```

### 步骤 4: 重构代码以使用新数据结构

**目标**: 让依赖树相关的函数利用预计算的 `map` 来提升效率。

**实施**:

1.  **优化 `findProcess`**:
    修改 `internal/tui/dependency.go` 中的 `findProcess` 函数，从线性扫描改为 `map` 查找。

    ```go
    // 旧实现 (O(n))
    // func (m model) findProcess(pid int32) *process.Item {
    // 	for _, it := range m.processes {
    // 		if it.Pid == pid {
    // 			return it
    // 		}
    // 	}
    // 	return nil
    // }

    // 新实现 (O(1))
    func (m model) findProcess(pid int32) *process.Item {
        return m.pidMap[pid] // 直接从map中获取，找不到时返回nil
    }
    ```

2.  **优化依赖树构建**:
    修改 `internal/tui/dependency.go` 中的 `buildDepLines` 函数和 `internal/tui/view.go` 中的 `renderDependencyView` 函数，移除对 `buildChildrenMap` 的调用，直接使用 `m.childrenMap`。

    ```go
    // 在 buildDepLines 中
    func buildDepLines(m model) []depLine {
        // ...
        // childrenMap := m.buildChildrenMap() // 删除此行
        childrenMap := m.childrenMap // 直接使用预计算的map
        // ...
        var walk func(pid int32, prefix string, depth int)
        walk = func(pid int32, prefix string, depth int) {
            kids := childrenMap[pid] // 直接从map中获取子节点
            // ...
        }
        // ...
    }

    // 在 renderDependencyView 中
    func (m model) renderDependencyView() string {
        // ...
        // childrenMap := m.buildChildrenMap() // 删除此行
        childrenMap := m.childrenMap // 直接使用预计算的map
        for i := start; i < end; i++ {
            // ...
            hasKids := len(childrenMap[ln.pid]) > 0 // 直接查询
            // ...
        }
        // ...
    }
    ```

在这一阶段，为了降低风险，可以先在依赖树相关代码中保留一个受保护的旧实现（例如通过临时的 build tag 或小范围 switch 控制），方便在发现异常时快速回退。

### 步骤 5: 测试与全面审查

在完成以上所有代码修改后，将进行一次全面的功能和性能审查，确保：
-   应用功能（主列表、依赖树、搜索、过滤、操作）与优化前完全一致。
-   在依赖树模式下的交互（光标移动、展开/折叠、过滤）变得流畅，无明显卡顿。
-   代码逻辑清晰，新增的预计算逻辑正确无误，没有与 `processes` 状态不同步的情况。

建议增加或增强以下测试/验证手段：

- **单元测试**：为依赖树构建逻辑增加针对性的测试（例如：给定一组 `processes`，验证 `childrenMap` 和 `buildDepLines` 的输出结构与预期一致，分页/钻取行为正确）。
- **回归测试**：重点覆盖 T 模式下的各种交互组合（展开/折叠、deeper/more、过滤、切换 root 等），确保行为与优化前一致。
- **性能对比**：在同一批样本数据（例如 1k~5k 进程）上，对比优化前后 T 模式若干典型操作序列的耗时，并记录在本文件或单独的 benchmark 记录中，作为后续调整的基线。
