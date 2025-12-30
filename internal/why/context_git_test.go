package why

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGitInfo(t *testing.T) {
	// Create temp dir structure
	tmpDir, err := os.MkdirTemp("", "gkill-test-git")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Case 1: Standard Repo
	repoDir := filepath.Join(tmpDir, "myrepo")
	gitDir := filepath.Join(repoDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	repo, branch := detectGitInfo(repoDir)
	if repo != "myrepo" {
		t.Errorf("expected repo 'myrepo', got '%s'", repo)
	}
	if branch != "main" {
		t.Errorf("expected branch 'main', got '%s'", branch)
	}

	// Case 2: Subdirectory
	subDir := filepath.Join(repoDir, "src", "cmd")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	repo, branch = detectGitInfo(subDir)
	if repo != "myrepo" {
		t.Errorf("expected repo 'myrepo' from subdir, got '%s'", repo)
	}
	if branch != "main" {
		t.Errorf("expected branch 'main' from subdir, got '%s'", branch)
	}

	// Case 2b: Repeated lookup should remain stable (and exercises cache paths).
	repo2, branch2 := detectGitInfo(subDir)
	if repo2 != "myrepo" || branch2 != "main" {
		t.Errorf("expected repeated lookup to return 'myrepo','main', got '%s','%s'", repo2, branch2)
	}

	// Case 3: Detached HEAD
	detachedDir := filepath.Join(tmpDir, "detached")
	dGitDir := filepath.Join(detachedDir, ".git")
	if err := os.MkdirAll(dGitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dGitDir, "HEAD"), []byte("a1b2c3d4e5\n"), 0644); err != nil {
		t.Fatal(err)
	}
	repo, branch = detectGitInfo(detachedDir)
	if repo != "detached" {
		t.Errorf("expected repo 'detached', got '%s'", repo)
	}
	if branch != "a1b2c3d" { // Check for truncated SHA
		t.Errorf("expected branch 'a1b2c3d', got '%s'", branch)
	}

	// Case 4: Not a repo
	notRepoDir := filepath.Join(tmpDir, "notrepo")
	if err := os.MkdirAll(notRepoDir, 0755); err != nil {
		t.Fatal(err)
	}
	repo, branch = detectGitInfo(notRepoDir)
	if repo != "" || branch != "" {
		t.Errorf("expected empty result for non-repo, got '%s', '%s'", repo, branch)
	}
}
