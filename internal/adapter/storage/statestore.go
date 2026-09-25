package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"caixin2kindle/internal/model"
)

// StateStore 实现 app.StateRepository：state.json 的原子读写，是进度的唯一来源（spec 4.1）。
type StateStore struct {
	// Workspace 提供相对 issueDir 的解析基准。
	Workspace *Workspace
	// Warn 可为空；解码失败或 schema 不匹配时告警（architecture.md §4：降级为全量、不迁移）。
	Warn func(msg string)
}

// NewStateStore 以给定工作区构造 StateStore。
func NewStateStore(workspace *Workspace) *StateStore {
	return &StateStore{Workspace: workspace}
}

// Load 读取 issueDir 下的 state.json：
//   - 文件不存在 → 返回空状态与 nil（首次运行）；
//   - 解码失败或 schema 版本不匹配 → 告警并返回空状态与 nil（降级为全量重跑，不做迁移）；
//   - 其他读取失败 → 返回包装错误。
//
// issueDir 通常是绝对期号目录；相对路径相对 Workspace 根目录解析。
func (s *StateStore) Load(ctx context.Context, issueDir string) (model.State, error) {
	if err := ctx.Err(); err != nil {
		return model.State{}, fmt.Errorf("读取状态被取消：%w", err)
	}

	path := s.statePath(issueDir)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.State{}, nil
		}
		return model.State{}, fmt.Errorf("打开状态文件 %s 失败：%w", path, err)
	}

	state, decodeErr := decodeState(file)
	if closeErr := file.Close(); closeErr != nil {
		return model.State{}, fmt.Errorf("关闭状态文件 %s 失败：%w", path, closeErr)
	}
	if decodeErr != nil {
		s.warn(fmt.Sprintf("状态文件 %s 无法解析（%v），已忽略既有进度并按全量重跑", path, decodeErr))
		return model.State{}, nil
	}
	if state.SchemaVersion != model.SchemaVersion {
		s.warn(fmt.Sprintf("状态文件 %s 结构版本为 %d、当前支持 %d，已忽略既有进度并按全量重跑",
			path, state.SchemaVersion, model.SchemaVersion))
		return model.State{}, nil
	}
	return state, nil
}

// Save 以稳定字段顺序序列化 state 并原子写入 issueDir 下的 state.json（0600）。
// 调用方必须遵循“先写正文并 fsync、再写 state”的顺序（architecture.md §7）。
func (s *StateStore) Save(ctx context.Context, issueDir string, state model.State) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("保存状态被取消：%w", err)
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("序列化状态失败：%w", err)
	}

	path := s.statePath(issueDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建状态目录失败：%w", err)
	}
	if err := atomicWriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("写入状态文件 %s 失败：%w", path, err)
	}
	return nil
}

// statePath 返回状态文件路径；相对 issueDir 相对 Workspace 根目录解析。
func (s *StateStore) statePath(issueDir string) string {
	resolved := issueDir
	if !filepath.IsAbs(resolved) && s.Workspace != nil {
		resolved = s.Workspace.resolve(resolved)
	}
	return filepath.Join(resolved, stateDirName, stateFileName)
}

// decodeState 用 DisallowUnknownFields 严格解码状态文件（architecture.md §4）。
func decodeState(file *os.File) (model.State, error) {
	var state model.State
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return model.State{}, err
	}
	return state, nil
}

// warn 在配置了回调时输出一条降级告警。
func (s *StateStore) warn(msg string) {
	if s.Warn != nil {
		s.Warn(msg)
	}
}
