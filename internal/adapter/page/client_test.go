package page

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
)

func TestIsTimeoutError(t *testing.T) {
	cancelledCtx, cancelCancelled := context.WithCancel(context.Background())
	cancelCancelled()
	expiredCtx, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()

	tests := []struct {
		name     string
		err      error
		contexts []context.Context
		want     bool
	}{
		{name: "错误本身是 DeadlineExceeded", err: context.DeadlineExceeded, want: true},
		{name: "错误包装了 DeadlineExceeded", err: fmt.Errorf("导航：%w", context.DeadlineExceeded), want: true},
		{name: "传入的 ctx 已过 deadline", err: errors.New("boom"), contexts: []context.Context{expiredCtx}, want: true},
		{name: "已取消的 ctx 不算超时", err: errors.New("boom"), contexts: []context.Context{cancelledCtx}, want: false},
		{name: "普通错误且无 ctx", err: errors.New("boom"), want: false},
		{name: "容忍 nil ctx", err: errors.New("boom"), contexts: []context.Context{nil}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := isTimeoutError(test.err, test.contexts...)

			if got != test.want {
				t.Fatalf("isTimeoutError(%v, %v) = %v，期望 %v", test.err, test.contexts, got, test.want)
			}
		})
	}
}

func TestCheckSingletonLock(t *testing.T) {
	hostname := testHostname(t)

	t.Run("无锁文件时不报错", func(t *testing.T) {
		client := &Client{hostname: hostname}

		err := client.checkSingletonLock(t.TempDir())

		if err != nil {
			t.Fatalf("checkSingletonLock() 意外错误 = %v", err)
		}
	})

	t.Run("陈旧锁被清理并告警", func(t *testing.T) {
		const impossiblePID = 1 << 30
		if processAlive(impossiblePID) {
			t.Skipf("pid %d 在本机意外存活，跳过", impossiblePID)
		}
		dir := t.TempDir()
		link := filepath.Join(dir, singletonLockName)
		if err := os.Symlink(fmt.Sprintf("%s-%d", hostname, impossiblePID), link); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}
		var warnings []string
		client := &Client{hostname: hostname, Warn: func(message string) { warnings = append(warnings, message) }}

		err := client.checkSingletonLock(dir)

		if err != nil {
			t.Fatalf("checkSingletonLock() 意外错误 = %v", err)
		}
		if _, statErr := os.Lstat(link); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("陈旧锁未被清理：Lstat err = %v", statErr)
		}
		if len(warnings) == 0 {
			t.Fatalf("清理陈旧锁时应告警")
		}
	})

	t.Run("本机活进程占用时返回依赖错误且保留锁", func(t *testing.T) {
		dir := t.TempDir()
		link := filepath.Join(dir, singletonLockName)
		if err := os.Symlink(fmt.Sprintf("%s-%d", hostname, os.Getpid()), link); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}
		client := &Client{hostname: hostname}

		err := client.checkSingletonLock(dir)

		if !errors.Is(err, model.ErrDependency) {
			t.Fatalf("checkSingletonLock() error = %v，期望 errors.Is(_, model.ErrDependency)", err)
		}
		if _, statErr := os.Lstat(link); statErr != nil {
			t.Fatalf("占用中的锁不得被删除：Lstat err = %v", statErr)
		}
	})

	t.Run("存在但不是符号链接时按陈旧处理", func(t *testing.T) {
		dir := t.TempDir()
		link := filepath.Join(dir, singletonLockName)
		if err := os.WriteFile(link, []byte("not a symlink"), 0o600); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}
		var warnings []string
		client := &Client{hostname: hostname, Warn: func(message string) { warnings = append(warnings, message) }}

		err := client.checkSingletonLock(dir)

		if err != nil {
			t.Fatalf("checkSingletonLock() 意外错误 = %v", err)
		}
		if _, statErr := os.Lstat(link); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("非法锁未被清理：Lstat err = %v", statErr)
		}
		if len(warnings) == 0 {
			t.Fatalf("按陈旧锁处理时应告警")
		}
	})
}

func TestRemoveSingletonLock(t *testing.T) {
	t.Run("文件不存在时返回 nil", func(t *testing.T) {
		err := removeSingletonLock(t.TempDir())

		if err != nil {
			t.Fatalf("removeSingletonLock() 意外错误 = %v", err)
		}
	})

	t.Run("存在时删除符号链接", func(t *testing.T) {
		dir := t.TempDir()
		link := filepath.Join(dir, singletonLockName)
		if err := os.Symlink("host-1", link); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}

		err := removeSingletonLock(dir)

		if err != nil {
			t.Fatalf("removeSingletonLock() 意外错误 = %v", err)
		}
		if _, statErr := os.Lstat(link); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("锁未被删除：Lstat err = %v", statErr)
		}
	})
}

func TestNormalizeCrashMarker(t *testing.T) {
	const original = `{"profile":{"exit_type":"Crashed","exit_count":3},"keep":[1,2]}`

	t.Run("就地把 Crashed 改写为 Normal 且其余字节不变", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "Default", "Preferences")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}
		if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
			t.Fatalf("Arrange 失败：%v", err)
		}
		var warnings []string
		client := &Client{Warn: func(message string) { warnings = append(warnings, message) }}

		client.normalizeCrashMarker(dir)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取结果失败：%v", err)
		}
		want := `{"profile":{"exit_type":"Normal","exit_count":3},"keep":[1,2]}`
		if string(data) != want {
			t.Fatalf("Preferences = %q，期望 %q", string(data), want)
		}
		if len(warnings) == 0 {
			t.Fatalf("改写崩溃标记时应告警")
		}
	})

	t.Run("文件不存在时不改写也不报错", func(t *testing.T) {
		dir := t.TempDir()
		var warnings []string
		client := &Client{Warn: func(message string) { warnings = append(warnings, message) }}

		client.normalizeCrashMarker(dir)

		if len(warnings) != 0 {
			t.Fatalf("文件不存在不应告警，实际 %v", warnings)
		}
	})
}

func TestActiveBrowserContextBeforeOpen(t *testing.T) {
	client := &Client{}

	browserCtx, err := client.activeBrowserContext()

	if !errors.Is(err, model.ErrDependency) {
		t.Fatalf("activeBrowserContext() error = %v，期望 errors.Is(_, model.ErrDependency)", err)
	}
	if browserCtx != nil {
		t.Fatalf("activeBrowserContext() = %v，期望 nil", browserCtx)
	}
}

func TestStatAndLookPath(t *testing.T) {
	t.Run("Stat 为 nil 时退回 os.Stat", func(t *testing.T) {
		dir := t.TempDir()

		info, err := (&Client{}).stat()(dir)

		if err != nil {
			t.Fatalf("stat()(%q) 意外错误 = %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("stat()(%q) 应为目录", dir)
		}
	})

	t.Run("Stat 注入时使用注入实现", func(t *testing.T) {
		sentinel := errors.New("injected stat failure")
		client := &Client{Stat: func(string) (os.FileInfo, error) { return nil, sentinel }}

		_, err := client.stat()("ignored")

		if !errors.Is(err, sentinel) {
			t.Fatalf("stat() error = %v，期望注入的 %v", err, sentinel)
		}
	})

	t.Run("LookPath 为 nil 时退回 exec.LookPath", func(t *testing.T) {
		path, err := (&Client{}).lookPath()("sh")

		if err != nil {
			t.Fatalf("lookPath()(\"sh\") 意外错误 = %v", err)
		}
		if path == "" {
			t.Fatalf("lookPath()(\"sh\") 返回空路径")
		}
	})

	t.Run("LookPath 注入时使用注入实现", func(t *testing.T) {
		client := &Client{LookPath: func(string) (string, error) { return "/injected/browser", nil }}

		path, err := client.lookPath()("ignored")

		if err != nil || path != "/injected/browser" {
			t.Fatalf("lookPath() = (%q, %v)，期望 (\"/injected/browser\", nil)", path, err)
		}
	})
}

func TestWarnf(t *testing.T) {
	t.Run("Warn 为 nil 时不 panic", func(t *testing.T) {
		(&Client{}).warnf("丢弃 %d 条告警", 1)
	})

	t.Run("Warn 已设置时转发格式化后的消息", func(t *testing.T) {
		var messages []string
		client := &Client{Warn: func(message string) { messages = append(messages, message) }}

		client.warnf("清理 %s 第 %d 次", "锁", 2)

		if len(messages) != 1 || messages[0] != "清理 锁 第 2 次" {
			t.Fatalf("warnf() 转发 = %v", messages)
		}
	})
}

func TestCloseNeverOpenedIsSafeAndIdempotent(t *testing.T) {
	client := &Client{}

	client.Close()
	client.Close()
}

func TestBuildAllocatorOptions(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
	}{
		{name: "默认配置", cfg: config.Default()},
		{name: "无头且开启远程调试", cfg: config.Config{Headless: true, EnableRemoteDebug: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := buildAllocatorOptions(test.cfg, "/Applications/browser", "/tmp/profile")

			wantLen := len(browserFlagSpecs(test.cfg, "/tmp/profile")) + 2
			if len(options) != wantLen {
				t.Fatalf("buildAllocatorOptions() 长度 = %d，期望 %d", len(options), wantLen)
			}
		})
	}
}

func TestMethodsBeforeOpenReturnDependencyError(t *testing.T) {
	client := &Client{cfg: config.Default()}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{name: "Navigate", call: func() error { return client.Navigate(ctx, "https://example.com") }},
		{name: "HTML", call: func() error { _, err := client.HTML(ctx); return err }},
		{name: "ScrollToBottom", call: func() error { return client.ScrollToBottom(ctx) }},
		{name: "WaitDOMStable", call: func() error { return client.WaitDOMStable(ctx) }},
		{name: "WaitNetworkIdle", call: func() error { return client.WaitNetworkIdle(ctx) }},
		{name: "WaitVisible", call: func() error { return client.WaitVisible(ctx, "body") }},
		{name: "LogoutDetected", call: func() error { _, err := client.LogoutDetected(ctx); return err }},
		{name: "EnsureReady", call: func() error { return client.EnsureReady(ctx) }},
		{
			name: "Execute",
			call: func() error {
				return client.Execute(ctx, model.PageAction{Kind: model.ActionNextPage, Selector: "#pageBtn > a:nth-of-type(1)"})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()

			if !errors.Is(err, model.ErrDependency) && !errors.Is(err, model.ErrFetch) {
				t.Fatalf("%s() error = %v，期望包装 model.ErrDependency 或 model.ErrFetch", test.name, err)
			}
		})
	}
}

func TestWaitDOMStableDisabledWhenChecksNonPositive(t *testing.T) {
	client := &Client{cfg: config.Config{DOMStableChecks: 0}}

	err := client.WaitDOMStable(context.Background())

	if err != nil {
		t.Fatalf("WaitDOMStable() 在 DOMStableChecks=0 时应返回 nil，实际 %v", err)
	}
}

func TestWaitVisibleRejectsEmptySelector(t *testing.T) {
	client := &Client{}

	err := client.WaitVisible(context.Background(), "")

	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("WaitVisible(\"\") error = %v，期望包装 model.ErrFetch", err)
	}
	if !strings.Contains(err.Error(), "selector") {
		t.Fatalf("WaitVisible(\"\") 错误信息应说明 selector 为空：%v", err)
	}
}

// testHostname 返回本机主机名；取不到时跳过用例（SingletonLock 判定依赖真实主机名）。
func testHostname(t *testing.T) string {
	t.Helper()

	hostname, err := os.Hostname()
	if err != nil {
		t.Skipf("无法获取主机名：%v", err)
	}
	return hostname
}
