package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/w31r4/gokill/internal/process"
)

// Dependency tree configuration shared across update and view logic.
const (
	dependencyTreeDepth      = 3
	dependencyTreeChildLimit = 5
	ancestorChainLimit       = 6
)

// depNodeState 记录依赖树中每个节点的展开状态、分页进度以及额外深度扩展。
type depNodeState struct {
	expanded    bool
	page        int
	depthExtend int
}

// depLine 是依赖树被扁平化为列表后的单行表示。
type depLine struct {
	pid      int32
	parent   int32
	isMore   bool
	text     string
	depth    int
	isDeeper bool
}

// buildDepLines 将当前依赖树按展开/分页状态扁平化为行。
func buildDepLines(m model) []depLine {
	root := m.findProcess(m.depRootPID)
	if root == nil {
		return nil
	}
	childrenMap := m.buildChildrenMap()

	var lines []depLine
	// 根行（深度0）
	lines = append(lines, depLine{pid: root.Pid, parent: 0, isMore: false, text: fmt.Sprintf("%s (%d)", root.Executable, root.Pid), depth: 0})

	// 递归子节点
	var walk func(pid int32, prefix string, depth int)
	walk = func(pid int32, prefix string, depth int) {
		kids := childrenMap[pid]
		if len(kids) == 0 {
			return
		}

		// 排序稳定
		sort.Slice(kids, func(i, j int) bool {
			if kids[i].Executable == kids[j].Executable {
				return kids[i].Pid < kids[j].Pid
			}
			return kids[i].Executable < kids[j].Executable
		})

		// 展开状态与分页
		st := m.depExpanded[pid]
		if !st.expanded && pid != m.depRootPID {
			return
		}
		page := st.page
		if page <= 0 {
			page = 1
		}
		limit := dependencyTreeChildLimit * page
		show := len(kids)
		if show > limit {
			show = limit
		}

		for i := 0; i < show; i++ {
			child := kids[i]
			last := (i == show-1) && (show == len(kids))
			connector := branchSymbol(last)
			line := fmt.Sprintf("%s%s %s (%d)", prefix, connector, child.Executable, child.Pid)
			lines = append(lines, depLine{pid: child.Pid, parent: pid, text: line, depth: depth + 1})

			nextPrefix := prefix
			if last {
				nextPrefix += "   "
			} else {
				nextPrefix += "│  "
			}

			// allow per-parent deeper expansion beyond global depth
			allowed := dependencyTreeDepth - 1 + m.depExpanded[pid].depthExtend
			if depth < allowed {
				walk(child.Pid, nextPrefix, depth+1)
			} else if len(childrenMap[child.Pid]) > 0 {
				// depth limit reached; add interactive deeper placeholder
				moreLine := fmt.Sprintf("%s└─ … (deeper)", nextPrefix)
				lines = append(lines, depLine{pid: 0, parent: child.Pid, isMore: true, isDeeper: true, text: moreLine, depth: depth + 1})
			}
		}

		if show < len(kids) {
			// 还有更多同级子项，提供分页提示行
			more := len(kids) - show
			connector := branchSymbol(true)
			moreLine := fmt.Sprintf("%s%s … (%d more)", prefix, connector, more)
			lines = append(lines, depLine{pid: 0, parent: pid, isMore: true, isDeeper: false, text: moreLine, depth: depth})
		}
	}

	// 根默认展开
	if st, ok := m.depExpanded[root.Pid]; !ok || !st.expanded {
		m.depExpanded[root.Pid] = depNodeState{expanded: true, page: 1}
	}
	walk(root.Pid, "", 0)
	return lines
}

// applyDepFilters 根据 T 模式的筛选条件过滤行：文本过滤、仅存活、仅监听端口。
func applyDepFilters(m model, lines []depLine) []depLine {
	if len(lines) == 0 {
		return lines
	}

	term := strings.TrimSpace(m.textInput.Value())
	hasTerm := term != ""
	var out []depLine
	for _, ln := range lines {
		// never drop paging/ellipsis lines完全, but they are not actionable
		if ln.pid == 0 {
			if hasTerm {
				// skip ellipsis on filter to reduce noise
				continue
			}
			out = append(out, ln)
			continue
		}
		it := m.findProcess(ln.pid)
		if it == nil {
			continue
		}
		if m.depAliveOnly && it.Status != process.Alive {
			continue
		}
		if m.depPortsOnly && len(it.Ports) == 0 {
			continue
		}
		if hasTerm {
			// case-insensitive substring on text and pid match
			if !strings.Contains(strings.ToLower(ln.text), strings.ToLower(term)) {
				if termPid, err := strconv.Atoi(term); err == nil {
					if int32(termPid) != it.Pid {
						continue
					}
				} else {
					continue
				}
			}
		}
		out = append(out, ln)
	}
	return out
}

// buildChildrenMap 为依赖树构建父 PID 到子进程列表的映射。
func (m model) buildChildrenMap() map[int32][]*process.Item {
	mp := make(map[int32][]*process.Item)
	for _, it := range m.processes {
		mp[it.PPid] = append(mp[it.PPid], it)
	}
	return mp
}

// findProcess 在当前进程列表中按 PID 查找进程。
func (m model) findProcess(pid int32) *process.Item {
	for _, it := range m.processes {
		if it.Pid == pid {
			return it
		}
	}
	return nil
}

// buildAncestorLines 生成从当前 root 向上的有限祖先进程链（最多 ancestorChainLimit）。
func (m model) buildAncestorLines(root *process.Item) []string {
	if root == nil {
		return nil
	}
	chain := make([]*process.Item, 0, ancestorChainLimit)
	cur := root
	for i := 0; i < ancestorChainLimit; i++ {
		if cur.PPid == 0 {
			break
		}
		p := m.findProcess(cur.PPid)
		if p == nil {
			break
		}
		chain = append(chain, p)
		cur = p
	}
	if len(chain) == 0 {
		return nil
	}
	out := make([]string, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		indent := strings.Repeat("   ", len(chain)-1-i)
		out = append(out, fmt.Sprintf("%s└─ %s (%d)", indent, chain[i].Executable, chain[i].Pid))
	}
	return out
}

// branchSymbol returns the tree drawing character for normal vs last child.
func branchSymbol(last bool) string {
	if last {
		return "└─"
	}
	return "├─"
}

