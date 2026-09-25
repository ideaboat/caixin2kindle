// Package page 是浏览器适配器：用 chromedp 实现 port.PageSource / port.Navigator 与
// app.BrowserOpener / app.LoginHandler。
//
// 自动化指纹抑制、优雅关闭与 DOM 稳定就绪判据都依赖真实浏览器、无法用 fixture 回放覆盖
// （architecture.md §8「adapter/page 的三项不可单测要求」），因此可判定的逻辑全部下沉到
// checks.go 的纯函数，本包其余文件保持薄、只做编排与错误归类。
package page

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/port"
	"caixin2kindle/internal/selector"
)

const (
	// defaultCloseWait 是配置缺失时优雅关闭的兜底上限（默认值 15s，见 config）。
	defaultCloseWait = 15 * time.Second
	// defaultElementTimeout 是配置缺失时元素类等待的兜底上限（默认值 30s）。
	defaultElementTimeout = 30 * time.Second
	// webdriverProbeTimeout 是启动后探测 navigator.webdriver 的单次上限。
	webdriverProbeTimeout = 10 * time.Second
	// preferencesMode 是 Preferences 回写时的创建权限（已存在的文件权限不变）。
	preferencesMode = 0o600
)

// defaultPreferencesRelPath 是崩溃标记所在文件的相对路径。
var defaultPreferencesRelPath = filepath.Join("Default", "Preferences")

// Client 实现 app.BrowserOpener + app.LoginHandler + port.Navigator。
// 生产用法：main 构造一次，Open 之后交给 app 使用，进程退出前调用 Close。
//
// LookPath/Stat/Warn 为注入点，nil 时分别退回 exec.LookPath/os.Stat/空实现。
type Client struct {
	LookPath func(string) (string, error)      // nil → exec.LookPath
	Stat     func(string) (os.FileInfo, error) // nil → os.Stat
	Warn     func(string)                      // 可空：webdriver 未抑制 / 清理陈旧锁 / DOM 稳定超时 / 点击降级 等

	cfg      config.Config
	set      selector.Set
	prompter port.Prompter
	hostname string

	mu             sync.Mutex
	browserContext context.Context
	browserCancel  context.CancelFunc
	allocCancel    context.CancelFunc
}

// NewClient 用配置、selector 表与人工交互桥接构造客户端；不启动浏览器，Open 才启动。
func NewClient(cfg config.Config, set selector.Set, prompter port.Prompter) *Client {
	hostname, err := os.Hostname()
	if err != nil {
		// 取不到主机名时留空：锁判定会保守处理（见 singletonLockBusy）。
		hostname = ""
	}
	return &Client{
		LookPath: exec.LookPath,
		Stat:     os.Stat,
		Warn:     func(string) {},
		cfg:      cfg,
		set:      set,
		prompter: prompter,
		hostname: hostname,
	}
}

// Open 启动浏览器：三级定位可执行文件 → 创建 0700 profile → 处理 SingletonLock 与崩溃标记
// → 以固定参数（指纹抑制）启动 → 探测 navigator.webdriver。
// 幂等：已启动时再次调用直接返回 nil。入参 cfg 与 NewClient 的配置相同，这里以构造时的配置为准。
// 启动失败一律包装 model.ErrDependency（退出码 1）。
func (c *Client) Open(ctx context.Context, cfg config.Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.browserContext != nil {
		return nil
	}

	browserPath, err := resolveBrowserPath(c.cfg, c.stat(), c.lookPath())
	if err != nil {
		return err
	}
	profileDir := c.cfg.BrowserProfileDir
	if err := ensureProfileDir(profileDir); err != nil {
		return err
	}
	if err := c.checkSingletonLock(profileDir); err != nil {
		return err
	}
	c.normalizeCrashMarker(profileDir)

	// 浏览器生命周期由 Close 管理，故 allocator 上下文与调用方 ctx 解耦（也不新建 Background）。
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.WithoutCancel(ctx),
		buildAllocatorOptions(c.cfg, browserPath, profileDir)...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	if err := chromedp.Run(browserCtx); err != nil {
		browserCancel()
		allocCancel()
		return fmt.Errorf("%w：浏览器启动失败：%w", model.ErrDependency, err)
	}

	c.browserContext = browserCtx
	c.browserCancel = browserCancel
	c.allocCancel = allocCancel
	c.warnWebdriver(browserCtx)
	return nil
}

// Close 优雅关闭浏览器并释放资源：发 CDP 关闭命令 → 等 allocator 收尾（上限 BrowserCloseWait）
// → 再取消浏览器与 allocator 上下文。可重复调用；从未 Open 时是空操作。超时或出错只告警。
func (c *Client) Close() {
	c.mu.Lock()
	browserCtx, browserCancel, allocCancel := c.browserContext, c.browserCancel, c.allocCancel
	c.browserContext, c.browserCancel, c.allocCancel = nil, nil, nil
	c.mu.Unlock()

	if browserCtx == nil {
		return
	}
	closeWait := c.cfg.BrowserCloseWait
	if closeWait <= 0 {
		closeWait = defaultCloseWait
	}

	closed := make(chan error, 1)
	go func() { closed <- chromedp.Cancel(browserCtx) }()

	timer := time.NewTimer(closeWait)
	defer timer.Stop()
	select {
	case err := <-closed:
		if err != nil {
			c.warnf("浏览器关闭时报错（已继续释放资源）：%v", err)
		}
	case <-timer.C:
		c.warnf("浏览器优雅关闭超时（上限 %s），强制结束进程", closeWait)
	}

	if browserCancel != nil {
		browserCancel()
	}
	if allocCancel != nil {
		allocCancel()
	}
}

// buildAllocatorOptions 把纯数据 flagSpec 转成 chromedp 选项。
// 额外显式声明 enable-automation=false：chromedp 对 false 的 bool flag 会整条省略，
// 故命令行不会出现该参数（architecture.md §7「自动化指纹抑制」）。
func buildAllocatorOptions(cfg config.Config, browserPath, profileDir string) []chromedp.ExecAllocatorOption {
	specs := browserFlagSpecs(cfg, profileDir)
	options := make([]chromedp.ExecAllocatorOption, 0, len(specs)+2)
	options = append(options, chromedp.ExecPath(browserPath), chromedp.Flag("enable-automation", false))
	for _, spec := range specs {
		options = append(options, chromedp.Flag(spec.name, spec.value))
	}
	return options
}

// warnWebdriver 启动后核对一次 navigator.webdriver；为 true 只告警、不阻断（v1.8）。
func (c *Client) warnWebdriver(browserCtx context.Context) {
	probeCtx, cancel := context.WithTimeout(browserCtx, webdriverProbeTimeout)
	defer cancel()

	var automated bool
	if err := chromedp.Run(probeCtx, chromedp.Evaluate("navigator.webdriver === true", &automated)); err != nil {
		c.warnf("自动化指纹探测失败（不影响运行）：%v", err)
		return
	}
	if automated {
		c.warnf("[WARN] navigator.webdriver 为 true：自动化指纹抑制未生效，站点可能识别出自动化浏览器")
	}
}

// checkSingletonLock 处理 profile 单实例锁：本机活进程占用 → model.ErrDependency；
// 陈旧锁（异机、进程已退出、无法解析）→ 告警后清理，不要求用户手工处理。
func (c *Client) checkSingletonLock(profileDir string) error {
	target, found, err := readSingletonLockTarget(profileDir)
	if err != nil {
		c.warnf("读取 profile 锁失败（按陈旧锁处理）：%v", err)
		return removeSingletonLock(profileDir)
	}
	if !found {
		return nil
	}
	if singletonLockBusy(target, c.hostname, processAlive) {
		return fmt.Errorf("%w：浏览器 profile 被占用（锁目标 %s），请先关闭正在运行的自动化窗口", model.ErrDependency, target)
	}
	c.warnf("发现陈旧 profile 锁（%s），已清理后继续", target)
	return removeSingletonLock(profileDir)
}

// removeSingletonLock 删除陈旧锁文件；删除失败按依赖错误返回，避免带着不可信锁继续启动。
func removeSingletonLock(profileDir string) error {
	err := os.Remove(singletonLockPath(profileDir))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("%w：清理陈旧 profile 锁失败：%w", model.ErrDependency, err)
}

// ensureProfileDir 创建 profile 目录并强制 0700；失败返回 model.ErrDependency。
func ensureProfileDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("%w：浏览器 profile 目录未配置", model.ErrDependency)
	}
	if err := os.MkdirAll(dir, defaultProfileMode); err != nil {
		return fmt.Errorf("%w：创建浏览器 profile 目录失败：%s：%w", model.ErrDependency, dir, err)
	}
	if err := os.Chmod(dir, defaultProfileMode); err != nil {
		return fmt.Errorf("%w：设置浏览器 profile 目录权限失败：%s：%w", model.ErrDependency, dir, err)
	}
	return nil
}

// normalizeCrashMarker 就地归一化上次遗留的崩溃标记，避免本次启动弹出「未正常关闭」对话框。
// 读写失败只告警、不阻断启动（architecture.md §7）。
func (c *Client) normalizeCrashMarker(profileDir string) {
	path := filepath.Join(profileDir, defaultPreferencesRelPath)
	changed, err := normalizeCrashMarkerFile(path)
	if err != nil {
		c.warnf("归一化浏览器崩溃标记失败（不影响启动）：%v", err)
		return
	}
	if changed {
		c.warnf("已把上次的 exit_type=Crashed 就地改写为 Normal，避免弹出「未正常关闭」对话框")
	}
}

// normalizeCrashMarkerFile 读取 Preferences 并做一次字面替换；文件不存在视为无需处理。
func normalizeCrashMarkerFile(path string) (changed bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	normalized, replaced := normalizeExitType(string(data))
	if !replaced {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(normalized), preferencesMode); err != nil {
		return false, err
	}
	return true, nil
}

// activeBrowserContext 返回已启动的浏览器上下文；未 Open 时返回包装 model.ErrDependency 的错误。
func (c *Client) activeBrowserContext() (context.Context, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.browserContext == nil {
		return nil, fmt.Errorf("%w：浏览器尚未启动，请先调用 Open", model.ErrDependency)
	}
	return c.browserContext, nil
}

// warnf 通过注入的 Warn 上报可恢复问题；Warn 为 nil 时安全丢弃（仅可观测性降级，不影响主流程）。
func (c *Client) warnf(format string, args ...any) {
	if c.Warn == nil {
		return
	}
	c.Warn(fmt.Sprintf(format, args...))
}

// stat 返回注入的 Stat，nil 时退回 os.Stat。
func (c *Client) stat() func(string) (os.FileInfo, error) {
	if c.Stat != nil {
		return c.Stat
	}
	return os.Stat
}

// lookPath 返回注入的 LookPath，nil 时退回 exec.LookPath。
func (c *Client) lookPath() func(string) (string, error) {
	if c.LookPath != nil {
		return c.LookPath
	}
	return exec.LookPath
}
