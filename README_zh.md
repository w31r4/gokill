# gokill（中文文档）

**一款适用于 macOS 和 Linux 的现代交互式进程管理/杀进程工具，集成「进程溯源分析」功能。**

`gokill` 基于 Go 与 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 构建，支持模糊搜索、端口过滤以及进程依赖树（T 模式）等功能。

> 提示：本项目的交互界面目前仍以英文为主，本文档主要帮助你快速理解功能与键位。

## 「为什么这个进程在运行？」（Why Is This Running）

gokill 集成了受 [witr (why-is-this-running)](https://github.com/pranshuparmar/witr) 项目启发的**进程溯源分析**能力，帮助你理解的不仅是*什么*在运行，更是*为什么*它会存在。

### 核心能力

| 功能 | 说明 |
|------|------|
| **进程祖先链** | 从 init/systemd 到目标进程的完整父子链追溯 |
| **来源检测** | 识别进程的管理者/启动器（systemd、launchd、Docker、PM2、supervisor、cron、shell） |
| **容器感知** | 检测进程是否运行在 Docker、containerd、Kubernetes 或 LXC 容器中 |
| **Git 上下文** | 当进程从 Git 目录运行时，显示仓库名和分支 |
| **健康警告** | 提示僵尸进程、root 执行、高内存占用、长时间运行等风险 |

### 示例：进程详情视图

在选中进程后按 `i` 键，gokill 会显示：

```
PID       : 14233
User      : pm2
Command   : node index.js
Started   : 2 days ago

Why It Exists:
  systemd (pid 1) → pm2 (pid 5034) → node (pid 14233)

Source    : pm2
Git Repo  : expense-manager (main)
Warnings  : Process is running as root
```

这在故障排查时尤其有用——你可以快速理解一个运行中进程的责任链。

## 安装

确保本地已安装并配置好 Go 环境，然后执行：

```sh
go install github.com/w31r4/gokill@latest
```

或从源码构建：

```sh
git clone https://github.com/w31r4/gokill.git
cd gokill
go build
```

## 使用

在终端中运行：

```sh
gokill
```

启动后即可直接键入关键字进行模糊搜索（进程名 / PID / 用户名 / 端口号）。

### 主界面快捷键（列表视图）

| 按键 | 功能 |
| --- | --- |
| `up` / `k` | 光标上移 |
| `down` / `j` | 光标下移 |
| `/` | 进入搜索模式（再次按 `enter` / `esc` 退出） |
| `enter` | 在导航模式下向选中进程发送 SIGTERM（kill） |
| `p` | 暂停进程（SIGSTOP） |
| `r` | 恢复进程（SIGCONT） |
| `i` | 打开详情视图 |
| `P` | 切换「仅显示监听端口的进程」模式（Ports-only） |
| `T` | 打开依赖树视图（T 模式），以当前选中进程为根 |
| `?` | 打开当前模式的帮助覆盖层 |
| `ctrl+r` | 刷新进程列表 |
| `esc` | 退出搜索 / 关闭覆盖层（详情、错误、T 模式、帮助） |
| `q` / `ctrl+c` | 退出程序 |

### 详情模式

- 在主列表中选中一项后按 `i` 进入详情视图。
- 会显示：PID、用户、CPU/MEM 使用率、启动时间、命令行，以及（可选）监听端口。
- 按 `esc` 返回主列表。

详情视图中可能出现的补充字段：
- `Target`：统一的目标摘要（名称 + PID + 第一个监听端口）。
- `Service`：当检测到 systemd/launchd 管理时显示具体服务名。
- `Container`：从 cgroup 信息中解析出的容器标识。
- `Restart Count`：基于祖先链的连续重启次数估算值。
- `Context`：额外运行上下文，例如 `Socket State`（监听数量/公网）、`Resource`（RSS + 线程数）、`Files`（打开文件/FD 数量）。

### Ports-only 模式

- 按 `P`（大写）进入「仅显示监听端口的进程」模式。
- 列表将仅展示当前有监听端口的进程，并按最小端口号升序排序。
- 按 `esc` 退出该模式。

### 依赖树模式（T 模式）

按 `T` 进入以当前选中进程为根节点的全屏依赖树视图（父子间基于 PPID 关系构建）。在 T 模式中：

- 导航：
  - `up` / `down`（`j` / `k`）：移动光标。
  - `left` / `right`（`h` / `l`）或 `space`：折叠/展开节点。
    - 当光标位于 `… (deeper)` 行时，按 `right` / `l` 或 `space` 可逐步向更深一层展开子树。
    - 当光标位于 `… (N more)` 行时，按 `right` / `l` 或 `space` 在同级之间分页显示更多子进程。
- 根节点操作：
  - `enter` / `o`：将当前选中节点设为新的根。
  - `u`：将根向上移动到父进程（若存在）。
  - `a`：切换是否在上方显示祖先链（父/祖父…）。
- 过滤与筛选：
  - `/`：在 T 模式中输入过滤关键字（名字或 PID），按 `enter` / `esc` 退出输入。
  - `S`：切换「仅显示存活进程」。
  - `L`：切换「仅显示有监听端口的进程」。
- 对节点发送信号（带确认对话）：
  - `i`：查看当前节点详情。
  - `x`：kill 选中进程（SIGTERM）。
  - `p`：暂停进程（SIGSTOP）。
  - `r`：恢复已暂停进程（SIGCONT）。
- 退出：
  - `esc`：退出 T 模式，返回主列表。

在树中，如果某一节点有未展示的子依赖，会在行尾显示一个淡色的 `+`，提示还有更多依赖可展开或深入查看。

### 帮助覆盖层

- 任意模式下按 `?` 打开帮助覆盖层，显示当前模式可用的主要键位与说明。
- 再次按 `?` 或 `esc` 关闭。

## 端口扫描与环境变量

默认情况下，`gokill` 会尝试扫描进程监听的端口，这在某些系统上可能较慢或需要更高权限。

- 关闭端口扫描：

  ```sh
  export GOKILL_SCAN_PORTS=0
  ```

- 调整单个进程端口扫描超时（毫秒，默认 300ms）：

  ```sh
  export GOKILL_PORT_TIMEOUT_MS=200
  ```

关闭端口扫描后，主列表不会高亮监听进程，详情视图和依赖树中也不会显示端口信息。

## 常见错误与应对

| 错误信息 | 触发场景 | 建议处理 |
| --- | --- | --- |
| `operation not permitted` | 尝试对 root / 受保护进程发送信号而当前用户权限不足。 | 使用 `sudo` 运行 `gokill`，或仅操作属于当前用户的进程。 |
| `process with pid XXX not found` | 进程在发送信号前已经退出或 PID 被复用。 | 按 `ctrl+r` 刷新列表后重新选择。 |
| `failed to get user/create time/...`（或在 warnings 计数中体现） | `gopsutil` 无法获取某些属性。 | 通常可以忽略；以更高权限运行可减少此类告警。 |
| `connection scan timeout`（启用端口扫描时） | 端口扫描超时或被防火墙/安全策略拦截。 | 增大 `GOKILL_PORT_TIMEOUT_MS` 或关闭端口扫描。 |

在 UI 中，错误会以红色面板（Error overlay）形式显示，下方提示 `esc: dismiss • q: quit`，可以直接关闭错误继续操作，无需重启程序。

## 相关项目

- [gkill](https://github.com/heppu/gkill) - 本项目的原始版本。
- [fkill-cli](https://github.com/sindresorhus/fkill-cli) - 一款优秀的 Node.js 进程管理工具。
- [witr](https://github.com/pranshuparmar/witr) - "Why Is This Running" 项目，启发了我们的进程溯源分析功能。

## 致谢

特别感谢 [**witr (why-is-this-running)**](https://github.com/pranshuparmar/witr) 项目及其作者 [@pranshuparmar](https://github.com/pranshuparmar)。gokill 的 `internal/why` 模块深受 witr 核心理念的启发——通过构建因果链来解释进程为何存在。witr 将进程因果关系显性化的哲学——不仅回答*什么*在运行，更回答*为什么*在运行——直接影响了我们的进程详情和祖先链分析功能的设计。

> *"当系统上有东西在运行时，必有原因。witr 让这种因果关系变得清晰可见。"*

## License

MIT（与英文 README 一致）。
