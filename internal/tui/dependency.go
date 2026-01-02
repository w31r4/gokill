package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/w31r4/gokill/internal/process"
)

// dependency.go 文件包含了实现“依赖树视图”（T-Mode）的所有核心逻辑。
//
// 核心思想:
// 为了在 Bubble Tea 的线性列表模型中渲染一个可交互的树形结构，我们采用了一种“扁平化”的策略。
// 1. 内存中的进程列表首先被转换成一个父子关系的映射 (`childrenMap`)。
// 2. 一个递归函数 (`walk`) 遍历这个映射，根据节点的展开/折叠状态、分页和深度限制，
//    动态地构建出一个扁平的 `[]depLine` 列表。
// 3. `depLine` 结构体不仅包含了要显示的文本（已预格式化好缩进和连接符），还存储了
//    其自身的PID、父PID、深度等元数据，这使得后续的事件处理（如光标移动、展开/折叠）变得简单。
// 4. `View` 函数最终只需遍历这个扁平化的 `[]depLine` 列表来渲染UI，而不需要在渲染时再处理复杂的树逻辑。
//
// 这种方法将复杂的树形数据结构转换为了简单的线性数据结构，完美地适配了 Bubble Tea 的 M-V-U 架构。

// 此处定义了依赖树视图的几个关键配置常量，它们在 `update` 和 `view` 逻辑中共享。
const (
	// dependencyTreeDepth 定义了依赖树默认递归展示的深度。
	dependencyTreeDepth = 3
	// dependencyTreeChildLimit 定义了当一个节点有大量子节点时，一次最多显示的数量（用于分页）。
	dependencyTreeChildLimit = 5
	// ancestorChainLimit 定义了在显示祖先链时，最多向上追溯的层级。
	ancestorChainLimit = 6
)

// depViewState 结构体聚合了所有与“依赖树视图”（T模式）相关的状态。
// 将这些状态封装在一起，而不是散落在主 `model` 结构中，有助于保持代码的组织性和清晰性。
type depViewState struct {
	mode          bool                   // `true` 表示当前处于依赖树视图模式。
	rootPID       int32                  // 依赖树的根进程PID。
	expanded      map[int32]depNodeState // 记录树中每个节点（由PID标识）的展开/折叠状态和分页信息。
	cursor        int                    // 光标在当前扁平化依赖树列表中的位置。
	showAncestors bool                   // 是否显示根进程的祖先链。
	aliveOnly     bool                   // 是否只显示存活的进程。
	portsOnly     bool                   // 是否只显示监听端口的进程。
}

// depNodeState 记录了依赖树中单个节点的可交互状态。
type depNodeState struct {
	expanded    bool // 节点是否被用户展开以显示其子节点。
	page        int  // 当子节点数量超过 `dependencyTreeChildLimit` 时，记录当前显示的是第几页。
	depthExtend int  // 用户可以对特定节点进行“钻取”，这个字段记录了额外的钻取深度。
}

// depLine 代表了在屏幕上显示的一行扁平化的依赖树。
// 整个树形结构被转换成一个 `depLine` 切片，以便于在列表中渲染和导航。
type depLine struct {
	pid      int32  // 这一行对应的进程PID。如果为0，则表示这是一个提示行（如“... more”）。
	parent   int32  // 父进程的PID，用于交互（如折叠父节点）。
	isMore   bool   // `true` 表示这是一个提示行（“... more”或“... deeper”）。
	text     string // 准备在屏幕上渲染的完整行文本，包括缩进和连接符。
	depth    int    // 这一行在树中的深度。
	isDeeper bool   // `true` 表示这是一个“... deeper”提示行，与分页的“... more”相区分。
}

type depLineBuilder struct {
	model       model
	childrenMap map[int32][]*process.Item
	lines       []depLine
}

// buildDepLines 是一个核心函数，它根据当前的 `model` 状态（特别是 `m.dep`），
// 将内存中的进程父子关系动态地构建成一个扁平化的、可供 `View` 函数直接渲染的 `[]depLine` 列表。
// 这个函数处理了节点的展开/折叠、子节点的分页以及递归深度的限制。
func buildDepLines(m model) []depLine {
	root := m.findProcess(m.dep.rootPID)
	if root == nil {
		return nil
	}

	builder := depLineBuilder{
		model:       m,
		childrenMap: m.buildChildrenMap(),
	}

	builder.addRoot(root)
	builder.ensureRootExpanded(root.Pid)
	builder.walk(root.Pid, "", 0)

	return builder.lines
}

func (b *depLineBuilder) addRoot(root *process.Item) {
	b.lines = append(b.lines, depLine{pid: root.Pid, parent: 0, text: fmt.Sprintf("%s (%d)", root.Executable, root.Pid), depth: 0})
}

func (b *depLineBuilder) ensureRootExpanded(pid int32) {
	st, ok := b.model.dep.expanded[pid]
	if ok && st.expanded {
		return
	}
	b.model.dep.expanded[pid] = depNodeState{expanded: true, page: 1}
}

func (b *depLineBuilder) walk(pid int32, prefix string, depth int) {
	kids := b.sortedChildren(pid)
	if len(kids) == 0 {
		return
	}
	if !b.model.dep.expanded[pid].expanded && pid != b.model.dep.rootPID {
		return
	}

	show := b.pageLimit(pid, len(kids))
	for i := 0; i < show; i++ {
		child := kids[i]
		last := b.isLastChild(i, show, len(kids))
		b.appendChild(pid, child, prefix, depth, last)
	}

	if show < len(kids) {
		b.appendMoreLine(prefix, pid, len(kids)-show, depth)
	}
}

func (b *depLineBuilder) sortedChildren(pid int32) []*process.Item {
	kids := b.childrenMap[pid]
	if len(kids) == 0 {
		return nil
	}
	sort.Slice(kids, func(i, j int) bool {
		if kids[i].Executable == kids[j].Executable {
			return kids[i].Pid < kids[j].Pid
		}
		return kids[i].Executable < kids[j].Executable
	})
	return kids
}

func (b *depLineBuilder) pageLimit(pid int32, total int) int {
	page := b.model.dep.expanded[pid].page
	if page <= 0 {
		page = 1
	}
	limit := dependencyTreeChildLimit * page
	if total > limit {
		return limit
	}
	return total
}

func (b *depLineBuilder) isLastChild(index, show, total int) bool {
	return index == show-1 && show == total
}

func (b *depLineBuilder) appendChild(parent int32, child *process.Item, prefix string, depth int, last bool) {
	connector := branchSymbol(last)
	line := fmt.Sprintf("%s%s %s (%d)", prefix, connector, child.Executable, child.Pid)
	b.lines = append(b.lines, depLine{pid: child.Pid, parent: parent, text: line, depth: depth + 1})

	nextPrefix := b.childPrefix(prefix, last)
	b.appendChildLines(parent, child, nextPrefix, depth)
}

func (b *depLineBuilder) childPrefix(prefix string, last bool) string {
	if last {
		return prefix + "   "
	}
	return prefix + "│  "
}

func (b *depLineBuilder) appendChildLines(parent int32, child *process.Item, nextPrefix string, depth int) {
	if b.allowDepth(parent, depth) {
		b.walk(child.Pid, nextPrefix, depth+1)
		return
	}
	if len(b.childrenMap[child.Pid]) == 0 {
		return
	}
	b.appendDeeperLine(nextPrefix, parent, depth+1)
}

func (b *depLineBuilder) allowDepth(pid int32, depth int) bool {
	allowed := dependencyTreeDepth - 1 + b.model.dep.expanded[pid].depthExtend
	return depth < allowed
}

func (b *depLineBuilder) appendDeeperLine(prefix string, parent int32, depth int) {
	moreLine := fmt.Sprintf("%s└─ … (deeper)", prefix)
	b.lines = append(b.lines, depLine{pid: 0, parent: parent, isMore: true, isDeeper: true, text: moreLine, depth: depth})
}

func (b *depLineBuilder) appendMoreLine(prefix string, parent int32, more int, depth int) {
	connector := branchSymbol(true)
	moreLine := fmt.Sprintf("%s%s … (%d more)", prefix, connector, more)
	b.lines = append(b.lines, depLine{pid: 0, parent: parent, isMore: true, isDeeper: false, text: moreLine, depth: depth})
}

// applyDepFilters 函数接收一个扁平化的 `depLine` 列表，并根据 T 模式下的各种过滤条件
// (文本搜索、仅存活、仅监听端口) 对其进行二次筛选。
func applyDepFilters(m model, lines []depLine) []depLine {
	if len(lines) == 0 {
		return lines
	}

	term := strings.TrimSpace(m.textInput.Value())
	hasTerm := term != ""
	var out []depLine
	for _, ln := range lines {
		// 提示行（pid为0）的处理：在有搜索词时，为了减少干扰，通常会隐藏它们。
		if ln.pid == 0 {
			if hasTerm {
				continue
			}
			out = append(out, ln)
			continue
		}

		// 查找行对应的进程项。
		it := m.findProcess(ln.pid)
		if it == nil {
			continue // 如果找不到，则忽略该行。
		}

		// 应用“仅存活”过滤器。
		if m.dep.aliveOnly && it.Status != process.Alive {
			continue
		}
		// 应用“仅监听端口”过滤器。
		if m.dep.portsOnly && len(it.Ports) == 0 {
			continue
		}

		// 应用文本过滤器。
		if hasTerm {
			// 搜索词不区分大小写地匹配行文本。
			match := strings.Contains(strings.ToLower(ln.text), strings.ToLower(term))
			if !match {
				// 如果文本不匹配，尝试将搜索词解析为PID进行精确匹配。
				if termPid, err := strconv.Atoi(term); err == nil {
					if int32(termPid) == it.Pid {
						match = true
					}
				}
			}
			if !match {
				continue // 如果所有匹配都失败，则过滤掉该行。
			}
		}
		out = append(out, ln)
	}
	return out
}

// buildChildrenMap 是一个辅助函数，它遍历 `m.processes` 列表，并构建一个
// 从父进程PID（PPID）到其直接子进程列表的映射（map）。
// 这个映射是构建依赖树的基础，因为它使得查找任何进程的子节点变得非常高效。
func (m model) buildChildrenMap() map[int32][]*process.Item {
	mp := make(map[int32][]*process.Item)
	for _, it := range m.processes {
		mp[it.PPid] = append(mp[it.PPid], it)
	}
	return mp
}

// findProcess 是一个优化的辅助函数。
// - 当 pidMap 已预计算时，使用 O(1) 的 map 查找；
// - 否则回退到对 processes 的线性扫描，以便在未初始化索引的测试/基准场景下也能正常工作。
func (m model) findProcess(pid int32) *process.Item {
	for _, it := range m.processes {
		if it.Pid == pid {
			return it
		}
	}
	return nil
}

// buildAncestorLines 函数用于构建并格式化当前根进程的祖先链。
// 它从根进程开始，通过 `findProcess` 不断向上追溯其父进程，直到达到系统根（PPID为0）或达到 `ancestorChainLimit` 限制。
func (m model) buildAncestorLines(root *process.Item) []string {
	if root == nil {
		return nil
	}
	// 1. 向上追溯，收集祖先进程。
	chain := make([]*process.Item, 0, ancestorChainLimit)
	cur := root
	for i := 0; i < ancestorChainLimit; i++ {
		if cur.PPid == 0 {
			break // 到达进程树的顶端。
		}
		p := m.findProcess(cur.PPid)
		if p == nil {
			break // 在当前进程列表中找不到父进程。
		}
		chain = append(chain, p)
		cur = p
	}

	if len(chain) == 0 {
		return nil
	}

	// 2. 将收集到的祖先链（从近到远）反转并格式化为带缩进的字符串列表。
	out := make([]string, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		// 缩进级别取决于其在反转链中的位置，从而创建出树状效果。
		indent := strings.Repeat("   ", len(chain)-1-i)
		out = append(out, fmt.Sprintf("%s└─ %s (%d)", indent, chain[i].Executable, chain[i].Pid))
	}
	return out
}

// branchSymbol 是一个视图辅助函数，根据一个节点是否是其父节点的最后一个子节点，
// 返回相应的树形连接符。
func branchSymbol(last bool) string {
	if last {
		return "└─"
	}
	return "├─"
}
