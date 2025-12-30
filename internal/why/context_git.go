package why

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type gitInfoCacheEntry struct {
	repo      string
	branch    string
	expiresAt time.Time
}

var gitInfoCache = struct {
	mu      sync.RWMutex
	entries map[string]gitInfoCacheEntry
	ttl     time.Duration
	maxSize int
}{
	entries: make(map[string]gitInfoCacheEntry),
	ttl:     30 * time.Second,
	maxSize: 2048,
}

func getGitInfoCache(dir string, now time.Time) (repo string, branch string, ok bool) {
	gitInfoCache.mu.RLock()
	entry, found := gitInfoCache.entries[dir]
	gitInfoCache.mu.RUnlock()
	if !found || now.After(entry.expiresAt) {
		return "", "", false
	}
	return entry.repo, entry.branch, true
}

func setGitInfoCache(dirs []string, repo string, branch string, now time.Time) {
	if len(dirs) == 0 {
		return
	}

	gitInfoCache.mu.Lock()
	defer gitInfoCache.mu.Unlock()

	// Evict expired entries first.
	for dir, entry := range gitInfoCache.entries {
		if now.After(entry.expiresAt) {
			delete(gitInfoCache.entries, dir)
		}
	}

	// Best-effort cap: if still too large, reset the map.
	if len(gitInfoCache.entries)+len(dirs) > gitInfoCache.maxSize {
		gitInfoCache.entries = make(map[string]gitInfoCacheEntry)
	}

	expiresAt := now.Add(gitInfoCache.ttl)
	for _, dir := range dirs {
		gitInfoCache.entries[dir] = gitInfoCacheEntry{
			repo:      repo,
			branch:    branch,
			expiresAt: expiresAt,
		}
	}
}

// detectGitInfo attempts to find the Git repository and branch for a given working directory.
// It walks up the directory tree looking for a .git directory.
func detectGitInfo(cwd string) (repo string, branch string) {
	cwd = filepath.Clean(cwd)
	if cwd == "" || cwd == "/" || cwd == "." {
		return "", ""
	}

	now := time.Now()

	var visited []string
	searchDir := cwd
	for searchDir != "/" && searchDir != "." && searchDir != "" {
		// Fast path: cache hit at this directory or any ancestor.
		if cachedRepo, cachedBranch, ok := getGitInfoCache(searchDir, now); ok {
			visited = append(visited, searchDir)
			setGitInfoCache(visited, cachedRepo, cachedBranch, now)
			return cachedRepo, cachedBranch
		}

		gitDir := filepath.Join(searchDir, ".git")
		if fi, err := os.Stat(gitDir); err == nil && fi.IsDir() {
			// Repo name is the base dir
			repo = filepath.Base(searchDir)

			// Try to read HEAD for branch
			headFile := filepath.Join(gitDir, "HEAD")
			if head, err := os.ReadFile(headFile); err == nil {
				content := strings.TrimSpace(string(head))
				if strings.HasPrefix(content, "ref: ") {
					ref := strings.TrimPrefix(content, "ref: ")
					// Extract branch name from ref (e.g., refs/heads/main -> main)
					parts := strings.Split(ref, "/")
					if len(parts) > 0 {
						branch = parts[len(parts)-1]
					}
				} else {
					// Detached HEAD or direct SHA
					if len(content) >= 7 {
						branch = content[:7]
					} else {
						branch = content
					}
				}
			}

			visited = append(visited, searchDir)
			setGitInfoCache(visited, repo, branch, now)
			return repo, branch
		}

		visited = append(visited, searchDir)

		// Move up one level
		parent := filepath.Dir(searchDir)
		if parent == searchDir {
			break
		}
		searchDir = parent
	}

	setGitInfoCache(visited, "", "", now)
	return "", ""
}
