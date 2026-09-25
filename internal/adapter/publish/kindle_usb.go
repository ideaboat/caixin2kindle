package publish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
)

// USB 相关默认值：与 config 默认值表（architecture.md §7）保持一致，供零值设备兜底。
const (
	defaultVolumesRoot      = "/Volumes"
	defaultDocumentsDirName = "documents"
)

// USBDevice 实现 app.VolumeScanner 与 app.DeviceWriter：只读枚举候选挂载卷，
// 并在目标卷上按需创建 documents/ 后覆盖式复制成品（spec 3.9、M7/C2）。
// ReadDir/Stat 可注入，扫描类测试因此无需真实挂载点。
type USBDevice struct {
	VolumesRoot      string                              // 挂载点根目录，默认 "/Volumes"
	DocumentsDirName string                              // 目标子目录名，默认 "documents"
	Mount            string                              // cfg.KindleMount：显式挂载点也纳入候选（可能位于 /Volumes 之外）
	ReadDir          func(string) ([]os.DirEntry, error) // nil → os.ReadDir
	Stat             func(string) (os.FileInfo, error)   // nil → os.Stat
}

// NewUSBDevice 用配置构造设备适配器：根目录固定为 /Volumes（macOS），
// 目标子目录名与显式挂载点取自 cfg，空值回退到默认。
func NewUSBDevice(cfg config.Config) *USBDevice {
	documents := cfg.DocumentsDirName
	if documents == "" {
		documents = defaultDocumentsDirName
	}
	return &USBDevice{
		VolumesRoot:      defaultVolumesRoot,
		DocumentsDirName: documents,
		Mount:            cfg.KindleMount,
	}
}

// Volumes 只读枚举候选挂载卷：<VolumesRoot> 下的目录，外加显式 Mount（存在且是目录、且尚未列出时）。
// 每个卷带 HasDocuments（<卷>/documents 是否存在）与 Writable（权限位 0o200）。
// 扫描阶段**绝不创建任何目录**（C2）；读取 VolumesRoot 失败返回包装 model.ErrCopy 的错误。
func (d *USBDevice) Volumes(ctx context.Context) ([]model.Volume, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w：扫描挂载点被取消：%w", model.ErrCopy, err)
	}

	root := d.volumesRoot()
	entries, err := d.readDir()(root)
	if err != nil {
		return nil, fmt.Errorf("%w：无法读取挂载点目录 %s：%w", model.ErrCopy, root, err)
	}

	volumes := make([]model.Volume, 0, len(entries)+1)
	seen := make(map[string]struct{}, len(entries)+1)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		volumes = append(volumes, d.inspect(path))
		seen[filepath.Clean(path)] = struct{}{}
	}

	if d.Mount == "" {
		return volumes, nil
	}
	mount := filepath.Clean(d.Mount)
	if _, listed := seen[mount]; listed {
		return volumes, nil
	}
	info, statErr := d.stat()(mount)
	if statErr != nil || !info.IsDir() {
		return volumes, nil
	}
	return append(volumes, d.inspect(mount)), nil
}

// inspect 读取单个候选卷的两项事实：是否含 documents/（存在性判定）与是否可写（权限位 0o200）。
func (d *USBDevice) inspect(path string) model.Volume {
	_, docErr := d.stat()(filepath.Join(path, d.documentsDirName()))
	writable := false
	if info, err := d.stat()(path); err == nil && info.Mode().Perm()&0o200 != 0 {
		writable = true
	}
	return model.Volume{Path: path, HasDocuments: docErr == nil, Writable: writable}
}

// EnsureDocuments 在目标卷上创建 <vol.Path>/<DocumentsDirName>（0755，已存在则无操作）。
// 仅在用户显式指定挂载点时由 app 调用（C2）；失败返回包装 model.ErrCopy 的错误。
func (d *USBDevice) EnsureDocuments(ctx context.Context, v model.Volume) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w：创建 documents 目录被取消：%w", model.ErrCopy, err)
	}
	target := filepath.Join(v.Path, d.documentsDirName())
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("%w：无法创建目录 %s：%w", model.ErrCopy, target, err)
	}
	return nil
}

// Copy 把 src 覆盖式复制为 <vol.Path>/<DocumentsDirName>/<name>
// （O_CREATE|O_TRUNC|O_WRONLY，0644），写入后 Sync 再关闭，确保拔出设备前数据已落盘。
// 文件名不得为空、不得含路径分隔符（防越界写入）；任何失败返回包装 model.ErrCopy 的错误。
func (d *USBDevice) Copy(ctx context.Context, v model.Volume, src, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w：复制被取消：%w", model.ErrCopy, err)
	}
	if !validTargetName(name) {
		return fmt.Errorf("%w：目标文件名非法：%q", model.ErrCopy, name)
	}

	target := filepath.Join(v.Path, d.documentsDirName(), name)
	if err := copyFile(ctx, src, target); err != nil {
		return fmt.Errorf("%w：复制 %s 到 %s 失败：%w", model.ErrCopy, src, target, err)
	}
	return nil
}

// validTargetName 判定目标文件名是否安全：非空、非 "."/".."，且不含任何路径分隔符。
func validTargetName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return filepath.Base(name) == name
}

// copyFile 以流式方式覆盖复制单个文件，写入后 Sync 并关闭；关闭错误同样上报，不静默吞掉。
func copyFile(ctx context.Context, src, target string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := source.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	destination, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(destination, source); err != nil {
		return errors.Join(err, destination.Close())
	}
	if err := destination.Sync(); err != nil {
		return errors.Join(err, destination.Close())
	}
	return destination.Close()
}

// volumesRoot 返回生效的挂载点根目录：空值回退到 /Volumes。
func (d *USBDevice) volumesRoot() string {
	if d.VolumesRoot != "" {
		return d.VolumesRoot
	}
	return defaultVolumesRoot
}

// documentsDirName 返回生效的目标子目录名：空值回退到 documents。
func (d *USBDevice) documentsDirName() string {
	if d.DocumentsDirName != "" {
		return d.DocumentsDirName
	}
	return defaultDocumentsDirName
}

// readDir 返回生效的目录读取实现：未注入时使用 os.ReadDir。
func (d *USBDevice) readDir() func(string) ([]os.DirEntry, error) {
	if d.ReadDir != nil {
		return d.ReadDir
	}
	return os.ReadDir
}

// stat 返回生效的 Stat 实现：未注入时使用 os.Stat。
func (d *USBDevice) stat() func(string) (os.FileInfo, error) {
	if d.Stat != nil {
		return d.Stat
	}
	return os.Stat
}
