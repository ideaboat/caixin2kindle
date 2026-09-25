package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"caixin2kindle/internal/model"
)

// FakeTextStore 是基于内存 map 的 port.TextStore 实现，key 为正文文件路径。
type FakeTextStore struct {
	Files     map[string]string // 路径 → 正文
	ExistsErr error             // 非 nil 时 Exists 直接失败
	ReadErr   error             // 非 nil 时 ReadArticleText 直接失败
}

var _ interface {
	Exists(path string) (bool, error)
	ReadArticleText(path string) (string, error)
} = (*FakeTextStore)(nil)

// Exists 报告路径是否在内存中。
func (s *FakeTextStore) Exists(path string) (bool, error) {
	if s.ExistsErr != nil {
		return false, s.ExistsErr
	}
	_, found := s.Files[path]
	return found, nil
}

// ReadArticleText 返回内存中的正文；缺失时返回 os.ErrNotExist，与真实文件系统语义一致。
func (s *FakeTextStore) ReadArticleText(path string) (string, error) {
	if s.ReadErr != nil {
		return "", s.ReadErr
	}
	content, found := s.Files[path]
	if !found {
		return "", os.ErrNotExist
	}
	return content, nil
}

// FakeStateRepository 是基于内存 map 的 app.StateRepository 实现，key 为输出目录。
type FakeStateRepository struct {
	States  map[string]model.State // 输出目录 → state
	LoadErr error
	SaveErr error
	Saves   []model.State // 记录每次 Save 的内容，供写入顺序断言
}

// Load 读取输出目录对应的 state；不存在时返回空 State。
func (r *FakeStateRepository) Load(_ context.Context, dir string) (model.State, error) {
	if r.LoadErr != nil {
		return model.State{}, r.LoadErr
	}
	if r.States == nil {
		return model.State{}, nil
	}
	return r.States[dir], nil
}

// Save 写入输出目录对应的 state，并记录一份副本。
func (r *FakeStateRepository) Save(_ context.Context, dir string, state model.State) error {
	if r.SaveErr != nil {
		return r.SaveErr
	}
	if r.States == nil {
		r.States = make(map[string]model.State)
	}
	r.States[dir] = state
	r.Saves = append(r.Saves, state)
	return nil
}

// FakeArtifactStore 是基于内存 map 的 app.ArtifactStore 实现。
type FakeArtifactStore struct {
	Dir        string            // 输出目录根
	Files      map[string][]byte // 路径 → 内容
	WriteErr   error             // 非 nil 时写正文失败
	EPUBErr    error             // 非 nil 时写 EPUB 失败
	EnsureErr  error             // 非 nil 时 EnsureWorkspace 失败
	CleanErr   error             // 非 nil 时 CleanWorkspace 失败
	EPUBWrites int               // EPUB 写入次数
	Ensured    []string          // 记录 EnsureWorkspace 调用
	Cleans     []string          // 记录 CleanWorkspace 调用
}

// Path 拼接输出目录内的路径。
func (s *FakeArtifactStore) Path(elem ...string) string {
	return filepath.Join(append([]string{s.Dir}, elem...)...)
}

// resolve 与真实 Workspace 保持一致：绝对路径原样使用，相对路径相对输出根解析。
func (s *FakeArtifactStore) resolve(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(s.Dir, path)
}

// WriteArticleText 写入正文文件。
func (s *FakeArtifactStore) WriteArticleText(path, body string) error {
	if s.WriteErr != nil {
		return s.WriteErr
	}
	s.ensure()
	s.Files[s.resolve(path)] = []byte(body)
	return nil
}

// ReadArticleText 读取正文文件。
func (s *FakeArtifactStore) ReadArticleText(path string) (string, error) {
	content, found := s.Files[s.resolve(path)]
	if !found {
		return "", os.ErrNotExist
	}
	return string(content), nil
}

// Exists 报告路径是否存在。
func (s *FakeArtifactStore) Exists(path string) (bool, error) {
	_, found := s.Files[s.resolve(path)]
	return found, nil
}

// WriteEPUB 记录 EPUB 内容，返回其路径。
func (s *FakeArtifactStore) WriteEPUB(issueDirName string, data []byte) (string, error) {
	if s.EPUBErr != nil {
		return "", s.EPUBErr
	}
	s.ensure()
	s.EPUBWrites++
	path := filepath.Join(s.Dir, issueDirName, issueDirName+".epub")
	s.Files[path] = data
	return path, nil
}

// EnsureWorkspace 记录一次工作区创建。
func (s *FakeArtifactStore) EnsureWorkspace(issueDirName string) error {
	if s.EnsureErr != nil {
		return s.EnsureErr
	}
	s.Ensured = append(s.Ensured, issueDirName)
	return nil
}

// CleanWorkspace 删除该期目录下的全部内存文件，模拟 --full 的清理。
func (s *FakeArtifactStore) CleanWorkspace(issueDirName string) error {
	if s.CleanErr != nil {
		return s.CleanErr
	}
	s.ensure()
	prefix := filepath.Join(s.Dir, issueDirName) + string(filepath.Separator)
	for path := range s.Files {
		if strings.HasPrefix(path, prefix) {
			delete(s.Files, path)
		}
	}
	s.Cleans = append(s.Cleans, issueDirName)
	return nil
}

// Seed 预置一个文件，供测试安排既有产物。
func (s *FakeArtifactStore) Seed(path string, content []byte) {
	s.ensure()
	s.Files[s.resolve(path)] = content
}

func (s *FakeArtifactStore) ensure() {
	if s.Files == nil {
		s.Files = make(map[string][]byte)
	}
}

// HTMLOf 返回按路径排序的内容摘要，便于断言产物的存在性而非字节细节。
func (s *FakeArtifactStore) Paths() []string {
	paths := make([]string, 0, len(s.Files))
	for path := range s.Files {
		paths = append(paths, path)
	}
	return paths
}

// Describe 返回可读的 store 快照，用于失败信息。
func (s *FakeArtifactStore) Describe() string {
	var builder strings.Builder
	for _, path := range s.Paths() {
		fmt.Fprintf(&builder, "%s(%d bytes)\n", path, len(s.Files[path]))
	}
	return builder.String()
}
