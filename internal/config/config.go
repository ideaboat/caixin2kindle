// Package config 集中全部可调量：默认值、CLI 覆盖与校验。
// 配置不加配置文件、不加环境变量（spec 1.3），由 CLI 参数注入（architecture.md §9.1 W1/W2）。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"caixin2kindle/internal/model"
)

// Config 是一次运行的全部可调量。默认值见 Default，字段与 architecture.md §7 默认值表一一对应。
type Config struct {
	// 抓取节奏（spec 3.4 拟人化）
	DelayMin      time.Duration // 篇间等待下限，默认 3s（--delay "M-N"）
	DelayMax      time.Duration // 篇间等待上限，默认 8s
	ClickDelayMin time.Duration // 篇内点击间等待下限，默认 1s
	ClickDelayMax time.Duration // 篇内点击间等待上限，默认 3s

	// 机器等待超时；人工确认是唯一不设超时的例外（§9.2）
	LoginPollTimeout      time.Duration // 登录轮询等待，默认 300s
	ElementVisibleTimeout time.Duration // 单次元素可见等待，默认 30s
	NetworkIdleTimeout    time.Duration // 单次网络空闲等待，默认 60s
	NavigateTimeout       time.Duration // 单次页面导航，默认 90s
	DOMStableTimeout      time.Duration // DOM 稳定判据上限，默认 25s
	DOMStableChecks       int           // DOM 指纹连续不变次数，默认 3
	DOMStablePollInterval time.Duration // DOM 指纹轮询间隔，默认 500ms
	BrowserCloseWait      time.Duration // 浏览器优雅关闭等待上限，默认 15s

	// 上限与重试
	ArticleClickLimit       int             // 单篇 Expand+NextPage 合计上限，默认 50（spec 3.7-5）
	ArticleTotalSteps       int             // 单篇动作总步数兜底闸，默认 200
	ArticleRetryLimit       int             // 单篇最多尝试次数，默认 3（spec 4.3）
	RetryBackoff            []time.Duration // 固定退避，默认 2s、4s
	ConsecutiveFailureLimit int             // 连败熔断阈值，默认 3
	BuildRetryRounds        int             // 构建前回抓次数上限，默认 2
	LoginRecoveryLimit      int             // “恢复→再失效”自动循环上限，默认 3

	// 外部世界
	OutputProfile       string   // ebook-convert --output-profile，默认 "kindle"
	BrowserCandidates   []string // 浏览器自动探测顺序
	BrowserEnvFallbacks []string // 浏览器 PATH 兜底顺序
	BrowserPath         string   // --browser 显式指定；非空时不存在即报错、不回退
	BrowserProfileDir   string   // 持久化 profile，默认 ~/.caixin/browser-profile（0700）
	KindleMount         string   // Kindle 挂载点，默认 /Volumes/Kindle
	KindleMountExplicit bool     // --kindle 是否显式给出
	DocumentsDirName    string   // 默认 "documents"，仅在显式指定的卷上创建
	EnableRemoteDebug   bool     // chromedp 调试端口，默认 false

	// 输出与开关
	URL      string // 期号页地址（唯一位置参数，spec 2）；是否给出由 cli 校验
	OutDir   string // 父目录，默认 ~/Downloads/caixin；Load 时归一化
	NoKindle bool   // --no-kindle：只跳过拷贝，仍产出 EPUB + MOBI
	Full     bool   // --full：忽略既有状态全量重跑
	Headless bool   // --headless：无头运行（登录/验证码下直接失败退出 2）
}

// Overrides 是 CLI 参数覆盖项；字段为 nil 表示该参数未给出、保持默认值。
type Overrides struct {
	URL      *string // 位置参数：期号页地址
	Out      *string // --out
	Kindle   *string // --kindle
	Browser  *string // --browser
	Delay    *string // --delay "M-N"
	NoKindle *bool   // --no-kindle
	Full     *bool   // --full
	Headless *bool   // --headless
}

// Env 汇聚 Load 归一化路径时所需的进程环境，便于测试注入确定值。
type Env struct {
	HomeDir string // 用于展开开头的 ~
	WorkDir string // 用于解析相对路径
}

// Default 返回全部可调量的默认值（architecture.md §7 默认值表）。
func Default() Config {
	return Config{
		DelayMin:      3 * time.Second,
		DelayMax:      8 * time.Second,
		ClickDelayMin: 1 * time.Second,
		ClickDelayMax: 3 * time.Second,

		LoginPollTimeout:      300 * time.Second,
		ElementVisibleTimeout: 30 * time.Second,
		NetworkIdleTimeout:    60 * time.Second,
		NavigateTimeout:       90 * time.Second,
		DOMStableTimeout:      25 * time.Second,
		DOMStableChecks:       3,
		DOMStablePollInterval: 500 * time.Millisecond,
		BrowserCloseWait:      15 * time.Second,

		ArticleClickLimit:       50,
		ArticleTotalSteps:       200,
		ArticleRetryLimit:       3,
		RetryBackoff:            []time.Duration{2 * time.Second, 4 * time.Second},
		ConsecutiveFailureLimit: 3,
		BuildRetryRounds:        2,
		LoginRecoveryLimit:      3,

		OutputProfile: "kindle",
		BrowserCandidates: []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		},
		BrowserEnvFallbacks: []string{"google-chrome", "chromium", "brave-browser"},
		BrowserProfileDir:   "~/.caixin/browser-profile",
		KindleMount:         "/Volumes/Kindle",
		DocumentsDirName:    "documents",
		EnableRemoteDebug:   false,

		OutDir: "~/Downloads/caixin",
	}
}

// Load 以 Default 为基底应用 overrides，并按 env 归一化路径，返回最终配置。
// 覆盖值语法非法（如 --delay）时返回包装 model.ErrUsage 的错误。
func Load(overrides Overrides, env Env) (Config, error) {
	cfg := Default()

	if overrides.URL != nil {
		cfg.URL = *overrides.URL
	}
	if overrides.Out != nil {
		cfg.OutDir = *overrides.Out
	}
	if overrides.Kindle != nil {
		cfg.KindleMount = *overrides.Kindle
		cfg.KindleMountExplicit = true
	}
	if overrides.Browser != nil {
		cfg.BrowserPath = *overrides.Browser
	}
	if overrides.NoKindle != nil {
		cfg.NoKindle = *overrides.NoKindle
	}
	if overrides.Full != nil {
		cfg.Full = *overrides.Full
	}
	if overrides.Headless != nil {
		cfg.Headless = *overrides.Headless
	}
	if overrides.Delay != nil {
		minimum, maximum, err := ParseDelay(*overrides.Delay)
		if err != nil {
			return Config{}, err
		}
		cfg.DelayMin, cfg.DelayMax = minimum, maximum
	}

	cfg.OutDir = ResolvePath(cfg.OutDir, env)
	cfg.BrowserProfileDir = ResolvePath(cfg.BrowserProfileDir, env)
	return cfg, nil
}

// ParseDelay 解析 "M-N" 形式的延迟区间，返回下限与上限。
// 规则（spec 2、architecture.md M4）：M、N 为非负整数且 M ≤ N；单值 "5" 等价 "5-5"；
// "8-3"、负数、非整数、缺上界等一律返回包装 model.ErrUsage 的错误。
func ParseDelay(spec string) (time.Duration, time.Duration, error) {
	parts := strings.Split(spec, "-")
	switch len(parts) {
	case 1:
		seconds, err := parseSeconds(parts[0])
		if err != nil {
			return 0, 0, err
		}
		return seconds, seconds, nil
	case 2:
		minimum, err := parseSeconds(parts[0])
		if err != nil {
			return 0, 0, err
		}
		maximum, err := parseSeconds(parts[1])
		if err != nil {
			return 0, 0, err
		}
		if minimum > maximum {
			return 0, 0, fmt.Errorf("%w：--delay 区间下界不得大于上界：%q", model.ErrUsage, spec)
		}
		return minimum, maximum, nil
	default:
		return 0, 0, fmt.Errorf("%w：--delay 需为 \"M-N\" 或单值 \"N\"：%q", model.ErrUsage, spec)
	}
}

// parseSeconds 把一段非负整数字符串解析为秒级时长。
func parseSeconds(field string) (time.Duration, error) {
	trimmed := strings.TrimSpace(field)
	seconds, err := strconv.Atoi(trimmed)
	if err != nil || seconds < 0 {
		return 0, fmt.Errorf("%w：--delay 只能是非负整数秒：%q", model.ErrUsage, field)
	}
	return time.Duration(seconds) * time.Second, nil
}

// ResolvePath 归一化路径：展开开头的 ~，相对路径按 env.WorkDir 解析，最后 filepath.Clean（L3）。
// env 中缺失的部分退回进程当前环境，便于生产调用；测试应显式传入确定值。
func ResolvePath(path string, env Env) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home := env.HomeDir
		if home == "" {
			// 故意忽略错误：取不到 home 时下面的 home != "" 判断会跳过展开，
			// 路径退化为原样返回，由 Validate 或后续 IO 明确报错，不在此处中断启动。
			home, _ = os.UserHomeDir()
		}
		if home != "" {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	if !filepath.IsAbs(path) {
		workDir := env.WorkDir
		if workDir == "" {
			// 故意忽略错误：同上，取不到工作目录时保持相对路径，交由 filepath.Join/Clean 处理。
			workDir, _ = os.Getwd()
		}
		path = filepath.Join(workDir, path)
	}
	return filepath.Clean(path)
}

// Validate 校验配置的跨字段一致性；非法配置返回包装 model.ErrUsage 的错误。
func (c Config) Validate() error {
	checks := []struct {
		ok   bool
		desc string
	}{
		{c.DelayMin >= 0 && c.DelayMax >= c.DelayMin, "DelayMin/DelayMax"},
		{c.ClickDelayMin >= 0 && c.ClickDelayMax >= c.ClickDelayMin, "ClickDelayMin/ClickDelayMax"},
		{c.LoginPollTimeout > 0, "LoginPollTimeout"},
		{c.ElementVisibleTimeout > 0, "ElementVisibleTimeout"},
		{c.NetworkIdleTimeout > 0, "NetworkIdleTimeout"},
		{c.NavigateTimeout > 0, "NavigateTimeout"},
		{c.DOMStableTimeout > 0, "DOMStableTimeout"},
		{c.DOMStableChecks > 0, "DOMStableChecks"},
		{c.DOMStablePollInterval > 0, "DOMStablePollInterval"},
		{c.BrowserCloseWait > 0, "BrowserCloseWait"},
		{c.ArticleClickLimit > 0, "ArticleClickLimit"},
		{c.ArticleTotalSteps > 0, "ArticleTotalSteps"},
		{c.ArticleRetryLimit > 0, "ArticleRetryLimit"},
		{len(c.RetryBackoff) >= c.ArticleRetryLimit-1, "RetryBackoff"},
		{c.ConsecutiveFailureLimit > 0, "ConsecutiveFailureLimit"},
		{c.BuildRetryRounds >= 0, "BuildRetryRounds"},
		{c.LoginRecoveryLimit > 0, "LoginRecoveryLimit"},
		{c.OutputProfile != "", "OutputProfile"},
		{c.DocumentsDirName != "", "DocumentsDirName"},
		{c.OutDir != "", "OutDir"},
		{c.BrowserProfileDir != "", "BrowserProfileDir"},
		{!c.KindleMountExplicit || c.KindleMount != "", "KindleMount"},
	}
	for _, check := range checks {
		if !check.ok {
			return fmt.Errorf("%w：配置项非法或缺失：%s", model.ErrUsage, check.desc)
		}
	}
	if len(c.BrowserCandidates) == 0 && len(c.BrowserEnvFallbacks) == 0 && c.BrowserPath == "" {
		return fmt.Errorf("%w：浏览器候选与兜底列表均为空，且未指定 --browser", model.ErrUsage)
	}
	return nil
}
