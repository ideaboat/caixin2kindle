package page

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/selector"
)

// 本文件只放纯函数：浏览器三级定位、启动参数、SingletonLock 解析、exit_type 归一化与 JS 片段。
// 把可判定逻辑集中在此，浏览器相关代码（open/navigate/execute/login）保持薄（architecture.md §8）。

// singletonLockName 是 Chrome 单实例锁在 profile 目录下的固定文件名。
const singletonLockName = "SingletonLock"

// defaultProfileMode 是 profile 目录权限：内含登录态，必须 0700（spec 3.1）。
const defaultProfileMode os.FileMode = 0o700

// flagSpec 是一条浏览器命令行参数；name 不含前导 "--"。
type flagSpec struct {
	name  string
	value any
}

// browserFlagSpecs 返回固定启动参数（architecture.md §7「自动化指纹抑制」，v1.8 硬要求）。
// 顺序固定，便于单测逐条断言；`--headless=new` 仅在 cfg.Headless 时出现，
// `--remote-debugging-port=9222` 仅在 cfg.EnableRemoteDebug 时出现。
//
// 这里不返回 `enable-automation`：chromedp 的 bool flag 为 false 时整条省略，
// 而 NewExecAllocator 不注入 DefaultExecAllocatorOptions，故该参数本就不会出现在命令行。
func browserFlagSpecs(cfg config.Config, profileDir string) []flagSpec {
	specs := []flagSpec{
		{name: "user-data-dir", value: profileDir},
		{name: "window-size", value: "1440,1000"},
		{name: "disable-blink-features", value: "AutomationControlled"},
		{name: "test-type", value: true},
		{name: "no-first-run", value: true},
		{name: "no-default-browser-check", value: true},
	}
	if cfg.Headless {
		specs = append(specs, flagSpec{name: "headless", value: "new"})
	}
	if cfg.EnableRemoteDebug {
		specs = append(specs, flagSpec{name: "remote-debugging-port", value: "9222"})
	}
	return specs
}

// renderFlagSpecs 按 chromedp 的规则把 flagSpec 渲染为命令行片段：
// 字符串渲染 `--name=value`，bool true 渲染 `--name`，bool false 整条省略。
func renderFlagSpecs(specs []flagSpec) []string {
	rendered := make([]string, 0, len(specs))
	for _, spec := range specs {
		switch value := spec.value.(type) {
		case string:
			rendered = append(rendered, fmt.Sprintf("--%s=%s", spec.name, value))
		case bool:
			if value {
				rendered = append(rendered, "--"+spec.name)
			}
		default:
			rendered = append(rendered, fmt.Sprintf("--%s=%v", spec.name, value))
		}
	}
	return rendered
}

// resolveBrowserPath 实现浏览器三级定位（M9）：
//  1. 显式 --browser：必须存在且不是目录，否则返回包装 model.ErrDependency 的错误，**不回退**；
//  2. 自动探测 cfg.BrowserCandidates，取第一个存在的非目录；
//  3. PATH 兜底 cfg.BrowserEnvFallbacks，依次 LookPath；
//  4. 全部未命中 → model.ErrDependency。
func resolveBrowserPath(cfg config.Config, stat func(string) (os.FileInfo, error), lookPath func(string) (string, error)) (string, error) {
	if stat == nil {
		stat = os.Stat
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	if cfg.BrowserPath != "" {
		info, err := stat(cfg.BrowserPath)
		if err != nil {
			return "", fmt.Errorf("%w：--browser 指定的浏览器不可用：%s（%v），不回退自动探测", model.ErrDependency, cfg.BrowserPath, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("%w：--browser 指向目录而非可执行文件：%s，不回退自动探测", model.ErrDependency, cfg.BrowserPath)
		}
		return cfg.BrowserPath, nil
	}

	for _, candidate := range cfg.BrowserCandidates {
		info, err := stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		return candidate, nil
	}
	for _, name := range cfg.BrowserEnvFallbacks {
		if path, err := lookPath(name); err == nil && path != "" {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w：未找到可用的 Chromium 系浏览器（应用候选 %d 个、PATH 兜底 %d 个均未命中）",
		model.ErrDependency, len(cfg.BrowserCandidates), len(cfg.BrowserEnvFallbacks))
}

// parseSingletonLockTarget 解析 Chrome SingletonLock 符号链接目标 `<hostname>-<pid>`。
// hostname 自身可能含连字符，故 pid 从最后一个 '-' 之后解析；host 为空、pid ≤ 0 或非数字均视为无法解析。
func parseSingletonLockTarget(target string) (host string, pid int, ok bool) {
	separator := strings.LastIndex(target, "-")
	if separator <= 0 || separator == len(target)-1 {
		return "", 0, false
	}
	host = target[:separator]
	parsedPID, err := strconv.Atoi(target[separator+1:])
	if err != nil || parsedPID <= 0 || host == "" {
		return "", 0, false
	}
	return host, parsedPID, true
}

// singletonLockBusy 判断锁是否表示"本机存在活进程"的占用。
// 主机名未知（hostname == ""）且 pid 存活时保守判为占用，避免误删他人锁。
func singletonLockBusy(target, hostname string, alive func(int) bool) bool {
	host, pid, ok := parseSingletonLockTarget(target)
	if !ok || alive == nil {
		return false
	}
	if !alive(pid) {
		return false
	}
	return hostname == "" || host == hostname
}

// readSingletonLockTarget 读取 profile 下的 SingletonLock 符号链接目标。
// 返回 found=false 表示锁不存在（正常）；读失败（含"存在但不是符号链接"）返回 err 供上层按陈旧锁处理。
func readSingletonLockTarget(profileDir string) (target string, found bool, err error) {
	target, err = os.Readlink(singletonLockPath(profileDir))
	if err == nil {
		return target, true, nil
	}
	if os.IsNotExist(err) {
		return "", false, nil
	}
	return "", false, err
}

// singletonLockPath 返回 profile 下 SingletonLock 的完整路径。
func singletonLockPath(profileDir string) string {
	return filepath.Join(profileDir, singletonLockName)
}

// normalizeExitType 把 Preferences 中上次遗留的 `"exit_type":"Crashed"` 就地改写为 Normal。
// 只替换第一次出现、保持其余字节完全不变（不重排 JSON）；同时容忍 `"exit_type": "Crashed"` 空格变体。
func normalizeExitType(content string) (string, bool) {
	replacements := []struct{ from, to string }{
		{`"exit_type":"Crashed"`, `"exit_type":"Normal"`},
		{`"exit_type": "Crashed"`, `"exit_type": "Normal"`},
	}
	for _, replacement := range replacements {
		if strings.Contains(content, replacement.from) {
			return strings.Replace(content, replacement.from, replacement.to, 1), true
		}
	}
	return content, false
}

// processAlive 判断 pid 是否有活进程（kill -0 语义）。
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// 登录态信号：LogoutDetected 与 EnsureReady 共用同一套取值。
const (
	loginSignalNone    = ""
	loginSignalPaywall = "paywall"
	loginSignalForm    = "login"
	loginSignalCaptcha = "captcha"
)

// domFingerprintJS 是页面就绪主判据的指纹表达式（v1.8）：元素总数 + 正文文本长度。
const domFingerprintJS = `document.querySelectorAll('*').length + ':' + document.body.textContent.length`

// scrollToBottomJS 滚动到底部，用于期号页懒加载。
const scrollToBottomJS = `window.scrollTo(0, document.body.scrollHeight)`

// domClickJS 生成降级用的 DOM 点击片段：元素存在则调用 el.click() 并返回 true，
// 缺失则返回 false（architecture.md §9.1 W9：真实点击失败后才降级）。
func domClickJS(target string) string {
	return "(function(){var el=document.querySelector(" + jsonStringLiteral(target) +
		");if(!el){return false;}el.click();return true;})()"
}

// logoutSignalJS 生成登出探测片段（checks.go 是唯一定义处）：
// 付费墙节点存在且 inline style（去空白、转小写）不含 display:none → "paywall"；
// 否则任一登录表单 selector 命中 → "login"；否则任一验证码 selector 命中 → "captcha"；否则 ""。
func logoutSignalJS(set selector.Set) string {
	var builder strings.Builder
	builder.WriteString("(function(){")
	if set.Login.Paywall != "" {
		builder.WriteString("var paywall=document.querySelector(" + jsonStringLiteral(set.Login.Paywall) + ");")
		builder.WriteString("if(paywall){var style=(paywall.getAttribute('style')||'').replace(/\\s+/g,'').toLowerCase();")
		builder.WriteString("if(style.indexOf('display:none')<0){return 'paywall';}}")
	}
	builder.WriteString("var forms=" + jsonArrayLiteral(set.Login.Form) + ";")
	builder.WriteString("for(var i=0;i<forms.length;i++){if(forms[i]&&document.querySelector(forms[i])){return 'login';}}")
	builder.WriteString("var captchas=" + jsonArrayLiteral(set.Login.Captcha) + ";")
	builder.WriteString("for(var j=0;j<captchas.length;j++){if(captchas[j]&&document.querySelector(captchas[j])){return 'captcha';}}")
	builder.WriteString("return '';})()")
	return builder.String()
}

// jsonStringLiteral 把字符串编码为 JSON 字面量，可安全嵌入 JS 源码。
// 关闭 HTML 转义，避免 selector 里的 `<`/`>` 被改写成 \u003c/\u003e 影响可读性。
func jsonStringLiteral(value string) string {
	encoded, ok := encodeJSONLiteral(value)
	if !ok {
		return `""`
	}
	return encoded
}

// jsonArrayLiteral 把字符串表编码为 JSON 数组字面量；空表编码为 []，避免 JS 侧 null.length 异常。
func jsonArrayLiteral(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	encoded, ok := encodeJSONLiteral(values)
	if !ok {
		return "[]"
	}
	return encoded
}

// encodeJSONLiteral 编码为单行 JSON 文本（去掉 Encoder 追加的换行）。
func encodeJSONLiteral(value any) (string, bool) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", false
	}
	return strings.TrimRight(buffer.String(), "\n"), true
}

// stableChecksAfter 更新"指纹连续不变"的计数：与上次相同则累加，否则重置为 1（首次观察计 1）。
func stableChecksAfter(previous, current string, previousChecks int) int {
	if previousChecks > 0 && previous == current {
		return previousChecks + 1
	}
	return 1
}

// isNetworkIdle 判断网络是否空闲：无在途请求且距最后一次网络事件已超过静默期。
func isNetworkIdle(pending int, lastChange, now time.Time, quiet time.Duration) bool {
	if pending != 0 {
		return false
	}
	return !now.Before(lastChange.Add(quiet))
}
