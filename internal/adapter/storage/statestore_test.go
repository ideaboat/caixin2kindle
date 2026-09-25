package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"caixin2kindle/internal/model"
)

// sampleState 返回一个字段全部非零的状态，用于往返一致性与产物持久化断言。
func sampleState() model.State {
	return model.State{
		SchemaVersion: model.SchemaVersion,
		IssueID:       testIssue,
		Articles: []model.ArticleState{
			{
				Order:         1,
				URL:           "https://weekly.caixin.com/2026/cw1224/01.html",
				NormalizedURL: "https://weekly.caixin.com/2026/cw1224/01.html",
				Title:         "第一篇",
				Status:        model.StatusSuccess,
				TextPath:      filepath.Join(articlesDirName, "001-a.txt"),
				Hash:          "sha256:aaa",
			},
			{
				Order:         2,
				URL:           "https://weekly.caixin.com/2026/cw1224/02.html",
				NormalizedURL: "https://weekly.caixin.com/2026/cw1224/02.html",
				Title:         "第二篇",
				Status:        model.StatusFail,
			},
		},
		Artifacts: model.Artifact{
			EPUB:         testIssue + ".epub",
			MOBI:         testIssue + ".mobi",
			BuiltIssueID: testIssue,
		},
	}
}

func TestStateStore_SaveThenLoad_RoundTrips(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	store := NewStateStore(workspace)
	issueDir := filepath.Join(root, testIssue)
	want := sampleState()

	if err := store.Save(context.Background(), issueDir, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Load(context.Background(), issueDir)

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestStateStore_Save_WritesPrivateStateFile(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	store := NewStateStore(workspace)
	issueDir := filepath.Join(root, testIssue)

	if err := store.Save(context.Background(), issueDir, sampleState()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	statePath := filepath.Join(issueDir, stateDirName, stateFileName)
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("stat(%s): %v", statePath, err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("state.json mode = %o, want 0600", got)
	}
	dirInfo, dirErr := os.Stat(filepath.Dir(statePath))
	if dirErr != nil {
		t.Fatalf("stat(dir): %v", dirErr)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("进度目录 mode = %o, want 0700", got)
	}
	assertNoTempFiles(t, filepath.Dir(statePath))
}

func TestStateStore_Save_OverwritesPreviousState(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	store := NewStateStore(workspace)
	issueDir := filepath.Join(root, testIssue)
	if err := store.Save(context.Background(), issueDir, sampleState()); err != nil {
		t.Fatalf("首次 Save() error = %v", err)
	}
	replacement := model.State{SchemaVersion: model.SchemaVersion, IssueID: "另一期"}

	if err := store.Save(context.Background(), issueDir, replacement); err != nil {
		t.Fatalf("覆盖 Save() error = %v", err)
	}

	got, err := store.Load(context.Background(), issueDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, replacement) {
		t.Fatalf("Load() = %+v, want %+v", got, replacement)
	}
}

func TestStateStore_Load_MissingFileReturnsEmptyState(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	store := NewStateStore(workspace)

	got, err := store.Load(context.Background(), filepath.Join(root, testIssue))

	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, model.State{}) {
		t.Fatalf("Load() = %+v, want 空状态", got)
	}
}

func TestStateStore_Load_DegradesCorruptOrMismatchedState(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: "{not json"},
		{
			name: "unknown field",
			body: `{"schema_version":1,"issue_id":"x","articles":[],"artifacts":{},"extra":true}`,
		},
		{name: "schema mismatch", body: `{"schema_version":99,"issue_id":"x","articles":[],"artifacts":{}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace, root := newTestWorkspace(t)
			issueDir := filepath.Join(root, testIssue)
			mustWriteFile(t, filepath.Join(issueDir, stateDirName, stateFileName), test.body)
			var warnings []string
			store := NewStateStore(workspace)
			store.Warn = func(msg string) { warnings = append(warnings, msg) }

			got, err := store.Load(context.Background(), issueDir)

			if err != nil {
				t.Fatalf("Load() error = %v, want nil（降级为全量）", err)
			}
			if !reflect.DeepEqual(got, model.State{}) {
				t.Fatalf("Load() = %+v, want 空状态", got)
			}
			if len(warnings) != 1 {
				t.Fatalf("Warn 调用 %d 次, want 1 次：%v", len(warnings), warnings)
			}
			if !strings.Contains(warnings[0], stateFileName) {
				t.Fatalf("Warn 消息 %q 未包含状态文件路径", warnings[0])
			}
		})
	}
}

func TestStateStore_Load_DegradesWithoutWarnCallback(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	issueDir := filepath.Join(root, testIssue)
	mustWriteFile(t, filepath.Join(issueDir, stateDirName, stateFileName), "{broken")
	store := NewStateStore(workspace)

	got, err := store.Load(context.Background(), issueDir)

	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, model.State{}) {
		t.Fatalf("Load() = %+v, want 空状态", got)
	}
}

func TestStateStore_Load_ErrorsWhenPathIsNotDirectory(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	blocker := filepath.Join(root, "blocker")
	mustWriteFile(t, blocker, "x")
	store := NewStateStore(workspace)

	_, err := store.Load(context.Background(), blocker)

	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestStateStore_Save_ErrorsWhenPathIsNotDirectory(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	blocker := filepath.Join(root, "blocker")
	mustWriteFile(t, blocker, "x")
	store := NewStateStore(workspace)

	err := store.Save(context.Background(), blocker, sampleState())

	if err == nil {
		t.Fatal("Save() error = nil, want error")
	}
}

func TestStateStore_Load_CanceledContext(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	store := NewStateStore(workspace)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := store.Load(ctx, filepath.Join(root, testIssue))

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Load() error = %v, want context.Canceled", err)
	}
}

func TestStateStore_Save_CanceledContext(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	store := NewStateStore(workspace)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := store.Save(ctx, filepath.Join(root, testIssue), sampleState())

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Save() error = %v, want context.Canceled", err)
	}
}

func TestStateStore_Load_RelativeIssueDirResolvesUnderRoot(t *testing.T) {
	workspace, root := newTestWorkspace(t)
	store := NewStateStore(workspace)
	if err := store.Save(context.Background(), filepath.Join(root, testIssue), sampleState()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Load(context.Background(), testIssue)

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.IssueID != testIssue {
		t.Fatalf("Load() IssueID = %q, want %q", got.IssueID, testIssue)
	}
}
