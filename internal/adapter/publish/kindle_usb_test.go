package publish

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
)

func TestNewUSBDevice(t *testing.T) {
	cases := []struct {
		name      string
		cfg       config.Config
		wantRoot  string
		wantDocs  string
		wantMount string
	}{
		{
			name:     "空配置补齐默认值",
			cfg:      config.Config{},
			wantRoot: defaultVolumesRoot,
			wantDocs: defaultDocumentsDirName,
		},
		{
			name:      "沿用配置中的挂载点与目录名",
			cfg:       config.Config{KindleMount: "/Volumes/Paperwhite", DocumentsDirName: "docs"},
			wantRoot:  defaultVolumesRoot,
			wantDocs:  "docs",
			wantMount: "/Volumes/Paperwhite",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			device := NewUSBDevice(tc.cfg)

			// Assert
			if device == nil {
				t.Fatal("NewUSBDevice 不应返回 nil")
			}
			if device.VolumesRoot != tc.wantRoot {
				t.Errorf("VolumesRoot = %q，期望 %q", device.VolumesRoot, tc.wantRoot)
			}
			if device.DocumentsDirName != tc.wantDocs {
				t.Errorf("DocumentsDirName = %q，期望 %q", device.DocumentsDirName, tc.wantDocs)
			}
			if device.Mount != tc.wantMount {
				t.Errorf("Mount = %q，期望 %q", device.Mount, tc.wantMount)
			}
		})
	}
}

func TestUSBDeviceVolumes(t *testing.T) {
	const root = "/Volumes"
	entries := []os.DirEntry{
		fakeEntry("Kindle", true),
		fakeEntry("USB", true),
		fakeEntry("README.txt", false),
		fakeEntry("Ghost", true),
	}
	stats := map[string]fakeFileInfo{
		"/Volumes/Kindle":              fakeDirMode("Kindle", 0o755),
		"/Volumes/Kindle/documents":    fakeDir("documents"),
		"/Volumes/USB":                 fakeDirMode("USB", 0o555),
		"/Volumes/SideMount":           fakeDirMode("SideMount", 0o700),
		"/Volumes/SideMount/documents": fakeDir("documents"),
		"/Volumes/README.txt":          fakeFile("README.txt"),
	}
	baseVolumes := []model.Volume{
		{Path: "/Volumes/Kindle", HasDocuments: true, Writable: true},
		{Path: "/Volumes/USB", HasDocuments: false, Writable: false},
		{Path: "/Volumes/Ghost", HasDocuments: false, Writable: false},
	}

	cases := []struct {
		name  string
		mount string
		want  []model.Volume
	}{
		{name: "只枚举目录并探测 documents 与写权限", mount: "/Volumes/Kindle", want: baseVolumes},
		{
			name:  "显式挂载点不在 Volumes 下时追加为候选",
			mount: "/Volumes/SideMount",
			want: append(append([]model.Volume{}, baseVolumes...), model.Volume{
				Path: "/Volumes/SideMount", HasDocuments: true, Writable: true,
			}),
		},
		{name: "显式挂载点带尾斜杠且已在列表时不重复", mount: "/Volumes/Kindle/", want: baseVolumes},
		{name: "显式挂载点不存在则不追加", mount: "/Volumes/Nope", want: baseVolumes},
		{name: "显式挂载点是文件则不追加", mount: "/Volumes/README.txt", want: baseVolumes},
		{name: "未配置挂载点则不追加", mount: "", want: baseVolumes},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			device := &USBDevice{
				VolumesRoot:      root,
				DocumentsDirName: defaultDocumentsDirName,
				Mount:            tc.mount,
				ReadDir:          func(string) ([]os.DirEntry, error) { return entries, nil },
				Stat:             statFromMap(stats),
			}

			// Act
			volumes, err := device.Volumes(context.Background())

			// Assert
			if err != nil {
				t.Fatalf("Volumes 返回错误：%v", err)
			}
			if !reflect.DeepEqual(volumes, tc.want) {
				t.Errorf("Volumes = %+v，期望 %+v", volumes, tc.want)
			}
		})
	}
}

func TestUSBDeviceVolumes_DocumentsExistsAsFile(t *testing.T) {
	// Arrange
	stats := map[string]fakeFileInfo{
		"/Volumes/Kindle":           fakeDirMode("Kindle", 0o755),
		"/Volumes/Kindle/documents": fakeFile("documents"),
	}
	device := &USBDevice{
		VolumesRoot:      "/Volumes",
		DocumentsDirName: defaultDocumentsDirName,
		ReadDir:          func(string) ([]os.DirEntry, error) { return []os.DirEntry{fakeEntry("Kindle", true)}, nil },
		Stat:             statFromMap(stats),
	}

	// Act
	volumes, err := device.Volumes(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("Volumes 返回错误：%v", err)
	}
	if len(volumes) != 1 || !volumes[0].HasDocuments {
		t.Errorf("documents 存在即应标记 HasDocuments，实际：%+v", volumes)
	}
}

func TestUSBDeviceVolumes_ReadDirError(t *testing.T) {
	// Arrange
	device := &USBDevice{
		VolumesRoot: "/Volumes",
		ReadDir: func(string) ([]os.DirEntry, error) {
			return nil, errors.New("permission denied")
		},
	}

	// Act
	_, err := device.Volumes(context.Background())

	// Assert
	if !errors.Is(err, model.ErrCopy) {
		t.Fatalf("读取挂载点失败应包装 model.ErrCopy，实际：%v", err)
	}
}

func TestUSBDeviceVolumes_CancelledContext(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	device := &USBDevice{VolumesRoot: "/Volumes"}

	// Act
	_, err := device.Volumes(ctx)

	// Assert
	if !errors.Is(err, model.ErrCopy) {
		t.Fatalf("上下文取消应包装 model.ErrCopy，实际：%v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("应保留 context.Canceled 因果链，实际：%v", err)
	}
}

func TestUSBDevice_DefaultFuncs(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, defaultDocumentsDirName), 0o755); err != nil {
		t.Fatalf("准备 documents 目录失败：%v", err)
	}
	device := &USBDevice{VolumesRoot: dir, DocumentsDirName: defaultDocumentsDirName}

	// Act
	volumes, err := device.Volumes(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("默认 ReadDir/Stat 应能扫描真实目录，实际错误：%v", err)
	}
	documentsPath := filepath.Join(dir, defaultDocumentsDirName)
	if len(volumes) != 1 || volumes[0].Path != documentsPath || volumes[0].HasDocuments {
		t.Errorf("Volumes = %+v，期望单卷 %q（无 documents 子目录）", volumes, documentsPath)
	}
}

func TestUSBDevice_GetterDefaults(t *testing.T) {
	// Arrange
	device := &USBDevice{}

	// Act
	root := device.volumesRoot()
	docs := device.documentsDirName()

	// Assert
	if root != defaultVolumesRoot {
		t.Errorf("volumesRoot = %q，期望 %q", root, defaultVolumesRoot)
	}
	if docs != defaultDocumentsDirName {
		t.Errorf("documentsDirName = %q，期望 %q", docs, defaultDocumentsDirName)
	}
}

func TestUSBDeviceEnsureDocuments(t *testing.T) {
	t.Run("创建缺失的 documents 目录", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		device := &USBDevice{DocumentsDirName: defaultDocumentsDirName}
		volume := model.Volume{Path: dir, Writable: true}

		// Act
		err := device.EnsureDocuments(context.Background(), volume)

		// Assert
		if err != nil {
			t.Fatalf("EnsureDocuments 返回错误：%v", err)
		}
		info, statErr := os.Stat(filepath.Join(dir, defaultDocumentsDirName))
		if statErr != nil || !info.IsDir() {
			t.Fatalf("应创建 documents 目录，实际 err=%v info=%v", statErr, info)
		}
	})

	t.Run("documents 位置被文件占用时报错", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, defaultDocumentsDirName), []byte("x"), 0o644); err != nil {
			t.Fatalf("准备占位文件失败：%v", err)
		}
		device := &USBDevice{DocumentsDirName: defaultDocumentsDirName}

		// Act
		err := device.EnsureDocuments(context.Background(), model.Volume{Path: dir})

		// Assert
		if !errors.Is(err, model.ErrCopy) {
			t.Fatalf("创建目录失败应包装 model.ErrCopy，实际：%v", err)
		}
	})

	t.Run("上下文取消时报错", func(t *testing.T) {
		// Arrange
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		device := &USBDevice{DocumentsDirName: defaultDocumentsDirName}

		// Act
		err := device.EnsureDocuments(ctx, model.Volume{Path: t.TempDir()})

		// Assert
		if !errors.Is(err, model.ErrCopy) {
			t.Fatalf("上下文取消应包装 model.ErrCopy，实际：%v", err)
		}
	})
}

func TestUSBDeviceCopy(t *testing.T) {
	t.Run("按字节复制到 documents", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		source := filepath.Join(dir, "issue.mobi")
		content := []byte("MOBI-BYTES-\x00\x01\x02-中文内容")
		if err := os.WriteFile(source, content, 0o644); err != nil {
			t.Fatalf("准备源文件失败：%v", err)
		}
		volume := model.Volume{Path: dir, Writable: true}
		device := &USBDevice{DocumentsDirName: defaultDocumentsDirName}
		if err := device.EnsureDocuments(context.Background(), volume); err != nil {
			t.Fatalf("准备 documents 目录失败：%v", err)
		}

		// Act
		err := device.Copy(context.Background(), volume, source, "issue.mobi")

		// Assert
		if err != nil {
			t.Fatalf("Copy 返回错误：%v", err)
		}
		copied, readErr := os.ReadFile(filepath.Join(dir, defaultDocumentsDirName, "issue.mobi"))
		if readErr != nil {
			t.Fatalf("读取目标文件失败：%v", readErr)
		}
		if !reflect.DeepEqual(copied, content) {
			t.Errorf("复制结果 = %q，期望 %q", copied, content)
		}
	})

	t.Run("覆盖同名文件", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		source := filepath.Join(dir, "issue.mobi")
		if err := os.WriteFile(source, []byte("new"), 0o644); err != nil {
			t.Fatalf("准备源文件失败：%v", err)
		}
		documents := filepath.Join(dir, defaultDocumentsDirName)
		if err := os.MkdirAll(documents, 0o755); err != nil {
			t.Fatalf("准备 documents 目录失败：%v", err)
		}
		stale := filepath.Join(documents, "issue.mobi")
		if err := os.WriteFile(stale, []byte("stale-content-longer"), 0o644); err != nil {
			t.Fatalf("准备旧文件失败：%v", err)
		}
		device := &USBDevice{DocumentsDirName: defaultDocumentsDirName}

		// Act
		err := device.Copy(context.Background(), model.Volume{Path: dir}, source, "issue.mobi")

		// Assert
		if err != nil {
			t.Fatalf("Copy 返回错误：%v", err)
		}
		copied, readErr := os.ReadFile(stale)
		if readErr != nil {
			t.Fatalf("读取目标文件失败：%v", readErr)
		}
		if string(copied) != "new" {
			t.Errorf("同名文件应被覆盖，实际内容 %q", copied)
		}
	})

	t.Run("源文件不存在时报错", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		device := &USBDevice{DocumentsDirName: defaultDocumentsDirName}
		volume := model.Volume{Path: dir}
		if err := device.EnsureDocuments(context.Background(), volume); err != nil {
			t.Fatalf("准备 documents 目录失败：%v", err)
		}

		// Act
		err := device.Copy(context.Background(), volume, filepath.Join(dir, "missing.mobi"), "missing.mobi")

		// Assert
		if !errors.Is(err, model.ErrCopy) {
			t.Fatalf("源文件缺失应包装 model.ErrCopy，实际：%v", err)
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("应保留 os.ErrNotExist 因果链，实际：%v", err)
		}
	})

	t.Run("documents 目录不存在时报错", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		source := filepath.Join(dir, "issue.mobi")
		if err := os.WriteFile(source, []byte("data"), 0o644); err != nil {
			t.Fatalf("准备源文件失败：%v", err)
		}
		device := &USBDevice{DocumentsDirName: defaultDocumentsDirName}

		// Act
		err := device.Copy(context.Background(), model.Volume{Path: dir}, source, "issue.mobi")

		// Assert
		if !errors.Is(err, model.ErrCopy) {
			t.Fatalf("目标目录缺失应包装 model.ErrCopy，实际：%v", err)
		}
	})

	t.Run("文件名为空时报错", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		device := &USBDevice{DocumentsDirName: defaultDocumentsDirName}

		// Act
		err := device.Copy(context.Background(), model.Volume{Path: dir}, "any.mobi", "")

		// Assert
		if !errors.Is(err, model.ErrCopy) {
			t.Fatalf("空文件名应包装 model.ErrCopy，实际：%v", err)
		}
	})

	t.Run("文件名含路径分隔符时报错", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		source := filepath.Join(dir, "issue.mobi")
		if err := os.WriteFile(source, []byte("data"), 0o644); err != nil {
			t.Fatalf("准备源文件失败：%v", err)
		}
		device := &USBDevice{DocumentsDirName: defaultDocumentsDirName}

		// Act
		err := device.Copy(context.Background(), model.Volume{Path: dir}, source, "../escape.mobi")

		// Assert
		if !errors.Is(err, model.ErrCopy) {
			t.Fatalf("越界文件名应包装 model.ErrCopy，实际：%v", err)
		}
	})

	t.Run("上下文取消时报错", func(t *testing.T) {
		// Arrange
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		device := &USBDevice{DocumentsDirName: defaultDocumentsDirName}

		// Act
		err := device.Copy(ctx, model.Volume{Path: t.TempDir()}, "any.mobi", "any.mobi")

		// Assert
		if !errors.Is(err, model.ErrCopy) {
			t.Fatalf("上下文取消应包装 model.ErrCopy，实际：%v", err)
		}
	})
}
