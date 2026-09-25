package publish

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// fakeFileInfo 是 os.FileInfo 的最小实现，供 Probe / USBDevice 的 Stat 注入。
type fakeFileInfo struct {
	name string
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return nil }

// fakeFile 返回一个普通文件的 FileInfo。
func fakeFile(name string) fakeFileInfo { return fakeFileInfo{name: name, mode: 0o644} }

// fakeDir 返回一个 0755 目录的 FileInfo。
func fakeDir(name string) fakeFileInfo { return fakeDirMode(name, 0o755) }

// fakeDirMode 返回指定权限目录的 FileInfo。
func fakeDirMode(name string, perm os.FileMode) fakeFileInfo {
	return fakeFileInfo{name: name, mode: os.ModeDir | perm}
}

// fakeDirEntry 是 os.DirEntry 的最小实现。
type fakeDirEntry struct {
	name string
	dir  bool
}

func (e fakeDirEntry) Name() string      { return e.name }
func (e fakeDirEntry) IsDir() bool       { return e.dir }
func (e fakeDirEntry) Type() os.FileMode { return e.info().Mode() }
func (e fakeDirEntry) Info() (os.FileInfo, error) {
	return e.info(), nil
}

func (e fakeDirEntry) info() fakeFileInfo {
	if e.dir {
		return fakeDir("")
	}
	return fakeFile("")
}

// fakeEntry 构造一个目录或文件条目。
func fakeEntry(name string, dir bool) fakeDirEntry { return fakeDirEntry{name: name, dir: dir} }

// statFromMap 返回按路径查表的 Stat fake；未登记路径一律返回 os.ErrNotExist。
func statFromMap(entries map[string]fakeFileInfo) func(string) (os.FileInfo, error) {
	return func(path string) (os.FileInfo, error) {
		if info, ok := entries[path]; ok {
			return info, nil
		}
		return nil, &os.PathError{Op: "stat", Path: path, Err: os.ErrNotExist}
	}
}

// availableLookPath 返回只认给定命令名的 LookPath fake，其余一律 exec.ErrNotFound。
func availableLookPath(names ...string) func(string) (string, error) {
	available := make(map[string]string, len(names))
	for _, name := range names {
		available[name] = filepath.Join("/usr/bin", name)
	}
	return func(name string) (string, error) {
		if path, ok := available[name]; ok {
			return path, nil
		}
		return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
}
