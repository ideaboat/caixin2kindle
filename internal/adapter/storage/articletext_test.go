package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspace_WriteArticleText_ThenRead(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	relative := filepath.Join(testIssue, articlesDirName, "001-a.txt")
	body := "第一段\n\n第二段"

	err := workspace.WriteArticleText(relative, body)

	if err != nil {
		t.Fatalf("WriteArticleText() error = %v", err)
	}
	got, readErr := workspace.ReadArticleText(relative)
	if readErr != nil {
		t.Fatalf("ReadArticleText() error = %v", readErr)
	}
	if got != body {
		t.Fatalf("正文 = %q, want %q", got, body)
	}
	assertNoTempFiles(t, filepath.Dir(filepath.Join(root, relative)))
}

func TestWorkspace_WriteArticleText_WritesPrivateMode(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	relative := filepath.Join(testIssue, articlesDirName, "001-a.txt")

	if err := workspace.WriteArticleText(relative, "body"); err != nil {
		t.Fatalf("WriteArticleText() error = %v", err)
	}

	info, err := os.Stat(filepath.Join(root, relative))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("正文文件 mode = %o, want 0600", got)
	}
}

func TestWorkspace_WriteArticleText_AcceptsAbsolutePath(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	absolute := filepath.Join(root, "custom", "001-a.txt")

	err := workspace.WriteArticleText(absolute, "abs")

	if err != nil {
		t.Fatalf("WriteArticleText() error = %v", err)
	}
	got, readErr := os.ReadFile(absolute)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(got) != "abs" {
		t.Fatalf("正文 = %q, want %q", got, "abs")
	}
}

func TestWorkspace_WriteArticleText_EmptyBody(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	relative := filepath.Join(testIssue, articlesDirName, "empty.txt")

	if err := workspace.WriteArticleText(relative, ""); err != nil {
		t.Fatalf("WriteArticleText() error = %v", err)
	}

	info, statErr := os.Stat(filepath.Join(root, relative))
	if statErr != nil {
		t.Fatalf("stat: %v", statErr)
	}
	if info.Size() != 0 {
		t.Fatalf("空正文文件大小 = %d, want 0", info.Size())
	}
}

func TestWorkspace_WriteArticleText_OverwritesExisting(t *testing.T) {
	workspace, _ := newTestWorkspace(t)
	relative := filepath.Join(testIssue, articlesDirName, "001-a.txt")
	if err := workspace.WriteArticleText(relative, "旧内容更长一些"); err != nil {
		t.Fatalf("首次 WriteArticleText() error = %v", err)
	}

	err := workspace.WriteArticleText(relative, "新")

	if err != nil {
		t.Fatalf("覆盖 WriteArticleText() error = %v", err)
	}
	got, readErr := workspace.ReadArticleText(relative)
	if readErr != nil {
		t.Fatalf("ReadArticleText() error = %v", readErr)
	}
	if got != "新" {
		t.Fatalf("正文 = %q, want %q", got, "新")
	}
}

func TestWorkspace_WriteArticleText_ErrorsWhenParentIsFile(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	mustWriteFile(t, filepath.Join(root, "blocker"), "x")

	err := workspace.WriteArticleText(filepath.Join("blocker", "001-a.txt"), "body")

	if err == nil {
		t.Fatal("WriteArticleText() error = nil, want error")
	}
}

func TestWorkspace_WriteArticleText_ErrorsWhenDestinationIsDirectory(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	relative := filepath.Join(testIssue, articlesDirName, "001-a.txt")
	mustWriteFile(t, filepath.Join(root, relative, "inner"), "x")

	err := workspace.WriteArticleText(relative, "body")

	if err == nil {
		t.Fatal("WriteArticleText() error = nil, want error")
	}
	assertNoTempFiles(t, filepath.Join(root, testIssue, articlesDirName))
}

func TestWorkspace_WriteArticleText_ErrorsWhenDirectoryNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 忽略目录权限位")
	}
	workspace, root := newTestWorkspace(t)
	directory := filepath.Join(root, "readonly")
	if err := os.MkdirAll(directory, 0o500); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := workspace.WriteArticleText(filepath.Join("readonly", "001-a.txt"), "body")

	if err == nil {
		t.Fatal("WriteArticleText() error = nil, want error")
	}
	assertNoTempFiles(t, directory)
}

func TestWorkspace_ReadArticleText_MissingFileErrorsNotExist(t *testing.T) {
	workspace, _ := newTestWorkspace(t)

	_, err := workspace.ReadArticleText(filepath.Join(testIssue, articlesDirName, "missing.txt"))

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadArticleText() error = %v, want os.ErrNotExist", err)
	}
}

func TestWorkspace_Exists(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	mustWriteFile(t, filepath.Join(root, "present.txt"), "x")
	mustWriteFile(t, filepath.Join(root, "blocker"), "x")

	tests := []struct {
		name    string
		path    string
		want    bool
		wantErr bool
	}{
		{name: "existing file", path: "present.txt", want: true},
		{name: "missing file", path: "missing.txt", want: false},
		{name: "path under a regular file", path: filepath.Join("blocker", "child.txt"), want: false, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := workspace.Exists(test.path)

			if test.wantErr {
				if err == nil {
					t.Fatal("Exists() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Exists() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("Exists(%q) = %v, want %v", test.path, got, test.want)
			}
		})
	}
}
