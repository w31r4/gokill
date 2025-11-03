// package main 表明这是一个可执行程序的入口。
package main

import (
	// "os" 包提供了与操作系统交互的功能，这里主要用于读取命令行参数。
	"os"
	// "strings" 包提供了字符串操作的函数。
	"strings"

	// 导入项目的核心TUI（文本用户界面）模块。
	"github.com/w31r4/gokill/internal/tui"
)

// main 函数是整个程序的入口点。
func main() {
	// 声明一个字符串变量 `filter`，用于存储从命令行传入的初始搜索/过滤条件。
	var filter string

	// `os.Args` 是一个字符串切片，包含了所有命令行参数。
	// `os.Args[0]` 是程序本身的名称，后续元素是传递给程序的参数。
	// 这里检查参数数量是否大于1，以确定用户是否提供了初始过滤条件。
	if len(os.Args) > 1 {
		// 如果用户提供了参数，就将除了程序名之外的所有参数用空格连接起来，
		// 形成一个单一的过滤字符串。
		// 例如，用户运行 `gkill myapp 8080`，`filter` 的值就会是 "myapp 8080"。
		filter = strings.Join(os.Args[1:], " ")
	}

	// 调用 `tui` 包的 `Start` 函数，启动整个文本用户界面。
	// 将从命令行获取的过滤字符串传递给TUI，作为初始的搜索值。
	tui.Start(filter)
}
