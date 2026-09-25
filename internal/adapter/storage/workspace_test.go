package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testIssue = "财新周刊第1224期"

func newTestWorkspace(t *testing.T) (*Workspace, string) {
	t.Helper()
	root := t.TempDir()
	return NewWorkspace(root), root
}

// mustWriteFile 写入文件并创建父目录；测试安排阶段的失败直接中止用例。
func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// assertNoTempFiles 断言目录内没有原子写遗留的临时文件（M2）。
func assertNoTempFiles(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", directory, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), tempFilePrefix) {
			t.Fatalf("临时文件未清理：%s", entry.Name())
		}
	}
}

func TestWorkspace_RootAndPath(t *testing.T) {
	root := t.TempDir()
	workspace := NewWorkspace(root)

	gotRoot := workspace.Root()
	gotPath := workspace.Path(testIssue, articlesDirName, "001.txt")

	if gotRoot != root {
		t.Fatalf("Root() = %q, want %q", gotRoot, root)
	}
	wantPath := filepath.Join(root, testIssue, articlesDirName, "001.txt")
	if gotPath != wantPath {
		t.Fatalf("Path() = %q, want %q", gotPath, wantPath)
	}
}

func TestWorkspace_Path_JoinsElements(t *testing.T) {
	root := t.TempDir()
	workspace := NewWorkspace(root)

	tests := []struct {
		name string
		elem []string
		want string
	}{
		{name: "no elements returns root", elem: nil, want: root},
		{name: "single element", elem: []string{"a"}, want: filepath.Join(root, "a")},
		{name: "nested elements", elem: []string{"a", "b", "c.txt"}, want: filepath.Join(root, "a", "b", "c.txt")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := workspace.Path(test.elem...)

			if got != test.want {
				t.Fatalf("Path(%v) = %q, want %q", test.elem, got, test.want)
			}
		})
	}
}

func TestWorkspace_EnsureWorkspace_CreatesLayout(t *testing.T) {
	workspace, root := newTestWorkspace(t)

	err := workspace.EnsureWorkspace(testIssue)

	if err != nil {
		t.Fatalf("EnsureWorkspace() error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(root, testIssue),
		filepath.Join(root, testIssue, stateDirName),
		filepath.Join(root, testIssue, articlesDirName),
	} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("stat %s: %v", path, statErr)
		}
		if !info.IsDir() {
			t.Fatalf("%s 不是目录", path)
		}
	}
}

func TestWorkspace_EnsureWorkspace_AppliesPermissions(t *testing.T) {
	workspace, root := newTestWorkspace(t)

	if err := workspace.EnsureWorkspace(testIssue); err != nil {
		t.Fatalf("EnsureWorkspace() error = %v", err)
	}

	tests := []struct {
		name string
		path string
		want os.FileMode
	}{
		{name: "issue dir is 0755", path: filepath.Join(root, testIssue), want: 0o755},
		{name: "progress dir is 0700", path: filepath.Join(root, testIssue, stateDirName), want: 0o700},
		{name: "articles dir is 0755", path: filepath.Join(root, testIssue, articlesDirName), want: 0o755},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info, statErr := os.Stat(test.path)
			if statErr != nil {
				t.Fatalf("stat %s: %v", test.path, statErr)
			}

			if got := info.Mode().Perm(); got != test.want {
				t.Fatalf("%s mode = %o, want %o", test.path, got, test.want)
			}
		})
	}
}

func TestWorkspace_EnsureWorkspace_IsIdempotent(t *testing.T) {
	workspace, _ := newTestWorkspace(t)

	if err := workspace.EnsureWorkspace(testIssue); err != nil {
		t.Fatalf("第一次 EnsureWorkspace() error = %v", err)
	}

	err := workspace.EnsureWorkspace(testIssue)

	if err != nil {
		t.Fatalf("第二次 EnsureWorkspace() error = %v", err)
	}
}

func TestWorkspace_EnsureWorkspace_ErrorsWhenRootIsFile(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	mustWriteFile(t, blocker, "x")
	workspace := NewWorkspace(blocker)

	err := workspace.EnsureWorkspace(testIssue)

	if err == nil {
		t.Fatal("EnsureWorkspace() error = nil, want error")
	}
}

func TestWorkspace_WriteEPUB_WritesExpectedPath(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	data := []byte("epub-bytes")

	gotPath, err := workspace.WriteEPUB(testIssue, data)

	if err != nil {
		t.Fatalf("WriteEPUB() error = %v", err)
	}
	wantPath := filepath.Join(root, testIssue, testIssue+".epub")
	if gotPath != wantPath {
		t.Fatalf("WriteEPUB() path = %q, want %q", gotPath, wantPath)
	}
	got, readErr := os.ReadFile(wantPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%s): %v", wantPath, readErr)
	}
	if string(got) != string(data) {
		t.Fatalf("EPUB 内容 = %q, want %q", got, data)
	}
	assertNoTempFiles(t, filepath.Dir(wantPath))
}

func TestWorkspace_WriteEPUB_OverwritesExisting(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	if err := workspace.EnsureWorkspace(testIssue); err != nil {
		t.Fatalf("EnsureWorkspace() error = %v", err)
	}
	existing := filepath.Join(root, testIssue, testIssue+".epub")
	mustWriteFile(t, existing, "old-longer-content")

	if _, err := workspace.WriteEPUB(testIssue, []byte("new")); err != nil {
		t.Fatalf("WriteEPUB() error = %v", err)
	}

	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("EPUB 内容 = %q, want %q", got, "new")
	}
	assertNoTempFiles(t, filepath.Join(root, testIssue))
}

func TestWorkspace_WriteEPUB_ErrorsWhenIssuePathIsFile(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	mustWriteFile(t, filepath.Join(root, testIssue), "blocker")

	_, err := workspace.WriteEPUB(testIssue, []byte("x"))

	if err == nil {
		t.Fatal("WriteEPUB() error = nil, want error")
	}
}
