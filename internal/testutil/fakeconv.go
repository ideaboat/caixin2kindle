package testutil

import (
	"context"
	"fmt"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
)

// FakeReporter 记录进度、告警与提示，供日志口径断言（L2：分母必须是去重后篇数）。
type FakeReporter struct {
	ProgressLines []string // 形如「第 3/32 篇：标题」
	Warnings      []string
	Hints         []string
}

// Progress 记录一行进度。
func (r *FakeReporter) Progress(done, total int, title string) {
	r.ProgressLines = append(r.ProgressLines, fmt.Sprintf("第 %d/%d 篇：%s", done, total, title))
}

// Warn 记录一条告警。
func (r *FakeReporter) Warn(msg string) { r.Warnings = append(r.Warnings, msg) }

// Hint 记录一条提示。
func (r *FakeReporter) Hint(msg string) { r.Hints = append(r.Hints, msg) }

// FakeWaiter 记录篇间等待与篇内点击等待次数，不做真实等待。
// 同时满足 app.Waiter 与 article.ClickWaiter。
type FakeWaiter struct {
	ArticleCalls int
	ClickCalls   int
	ArticleErr   error // BetweenArticles 的返回值
	Err          error // BetweenClicks 的返回值
}

// BetweenArticles 记录一次篇间等待。
func (w *FakeWaiter) BetweenArticles(context.Context) error {
	w.ArticleCalls++
	return w.ArticleErr
}

// BetweenClicks 记录一次篇内点击等待。
func (w *FakeWaiter) BetweenClicks(context.Context) error {
	w.ClickCalls++
	return w.Err
}

// FakeOpener 记录浏览器启动调用。
type FakeOpener struct {
	Calls   int
	Err     error
	LastCfg config.Config
}

// Open 记录一次启动。
func (o *FakeOpener) Open(_ context.Context, cfg config.Config) error {
	o.Calls++
	o.LastCfg = cfg
	return o.Err
}

// FakeLogin 记录登录恢复调用。
type FakeLogin struct {
	Calls int
	Err   error
}

// EnsureReady 记录一次登录恢复。
func (l *FakeLogin) EnsureReady(context.Context) error {
	l.Calls++
	return l.Err
}

// FakeEPUBBuilder 记录构建入参并返回可控产物。
type FakeEPUBBuilder struct {
	Name      string
	Data      []byte
	Err       error
	Calls     int
	LastIssue model.Issue
	LastTexts []model.ArticleText
}

// Build 记录调用并返回预设产物。
func (b *FakeEPUBBuilder) Build(issue model.Issue, texts []model.ArticleText) (string, []byte, error) {
	b.Calls++
	b.LastIssue = issue
	b.LastTexts = texts
	if b.Err != nil {
		return "", nil, b.Err
	}
	name := b.Name
	if name == "" {
		name = issue.DirName + ".epub"
	}
	data := b.Data
	if data == nil {
		data = []byte("epub-bytes")
	}
	return name, data, nil
}

// FakeConverter 记录 ebook-convert 调用。
type FakeConverter struct {
	Plans []model.ConvertPlan
	Err   error
	// OnRun 可空：转换成功时调用，用于模拟 calibre 真正写出 MOBI 文件的副作用
	// （否则「构建后重跑应增量跳过」这类用例会因 MOBI 始终不存在而误判）。
	OnRun func(plan model.ConvertPlan)
}

// ToMOBI 记录计划；成功时触发 OnRun，再返回预设错误。
func (c *FakeConverter) ToMOBI(_ context.Context, plan model.ConvertPlan) error {
	c.Plans = append(c.Plans, plan)
	if c.Err != nil {
		return c.Err
	}
	if c.OnRun != nil {
		c.OnRun(plan)
	}
	return nil
}

// FakeVolumeScanner 返回预设候选卷。
type FakeVolumeScanner struct {
	List  []model.Volume
	Err   error
	Calls int
}

// Volumes 返回预设卷列表。
func (s *FakeVolumeScanner) Volumes(context.Context) ([]model.Volume, error) {
	s.Calls++
	if s.Err != nil {
		return nil, s.Err
	}
	return s.List, nil
}

// CopyCall 记录一次设备拷贝。
type CopyCall struct {
	Volume model.Volume
	Src    string
	Name   string
}

// FakeDeviceWriter 记录 documents/ 创建与拷贝。
type FakeDeviceWriter struct {
	Ensured   []model.Volume
	Copies    []CopyCall
	EnsureErr error
	CopyErr   error
}

// EnsureDocuments 记录一次目录创建。
func (d *FakeDeviceWriter) EnsureDocuments(_ context.Context, volume model.Volume) error {
	d.Ensured = append(d.Ensured, volume)
	return d.EnsureErr
}

// Copy 记录一次拷贝。
func (d *FakeDeviceWriter) Copy(_ context.Context, volume model.Volume, src, name string) error {
	d.Copies = append(d.Copies, CopyCall{Volume: volume, Src: src, Name: name})
	return d.CopyErr
}
