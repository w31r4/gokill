package tui

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/w31r4/gokill/internal/process"
)

// generateMockProcesses 生成指定数量的模拟进程数据用于基准测试
func generateMockProcesses(count int) []*process.Item {
	procs := make([]*process.Item, count)
	for i := 0; i < count; i++ {
		pid := int32(1000 + i)
		ppid := int32(1) // 大部分进程的父进程为init
		if i > 0 && i%10 == 0 {
			// 创建一些子进程关系
			ppid = int32(1000 + i/10)
		}
		procs[i] = &process.Item{
			Pid:        pid,
			PPid:       ppid,
			Executable: fmt.Sprintf("process-%d", i),
			User:       "testuser",
			Status:     process.Alive,
			Ports:      []uint32{},
		}
	}
	return procs
}

// BenchmarkBuildChildrenMap 测试 buildChildrenMap 函数的性能
func BenchmarkBuildChildrenMap(b *testing.B) {
	testCases := []struct {
		name  string
		count int
	}{
		{"Small-100", 100},
		{"Medium-1000", 1000},
		{"Large-5000", 5000},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			procs := generateMockProcesses(tc.count)
			m := model{processes: procs}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = m.buildChildrenMap()
			}
		})
	}
}

// BenchmarkFindProcess 测试 findProcess 函数的性能
func BenchmarkFindProcess(b *testing.B) {
	testCases := []struct {
		name  string
		count int
	}{
		{"Small-100", 100},
		{"Medium-1000", 1000},
		{"Large-5000", 5000},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			procs := generateMockProcesses(tc.count)
			m := model{processes: procs}
			targetPid := int32(1000 + tc.count/2) // 查找中间位置的进程

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = m.findProcess(targetPid)
			}
		})
	}
}

// BenchmarkBuildDepLines 测试 buildDepLines 函数的性能
func BenchmarkBuildDepLines(b *testing.B) {
	testCases := []struct {
		name  string
		count int
	}{
		{"Small-100", 100},
		{"Medium-1000", 1000},
		{"Large-5000", 5000},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			procs := generateMockProcesses(tc.count)
			m := model{
				processes: procs,
				dep: depViewState{
					mode:     true,
					rootPID:  1000, // 第一个进程作为根
					expanded: make(map[int32]depNodeState),
				},
			}
			// 确保根节点已展开
			m.dep.expanded[1000] = depNodeState{expanded: true, page: 1}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = buildDepLines(m)
			}
		})
	}
}

// BenchmarkTypicalInteractions 模拟典型的用户交互序列
func BenchmarkTypicalInteractions(b *testing.B) {
	testCases := []struct {
		name  string
		count int
	}{
		{"Medium-1000", 1000},
		{"Large-5000", 5000},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			procs := generateMockProcesses(tc.count)
			m := model{
				processes: procs,
				dep: depViewState{
					mode:     true,
					rootPID:  1000,
					expanded: make(map[int32]depNodeState),
					cursor:   0,
				},
			}
			m.dep.expanded[1000] = depNodeState{expanded: true, page: 1}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// 模拟 100 次光标移动
				for j := 0; j < 100; j++ {
					lines := buildDepLines(m)
					if len(lines) > 0 {
						m.dep.cursor = (m.dep.cursor + 1) % len(lines)
					}
				}

				// 模拟 50 次展开/折叠操作
				for j := 0; j < 50; j++ {
					lines := buildDepLines(m)
					if len(lines) > 0 && m.dep.cursor < len(lines) {
						ln := lines[m.dep.cursor]
						if ln.pid != 0 {
							st := m.dep.expanded[ln.pid]
							st.expanded = !st.expanded
							m.dep.expanded[ln.pid] = st
						}
					}
				}
			}
		})
	}
}

// measureInteractionLatency 测量实际交互延迟
func measureInteractionLatency() {
	fmt.Println("=== 性能基准测试 - 优化前 ===")

	testSizes := []struct {
		name  string
		count int
	}{
		{"小型系统 (100进程)", 100},
		{"中型系统 (1000进程)", 1000},
		{"大型系统 (5000进程)", 5000},
	}

	for _, size := range testSizes {
		fmt.Printf("\n%s:\n", size.name)

		procs := generateMockProcesses(size.count)
		m := model{
			processes: procs,
			dep: depViewState{
				mode:     true,
				rootPID:  1000,
				expanded: make(map[int32]depNodeState),
				cursor:   0,
			},
		}
		m.dep.expanded[1000] = depNodeState{expanded: true, page: 1}

		// 测试 buildChildrenMap 性能
		start := time.Now()
		for i := 0; i < 100; i++ {
			_ = m.buildChildrenMap()
		}
		childrenMapTime := time.Since(start) / 100
		fmt.Printf("  buildChildrenMap (单次): %v\n", childrenMapTime)

		// 测试 findProcess 性能
		targetPid := int32(1000 + size.count/2)
		start = time.Now()
		for i := 0; i < 1000; i++ {
			_ = m.findProcess(targetPid)
		}
		findProcessTime := time.Since(start) / 1000
		fmt.Printf("  findProcess (单次): %v\n", findProcessTime)

		// 测试完整交互序列
		start = time.Now()
		for i := 0; i < 10; i++ {
			// 100次光标移动
			for j := 0; j < 100; j++ {
				lines := buildDepLines(m)
				if len(lines) > 0 {
					m.dep.cursor = (m.dep.cursor + 1) % len(lines)
				}
			}
			// 50次展开/折叠
			for j := 0; j < 50; j++ {
				lines := buildDepLines(m)
				if len(lines) > 0 && m.dep.cursor < len(lines) {
					ln := lines[m.dep.cursor]
					if ln.pid != 0 {
						st := m.dep.expanded[ln.pid]
						st.expanded = !st.expanded
						m.dep.expanded[ln.pid] = st
					}
				}
			}
		}
		interactionTime := time.Since(start) / 10
		fmt.Printf("  典型交互序列 (100光标移动+50展开/折叠): %v\n", interactionTime)
	}
}

// TestMain 是测试的入口点
func TestMain(m *testing.M) {
	// 运行基准测试前先打印性能数据
	measureInteractionLatency()

	// 运行标准的基准测试
	code := m.Run()
	os.Exit(code)
}
