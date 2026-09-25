package page

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/selector"
)

// 本文件覆盖"不需要真实浏览器"的剩余分支：构造器、注入回退、启动前校验与错误归类。
// 一律不启动任何浏览器进程。

func TestNewClient(t *testing.T) {
	cfg := config.Default()
	set := selector.Default()

	client := NewClient(cfg, set, nil)

	if client == nil {
		t.Fatalf("NewClient() = nil")
	}
	if client.LookPath == nil || client.Stat == nil || client.Warn == nil {
		t.Fatalf("NewClient() 应填好 LookPath/Stat/Warn 的默认实现")
	}
	if client.cfg.BrowserProfileDir != cfg.BrowserProfileDir {
		t.Fatalf("NewClient() 未保存配置：profile = %q", client.cfg.BrowserProfileDir)
	}
	if client.set.Login.Paywall != set.Login.Paywall {
		t.Fatalf("NewClient() 未保存 selector：paywall = %q", client.set.Login.Paywall)
	}
	if client.prompter != nil {
		t.Fatalf("NewClient() prompter = %v，期望 nil", client.prompter)
	}
}

func TestRenderFlagSpecsFallbackFormatting(t *testing.T) {
	specs := []flagSpec{{name: "max-connections", value: 4}}

	got := renderFlagSpecs(specs)

	want := []string{"--max-connections=4"}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("renderFlagSpecs() = %v，期望 %v", got, want)
	}
}

func TestWarnWebdriverWithInvalidContext(t *testing.T) {
	var warnings []string
	client := &Client{Warn: func(message string) { warnings = append(warnings, message) }}

	client.warnWebdriver(context.Background())

	if len(warnings) == 0 {
		t.Fatalf("在非 chromedp 上下文上探测失败应告警")
	}
}

func TestBrowserMethodsWrapChromedpContextError(t *testing.T) {
	// 用普通 context 冒充浏览器上下文：chromedp.Run 会立即返回 ErrInvalidContext，不会启动浏览器。
	client := &Client{browserContext: context.Background(), cfg: config.Default()}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{name: "Navigate", call: func() error { return client.Navigate(ctx, "https://example.com/issue") }},
		{name: "HTML", call: func() error { _, err := client.HTML(ctx); return err }},
		{name: "ScrollToBottom", call: func() error { return client.ScrollToBottom(ctx) }},
		{name: "WaitVisible", call: func() error { return client.WaitVisible(ctx, "body") }},
		{name: "LogoutDetected", call: func() error { _, err := client.LogoutDetected(ctx); return err }},
		{name: "checkLogout", call: func() error { return client.checkLogout(ctx) }},
		{name: "Execute", call: func() error {
			return client.Execute(ctx, model.PageAction{Kind: model.ActionNextPage, Selector: "#pageBtn > a:nth-of-type(1)"})
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()

			if !errors.Is(err, model.ErrFetch) {
				t.Fatalf("%s() error = %v，期望包装 model.ErrFetch", test.name, err)
			}
		})
	}
}

func TestActiveBrowserContextReturnsInjectedContext(t *testing.T) {
	injected := context.Background()
	client := &Client{browserContext: injected}

	got, err := client.activeBrowserContext()

	if err != nil {
		t.Fatalf("activeBrowserContext() 意外错误 = %v", err)
	}
	if got != injected {
		t.Fatalf("activeBrowserContext() = %v，期望注入的 context", got)
	}
}

func TestCloseReleasesContextsAndIsIdempotent(t *testing.T) {
	var browserCancelled, allocCancelled bool
	client := &Client{
		browserContext: context.Background(),
		browserCancel:  func() { browserCancelled = true },
		allocCancel:    func() { allocCancelled = true },
		Warn:           func(string) {},
	}

	client.Close()
	client.Close()

	if !browserCancelled {
		t.Fatalf("Close() 未调用 browserCancel")
	}
	if !allocCancelled {
		t.Fatalf("Close() 未调用 allocCancel")
	}
}

func TestExecuteRejectsEmptySelector(t *testing.T) {
	client := &Client{}

	err := client.Execute(context.Background(), model.PageAction{Kind: model.ActionNextPage})

	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("Execute(空 selector) error = %v，期望包装 model.ErrFetch", err)
	}
	if !strings.Contains(err.Error(), "selector") {
		t.Fatalf("Execute(空 selector) 错误信息应说明缺少 selector：%v", err)
	}
}

func TestExecuteScrollActionPropagatesBrowserError(t *testing.T) {
	client := &Client{}

	err := client.Execute(context.Background(), model.PageAction{Kind: model.ActionScrollToBottom})

	if !errors.Is(err, model.ErrDependency) {
		t.Fatalf("Execute(滚动) error = %v，期望包装 model.ErrDependency", err)
	}
}

func TestOpenRejectsMissingExplicitBrowser(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-browser")
	client := &Client{cfg: config.Config{BrowserPath: missing}}

	err := client.Open(context.Background(), client.cfg)

	if !errors.Is(err, model.ErrDependency) {
		t.Fatalf("Open() error = %v，期望包装 model.ErrDependency", err)
	}
	if !strings.Contains(err.Error(), missing) || !strings.Contains(err.Error(), "不回退") {
		t.Fatalf("Open() 错误信息应含路径与「不回退」：%v", err)
	}
}

func TestOpenFailsWhenProfileDirUnusable(t *testing.T) {
	dir := t.TempDir()
	browser := filepath.Join(dir, "browser")
	if err := os.WriteFile(browser, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("Arrange 失败：%v", err)
	}
	client := &Client{cfg: config.Config{
		BrowserPath:       browser,
		BrowserProfileDir: filepath.Join(browser, "profile"),
	}}

	err := client.Open(context.Background(), client.cfg)

	if !errors.Is(err, model.ErrDependency) {
		t.Fatalf("Open() error = %v，期望包装 model.ErrDependency", err)
	}
}

func TestOpenIsIdempotentWhenAlreadyStarted(t *testing.T) {
	client := &Client{cfg: config.Config{BrowserPath: filepath.Join(t.TempDir(), "missing-browser")}}
	client.browserContext = context.Background()

	err := client.Open(context.Background(), client.cfg)

	if err != nil {
		t.Fatalf("已启动时 Open() = %v，期望 nil", err)
	}
}

func TestRemoveSingletonLockFailure(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("Arrange 失败：%v", err)
	}

	err := removeSingletonLock(file)

	if !errors.Is(err, model.ErrDependency) {
		t.Fatalf("removeSingletonLock() error = %v，期望包装 model.ErrDependency", err)
	}
}

func TestNormalizeCrashMarkerReadFailureWarns(t *testing.T) {
	dir := t.TempDir()
	preferences := filepath.Join(dir, "Default", "Preferences")
	if err := os.MkdirAll(preferences, 0o700); err != nil {
		t.Fatalf("Arrange 失败：%v", err)
	}
	var warnings []string
	client := &Client{Warn: func(message string) { warnings = append(warnings, message) }}

	client.normalizeCrashMarker(dir)

	if len(warnings) == 0 {
		t.Fatalf("读取 Preferences 失败时应告警且不阻断启动")
	}
}
