package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WriteArticleText 写入一篇正文：先创建父目录（0755），再原子写入（0600）。
// path 可以是绝对路径，也可以是相对输出根目录的路径（state.json 的 text_path 即后者）。
func (w *Workspace) WriteArticleText(path, body string) error {
	target := w.resolve(path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("创建正文目录失败：%w", err)
	}
	if err := atomicWriteFile(target, []byte(body), 0o600); err != nil {
		return fmt.Errorf("写入正文 %s 失败：%w", target, err)
	}
	return nil
}

// ReadArticleText 读取一篇正文；文件缺失时返回的错误可用 errors.Is(err, os.ErrNotExist) 判定。
func (w *Workspace) ReadArticleText(path string) (string, error) {
	target := w.resolve(path)
	data, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("读取正文 %s 失败：%w", target, err)
	}
	return string(data), nil
}

// Exists 报告路径是否存在；不存在返回 (false, nil)，其他探测失败返回错误。
func (w *Workspace) Exists(path string) (bool, error) {
	target := w.resolve(path)
	_, err := os.Stat(target)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("检查路径 %s 失败：%w", target, err)
	}
}
