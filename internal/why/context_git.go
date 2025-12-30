package why

import (
	"os"
	"path/filepath"
	"strings"
)

// detectGitInfo attempts to find the Git repository and branch for a given working directory.
// It walks up the directory tree looking for a .git directory.
func detectGitInfo(cwd string) (repo string, branch string) {
	if cwd == "" || cwd == "/" {
		return "", ""
	}

	searchDir := cwd
	for searchDir != "/" && searchDir != "." && searchDir != "" {
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
			return repo, branch
		}

		// Move up one level
		parent := filepath.Dir(searchDir)
		if parent == searchDir {
			break
		}
		searchDir = parent
	}

	return "", ""
}
