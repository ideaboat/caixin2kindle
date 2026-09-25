// Package storage 实现输出目录布局、原子文件写入与 state.json 的读写。
//
// 它结构性地实现 app.ArtifactStore 与 port.TextStore（import 方向见 architecture.md §3）：
// 适配器不反向导入 app，接口由消费方定义、由本包隐式满足。
package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"caixin2kindle/internal/model"
)

const (
	// stateDirName 是期号目录下保存进度的隐藏目录（0700，内含登录无关的抓取状态）。
	stateDirName = ".caixin2kindle"
	// stateFileName 是进度文件名，是增量的唯一依据（spec 4.1）。
	stateFileName = "state.json"
	// articlesDirName 是正文纯文本中间产物目录。
	articlesDirName = "articles"
	// tempFilePrefix 是原子写临时文件的前缀，便于测试断言无残留。
	tempFilePrefix = ".caixin2kindle-"
)

// Workspace 管理 `<root>/<期号>/` 的输出目录布局，并提供原子写入与 `--full` 清理。
// root 即 `--out` 指定的父目录。
type Workspace struct {
	root string
}

// NewWorkspace 以输出根目录（`--out` 父目录）构造 Workspace。
func NewWorkspace(root string) *Workspace {
	return &Workspace{root: root}
}

// Root 返回输出根目录。
func (w *Workspace) Root() string {
	return w.root
}

// Path 拼接输出根目录下的路径；不解析绝对路径参数（需要解析用内部 resolve）。
func (w *Workspace) Path(elem ...string) string {
	return filepath.Join(append([]string{w.root}, elem...)...)
}

// resolve 把路径参数解析为绝对路径：绝对路径原样使用，相对路径相对输出根目录解析。
func (w *Workspace) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(w.root, path)
}

// EnsureWorkspace 创建 `<root>/<期号>/`、其 0700 的 `.caixin2kindle/` 与 `articles/`。
// 已存在时不报错（幂等）。
func (w *Workspace) EnsureWorkspace(issueDirName string) error {
	issueDir := w.Path(issueDirName)
	targets := []struct {
		path string
		perm os.FileMode
	}{
		{path: issueDir, perm: 0o755},
		{path: filepath.Join(issueDir, stateDirName), perm: 0o700},
		{path: filepath.Join(issueDir, articlesDirName), perm: 0o755},
	}

	for _, target := range targets {
		if err := os.MkdirAll(target.path, target.perm); err != nil {
			return fmt.Errorf("创建工作区目录 %s 失败：%w", target.path, err)
		}
	}
	return nil
}

// CleanWorkspace 删除本工具在该期目录内生成的已知产物（state.json、articles/ 下的普通文件、
// `<期号>.epub` / `<期号>.mobi`），供 `--full` 使用。不做目录级 RemoveAll，缺失路径不算错误；
// 真实删除失败返回包装 model.ErrUsage 的错误并附上需手动清理的路径（architecture.md §7）。
func (w *Workspace) CleanWorkspace(issueDirName string) error {
	targets, err := w.artifactPaths(issueDirName)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := removeArtifact(target); err != nil {
			return err
		}
	}
	return nil
}

// artifactPaths 列出该期目录下本工具生成的已知产物：state.json、articles/ 下的一级普通文件、
// `<期号>.epub` / `<期号>.mobi`。articles/ 不存在时视为无产物。
func (w *Workspace) artifactPaths(issueDirName string) ([]string, error) {
	issueDir := w.Path(issueDirName)
	targets := []string{
		filepath.Join(issueDir, stateDirName, stateFileName),
		filepath.Join(issueDir, issueDirName+".epub"),
		filepath.Join(issueDir, issueDirName+".mobi"),
	}

	articlesDir := filepath.Join(issueDir, articlesDirName)
	entries, err := os.ReadDir(articlesDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w：--full 清理失败，请手动检查目录 %s：%w", model.ErrUsage, articlesDir, err)
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			targets = append(targets, filepath.Join(articlesDir, entry.Name()))
		}
	}
	return targets, nil
}

// removeArtifact 删除单个已知产物；路径不存在视为已清理，其他失败包装 model.ErrUsage。
func removeArtifact(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("%w：--full 清理失败，请手动删除 %s：%w", model.ErrUsage, path, err)
}

// WriteEPUB 原子写入 `<root>/<期号>/<期号>.epub`，返回其完整路径。
func (w *Workspace) WriteEPUB(issueDirName string, data []byte) (string, error) {
	target := w.Path(issueDirName, issueDirName+".epub")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("创建期号目录失败：%w", err)
	}
	if err := atomicWriteFile(target, data, 0o600); err != nil {
		return "", fmt.Errorf("写入 EPUB %s 失败：%w", target, err)
	}
	return target, nil
}

// atomicWriteFile 把 data 原子写入 path（architecture.md §4 M2）：
// 同目录临时文件 → Write → Sync → Close → os.Rename 覆盖 → fsync 目标目录。
// 任何失败都删除临时文件，读者只会看到旧的或新的完整文件。
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	directory := filepath.Dir(path)
	temp, err := os.CreateTemp(directory, tempFilePrefix+"*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败：%w", err)
	}
	tempPath := temp.Name()

	if err := writeAndSync(temp, data, perm); err != nil {
		return discardTemp(tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return discardTemp(tempPath, fmt.Errorf("替换 %s 失败：%w", path, err))
	}
	return syncDirectory(directory)
}

// writeAndSync 把数据写入临时文件、落地并关闭；失败时确保文件已关闭以便删除。
func writeAndSync(temp *os.File, data []byte, perm os.FileMode) error {
	if err := temp.Chmod(perm); err != nil {
		return errors.Join(fmt.Errorf("设置临时文件权限失败：%w", err), temp.Close())
	}
	if _, err := temp.Write(data); err != nil {
		return errors.Join(fmt.Errorf("写入临时文件失败：%w", err), temp.Close())
	}
	if err := temp.Sync(); err != nil {
		return errors.Join(fmt.Errorf("同步临时文件失败：%w", err), temp.Close())
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败：%w", err)
	}
	return nil
}

// discardTemp 删除失败的临时文件；清理本身失败时与原错误一并返回，不静默吞错。
func discardTemp(tempPath string, cause error) error {
	if removeErr := os.Remove(tempPath); removeErr != nil {
		return errors.Join(cause, fmt.Errorf("清理临时文件 %s 失败：%w", tempPath, removeErr))
	}
	return cause
}

// syncDirectory fsync 目标目录，保证 rename 结果持久化。
func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开目录 %s 失败：%w", path, err)
	}
	if err := directory.Sync(); err != nil {
		return errors.Join(fmt.Errorf("同步目录 %s 失败：%w", path, err), directory.Close())
	}
	return directory.Close()
}
