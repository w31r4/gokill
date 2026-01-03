package why

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGitInfo(t *testing.T) {
	tmpDir := tempDir(t)

	repoDir := setupRepo(t, tmpDir, "myrepo", "ref: refs/heads/main\n")
	assertGitInfo(t, repoDir, "myrepo", "main", "standard repo")

	subDir := filepath.Join(repoDir, "src", "cmd")
	ensureDir(t, subDir)
	assertGitInfo(t, subDir, "myrepo", "main", "subdir")
	assertGitInfo(t, subDir, "myrepo", "main", "repeated lookup")

	detachedDir := setupRepo(t, tmpDir, "detached", "a1b2c3d4e5\n")
	assertGitInfo(t, detachedDir, "detached", "a1b2c3d", "detached HEAD")

	notRepoDir := filepath.Join(tmpDir, "notrepo")
	ensureDir(t, notRepoDir)
	assertGitInfo(t, notRepoDir, "", "", "not a repo")
}

func tempDir(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "gkill-test-git")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
	})
	return tmpDir
}

func setupRepo(t *testing.T, baseDir, name, headContents string) string {
	t.Helper()

	repoDir := filepath.Join(baseDir, name)
	gitDir := filepath.Join(repoDir, ".git")
	ensureDir(t, gitDir)
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headContents), 0644); err != nil {
		t.Fatal(err)
	}
	return repoDir
}

func ensureDir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func assertGitInfo(t *testing.T, path, wantRepo, wantBranch, label string) {
	t.Helper()

	repo, branch := detectGitInfo(path)
	if repo != wantRepo {
		t.Errorf("%s: expected repo '%s', got '%s'", label, wantRepo, repo)
	}
	if branch != wantBranch {
		t.Errorf("%s: expected branch '%s', got '%s'", label, wantBranch, branch)
	}
}
