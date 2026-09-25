package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"caixin2kindle/internal/model"
)

func TestWorkspace_CleanWorkspace_RemovesOnlyKnownArtifacts(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	issueDir := filepath.Join(root, testIssue)
	known := []string{
		filepath.Join(issueDir, stateDirName, stateFileName),
		filepath.Join(issueDir, articlesDirName, "001-a.txt"),
		filepath.Join(issueDir, articlesDirName, "002-b.txt"),
		filepath.Join(issueDir, testIssue+".epub"),
		filepath.Join(issueDir, testIssue+".mobi"),
	}
	for index, path := range known {
		mustWriteFile(t, path, "content-"+string(rune('a'+index)))
	}
	unrelated := filepath.Join(issueDir, "notes.txt")
	nested := filepath.Join(issueDir, articlesDirName, "nested", "003-c.txt")
	mustWriteFile(t, unrelated, "keep")
	mustWriteFile(t, nested, "keep")

	err := workspace.CleanWorkspace(testIssue)

	if err != nil {
		t.Fatalf("CleanWorkspace() error = %v", err)
	}
	for _, path := range known {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("已知产物 %s 未被删除（err=%v）", path, statErr)
		}
	}
	for _, path := range []string{unrelated, nested, issueDir} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("非目标路径 %s 被删除：%v", path, statErr)
		}
	}
}

func TestWorkspace_CleanWorkspace_MissingArtifactsAreNotErrors(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	if err := os.MkdirAll(filepath.Join(root, testIssue), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := workspace.CleanWorkspace(testIssue)

	if err != nil {
		t.Fatalf("CleanWorkspace() error = %v, want nil", err)
	}
}

func TestWorkspace_CleanWorkspace_MissingIssueDirectoryIsNotError(t *testing.T) {
	workspace, _ := newTestWorkspace(t)

	err := workspace.CleanWorkspace("不存在的期号")

	if err != nil {
		t.Fatalf("CleanWorkspace() error = %v, want nil", err)
	}
}

func TestWorkspace_CleanWorkspace_RemovalFailureWrapsUsage(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	statePath := filepath.Join(root, testIssue, stateDirName, stateFileName)
	mustWriteFile(t, filepath.Join(statePath, "inner"), "x")

	err := workspace.CleanWorkspace(testIssue)

	if !errors.Is(err, model.ErrUsage) {
		t.Fatalf("CleanWorkspace() error = %v, want ErrUsage", err)
	}
	if err == nil || !strings.Contains(err.Error(), statePath) {
		t.Fatalf("CleanWorkspace() error = %v, want 包含待清理路径 %s", err, statePath)
	}
}

func TestWorkspace_CleanWorkspace_UnreadableArticlesDirWrapsUsage(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	mustWriteFile(t, filepath.Join(root, testIssue, articlesDirName), "not a directory")

	err := workspace.CleanWorkspace(testIssue)

	if !errors.Is(err, model.ErrUsage) {
		t.Fatalf("CleanWorkspace() error = %v, want ErrUsage", err)
	}
}
