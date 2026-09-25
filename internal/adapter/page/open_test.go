package page

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureProfileDir(t *testing.T) {
	t.Run("创建多层目录并置为 0700", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nested", "browser-profile")

		err := ensureProfileDir(dir)

		if err != nil {
			t.Fatalf("ensureProfileDir() 意外错误 = %v", err)
		}
		info, statErr := os.Stat(dir)
		if statErr != nil {
			t.Fatalf("profile 目录未创建：%v", statErr)
		}
		if !info.IsDir() {
			t.Fatalf("profile 路径不是目录：%s", dir)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("profile 目录权限 = %04o，期望 0700", got)
		}
	})

	t.Run("已有目录被强制修正为 0700", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "browser-profile")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}

		err := ensureProfileDir(dir)

		if err != nil {
			t.Fatalf("ensureProfileDir() 意外错误 = %v", err)
		}
		info, statErr := os.Stat(dir)
		if statErr != nil {
			t.Fatalf("Stat() 失败：%v", statErr)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("profile 目录权限 = %04o，期望 0700", got)
		}
	})

	t.Run("空路径返回错误", func(t *testing.T) {
		err := ensureProfileDir("")

		if err == nil {
			t.Fatalf("ensureProfileDir(\"\") 期望错误")
		}
	})

	t.Run("路径被文件占用时报错", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "occupied")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}

		err := ensureProfileDir(file)

		if err == nil {
			t.Fatalf("ensureProfileDir(文件) 期望错误")
		}
	})
}

func TestNormalizeCrashMarkerFile(t *testing.T) {
	t.Run("崩溃标记被就地替换", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "Preferences")
		original := `{"profile":{"exit_type":"Crashed","exit_count":3},"keep":1}`
		if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}

		changed, err := normalizeCrashMarkerFile(path)

		if err != nil {
			t.Fatalf("normalizeCrashMarkerFile() 意外错误 = %v", err)
		}
		if !changed {
			t.Fatalf("normalizeCrashMarkerFile() changed = false，期望 true")
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("读取结果失败：%v", readErr)
		}
		got := string(data)
		if strings.Contains(got, "Crashed") {
			t.Fatalf("Crashed 未被替换：%s", got)
		}
		if !strings.Contains(got, `"exit_type":"Normal"`) {
			t.Fatalf("未写入 Normal：%s", got)
		}
		if !strings.Contains(got, `"exit_count":3`) || !strings.Contains(got, `"keep":1`) {
			t.Fatalf("JSON 其它字段被改动：%s", got)
		}
	})

	t.Run("无崩溃标记时不写盘", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "Preferences")
		original := `{"profile":{"exit_type":"Normal"}}`
		if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}

		changed, err := normalizeCrashMarkerFile(path)

		if err != nil {
			t.Fatalf("normalizeCrashMarkerFile() 意外错误 = %v", err)
		}
		if changed {
			t.Fatalf("normalizeCrashMarkerFile() changed = true，期望 false")
		}
	})

	t.Run("文件不存在时跳过且不报错", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "Preferences")

		changed, err := normalizeCrashMarkerFile(path)

		if err != nil {
			t.Fatalf("normalizeCrashMarkerFile() 意外错误 = %v", err)
		}
		if changed {
			t.Fatalf("文件不存在时 changed = true，期望 false")
		}
	})
}

func TestReadSingletonLockTarget(t *testing.T) {
	t.Run("符号链接存在时读出目标", func(t *testing.T) {
		profileDir := t.TempDir()
		link := filepath.Join(profileDir, singletonLockName)
		if err := os.Symlink("myhost-4321", link); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}

		target, found, err := readSingletonLockTarget(profileDir)

		if err != nil {
			t.Fatalf("readSingletonLockTarget() 意外错误 = %v", err)
		}
		if !found || target != "myhost-4321" {
			t.Fatalf("readSingletonLockTarget() = (%q, %v)，期望 (\"myhost-4321\", true)", target, found)
		}
	})

	t.Run("锁不存在时返回未找到", func(t *testing.T) {
		target, found, err := readSingletonLockTarget(t.TempDir())

		if err != nil {
			t.Fatalf("readSingletonLockTarget() 意外错误 = %v", err)
		}
		if found || target != "" {
			t.Fatalf("readSingletonLockTarget() = (%q, %v)，期望 (\"\", false)", target, found)
		}
	})

	t.Run("普通文件不是有效锁，返回错误供上层按陈旧处理", func(t *testing.T) {
		profileDir := t.TempDir()
		link := filepath.Join(profileDir, singletonLockName)
		if err := os.WriteFile(link, []byte("not a symlink"), 0o600); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}

		target, found, err := readSingletonLockTarget(profileDir)

		if err == nil {
			t.Fatalf("普通文件应返回错误，实际 (%q, %v, nil)", target, found)
		}
	})
}

func TestSingletonLockBusy(t *testing.T) {
	const hostname = "myhost"

	tests := []struct {
		name     string
		target   string
		hostname string
		alive    func(int) bool
		want     bool
	}{
		{name: "同机同 pid 且进程存活视为占用", target: "myhost-100", hostname: hostname, alive: func(int) bool { return true }, want: true},
		{name: "同机但进程已退出视为陈旧", target: "myhost-100", hostname: hostname, alive: func(int) bool { return false }, want: false},
		{name: "异机同 pid 视为陈旧", target: "otherhost-100", hostname: hostname, alive: func(int) bool { return true }, want: false},
		{name: "无法解析的锁视为陈旧", target: "garbage", hostname: hostname, alive: func(int) bool { return true }, want: false},
		{name: "主机名未知且 pid 存活时保守视为占用", target: "myhost-100", hostname: "", alive: func(int) bool { return true }, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := singletonLockBusy(test.target, test.hostname, test.alive)

			if got != test.want {
				t.Fatalf("singletonLockBusy(%q, %q) = %v，期望 %v", test.target, test.hostname, got, test.want)
			}
		})
	}
}

func TestWarnHelpersNeverPanicOnNilWarn(t *testing.T) {
	client := &Client{}

	client.warnf("这条告警应被安全丢弃：%d", 1)
}
