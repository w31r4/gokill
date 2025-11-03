// package process 声明了此文件属于 process 包。
package process

import (
	// "encoding/json" 包用于处理JSON数据的编码和解码。
	"encoding/json"
	// "os" 包提供了与操作系统交互的功能，如文件操作、获取用户目录等。
	"os"
	// "path/filepath" 包提供了处理文件路径的可移植方式。
	"path/filepath"
)

// cachePath 函数用于获取缓存文件的标准路径。
// 它遵循操作系统的惯例，将缓存文件存储在用户特定的缓存目录下，以避免污染用户的主目录。
func cachePath() (string, error) {
	// os.UserCacheDir() 会返回适合当前用户的缓存目录路径。
	// 例如，在 Linux 上可能是 `~/.cache`，在 macOS 上是 `~/Library/Caches`。
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		// 如果无法获取缓存目录，则返回错误。
		return "", err
	}
	// 使用 filepath.Join 来安全地拼接路径，它会自动处理不同操作系统下的路径分隔符（'/' 或 '\'）。
	// 最终的缓存文件路径类似于 `~/.cache/gkill_cache.json`。
	return filepath.Join(cacheDir, "gkill_cache.json"), nil
}

// Save 函数将当前的进程列表（`[]*Item` 类型）序列化为JSON格式，并保存到缓存文件中。
// 这个操作通常在获取到新的进程列表后异步执行，以便下次启动时可以快速加载。
func Save(processes []*Item) error {
	// 获取标准的缓存文件路径。
	path, err := cachePath()
	if err != nil {
		return err
	}

	// os.Create 会创建一个新文件用于写入。如果文件已存在，其内容将被清空。
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	// 使用 defer 语句确保在函数返回前，文件句柄一定会被关闭，以释放资源。
	defer f.Close()

	// json.NewEncoder(f) 创建一个将数据编码为JSON并写入到文件 `f` 的编码器。
	// Encode(processes) 将 `processes` 这个Go的数据结构转换为JSON字符串并写入文件。
	return json.NewEncoder(f).Encode(processes)
}

// Load 函数从缓存文件中读取JSON数据，并将其反序列化为 `[]*Item` 类型的Go切片。
// 这个函数在程序启动时被调用，用于快速加载上一次的进程列表，从而在等待新的、实时的进程列表时，
// 用户界面不会是空的，可以立即显示一些（可能已过时的）数据。
func Load() ([]*Item, error) {
	// 获取标准的缓存文件路径。
	path, err := cachePath()
	if err != nil {
		return nil, err
	}

	// os.Open 以只读方式打开缓存文件。
	f, err := os.Open(path)
	if err != nil {
		// 如果文件不存在或无法打开（例如权限问题），会返回错误。
		// 这在首次运行时是正常情况，调用者需要处理这个错误。
		return nil, err
	}
	// 同样，使用 defer 确保文件句柄被关闭。
	defer f.Close()

	// 声明一个切片用于存储从JSON解码出的数据。
	var processes []*Item
	// json.NewDecoder(f) 创建一个从文件 `f` 读取并解码JSON数据的解码器。
	// Decode(&processes) 读取JSON数据，并将其填充到 `processes` 切片的地址中。
	if err := json.NewDecoder(f).Decode(&processes); err != nil {
		// 如果JSON解析失败（例如文件损坏），则返回错误。
		return nil, err
	}
	// 如果一切顺利，返回加载并解析好的进程列表。
	return processes, nil
}
