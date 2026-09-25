// Package publish 是外部发布能力适配器：依赖探测（probe）、ebook-convert 执行（calibre）
// 与 Kindle USB 卷扫描/复制（kindle_usb）。本包只依赖 model/config 与标准库，
// 不反向导入 app/service/cli（architecture.md §3 依赖表）。
package publish

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
)

// calibreBundlePath 是 macOS 上 calibre 应用包内的 ebook-convert 路径（C3 回退项）：
// 通过 app 安装 calibre 时该命令通常不在 PATH 中。
const calibreBundlePath = "/Applications/calibre.app/Contents/MacOS/ebook-convert"

// ebookConvertExecutable 是 PATH 中优先探测的 ebook-convert 命令名。
const ebookConvertExecutable = "ebook-convert"

// 依赖名与提示文案（spec 3.1、architecture.md §7 M9）。
const (
	browserExplicitName = "浏览器"
	browserName         = "Chromium 系浏览器"
	ebookConvertName    = "ebook-convert"
	browserHint         = "未检测到 Chromium 系浏览器；请安装 Google Chrome / Chromium / Brave / Edge，或用 --browser 指定可执行文件路径"
	ebookConvertHint    = "未找到 ebook-convert；请安装 calibre（https://calibre-ebook.com/download）后重试，或把 calibre 加入 PATH"
)

// Probe 实现 cli.DependencyChecker：在启动时一次性探测 Chromium 系浏览器与 ebook-convert。
// 两个注入点默认走真实机器，测试可替换为 fake，从而完全不接触本机环境。
type Probe struct {
	LookPath func(string) (string, error)      // nil → exec.LookPath
	Stat     func(string) (os.FileInfo, error) // nil → os.Stat

	browserPath       string   // cfg.BrowserPath：显式 --browser，非空时不回退自动探测
	browserCandidates []string // cfg.BrowserCandidates：自动探测顺序
	browserFallbacks  []string // cfg.BrowserEnvFallbacks：PATH 兜底顺序
	ebookCandidates   []string // ebook-convert 的绝对路径候选（config 暂无该字段，硬编码 C3 回退项）
}

// NewProbe 用配置构造探测器（config 已在 CLI 解析后可用，故此处即可捕获全部候选路径）。
// 浏览器三级定位的三个来源取自 cfg；ebook-convert 回退项来自本包常量（C3）。
func NewProbe(cfg config.Config) *Probe {
	return &Probe{
		browserPath:       cfg.BrowserPath,
		browserCandidates: append([]string(nil), cfg.BrowserCandidates...),
		browserFallbacks:  append([]string(nil), cfg.BrowserEnvFallbacks...),
		ebookCandidates:   []string{calibreBundlePath},
	}
}

// Check 依次探测浏览器与 ebook-convert，并按 spec 3.1 的顺序返回**全部**缺失项
// （浏览器在前、ebook-convert 在后）。返回值恒为非 nil 切片：全部可用时长度为 0。
//
// ctx 已被取消时立即返回空切片——调用方已放弃本次运行，依赖提示不再有意义；
// 本方法只做本地 Stat/LookPath，不存在阻塞 IO，正常情况下观察不到取消。
func (p *Probe) Check(ctx context.Context) []model.MissingDep {
	missing := make([]model.MissingDep, 0, 2)
	if ctx != nil && ctx.Err() != nil {
		return missing
	}
	if !p.browserAvailable() {
		missing = append(missing, p.browserMissingDep())
	}
	if !p.ebookConvertAvailable() {
		missing = append(missing, model.MissingDep{Name: ebookConvertName, Hint: ebookConvertHint})
	}
	return missing
}

// browserAvailable 按 M9 三级定位判定浏览器是否可用：
// ① 显式 --browser：存在且非目录即通过，**不回退**；
// ② 自动探测：第一个 Stat 成功且非目录的候选；
// ③ PATH 兜底：第一个 LookPath 成功的命令名。
func (p *Probe) browserAvailable() bool {
	if p.browserPath != "" {
		info, err := p.stat()(p.browserPath)
		return err == nil && !info.IsDir()
	}
	for _, candidate := range p.browserCandidates {
		if info, err := p.stat()(candidate); err == nil && !info.IsDir() {
			return true
		}
	}
	for _, name := range p.browserFallbacks {
		if _, err := p.lookPath()(name); err == nil {
			return true
		}
	}
	return false
}

// browserMissingDep 生成浏览器缺失项：显式指定时给出“不回退”的具体路径提示，自动探测时给安装指引。
func (p *Probe) browserMissingDep() model.MissingDep {
	if p.browserPath != "" {
		return model.MissingDep{
			Name: browserExplicitName,
			Hint: fmt.Sprintf("--browser 指定的路径不存在或不是可执行文件：%s（按约定不回退到自动探测）", p.browserPath),
		}
	}
	return model.MissingDep{Name: browserName, Hint: browserHint}
}

// ebookConvertAvailable 按 C3 判定 ebook-convert 是否可用：PATH 优先，其次 macOS 应用包内绝对路径。
func (p *Probe) ebookConvertAvailable() bool {
	if _, err := p.lookPath()(ebookConvertExecutable); err == nil {
		return true
	}
	for _, candidate := range p.ebookCandidates {
		if info, err := p.stat()(candidate); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// stat 返回生效的 Stat 实现：未注入时使用 os.Stat。
func (p *Probe) stat() func(string) (os.FileInfo, error) {
	if p.Stat != nil {
		return p.Stat
	}
	return os.Stat
}

// lookPath 返回生效的 LookPath 实现：未注入时使用 exec.LookPath。
func (p *Probe) lookPath() func(string) (string, error) {
	if p.LookPath != nil {
		return p.LookPath
	}
	return exec.LookPath
}
