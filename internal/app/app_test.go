package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/selector"
	"caixin2kindle/internal/service/article"
	"caixin2kindle/internal/service/issue"
	"caixin2kindle/internal/service/kindle"
	"caixin2kindle/internal/service/state"
	"caixin2kindle/internal/testutil"
)

const (
	fixturesDir = "../../testdata/fixtures"
	issueURL    = "https://weekly.caixin.com/2026/cw1224/"
)

// harness 汇聚 App 的全部 fake 与真实纯逻辑服务，构成 §8 的跨层集成环境。
type harness struct {
	t           *testing.T
	app         *App
	reporter    *testutil.FakeReporter
	opener      *testutil.FakeOpener
	login       *testutil.FakeLogin
	nav         *testutil.FakeNavigator
	waiter      *testutil.FakeWaiter
	states      *testutil.FakeStateRepository
	artifacts   *testutil.FakeArtifactStore
	epub        *testutil.FakeEPUBBuilder
	epubBuilder EPUBBuilder
	converter   *testutil.FakeConverter
	scanner     *testutil.FakeVolumeScanner
	device      *testutil.FakeDeviceWriter
	cfg         config.Config
}

// mustFixture 读取 testdata/fixtures 下的脱敏快照。
func mustFixture(t *testing.T, name string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(fixturesDir, name))
	if err != nil {
		t.Fatalf("读取 fixture %s 失败：%v", name, err)
	}
	return string(raw)
}

// titledFixture 把 fixture 的标题替换为指定标题，便于在多篇集成用例中按标题定位。
func titledFixture(t *testing.T, name, title string) string {
	t.Helper()

	return strings.ReplaceAll(mustFixture(t, name), "封面报道｜示例报道标题", "封面报道｜"+title)
}

// withoutExpand 去掉并存页面里的「余下全文」按钮，派生出纯「下一页」路线。
func withoutExpand(html string) string {
	replaced := strings.ReplaceAll(html, "<a target=\"_self\" href=\"?p0#page2\">余下全文</a>\n", "")
	replaced = strings.ReplaceAll(replaced, "<a target=\"_self\" href=\"?p0#page3\">余下全文</a>\n", "")
	return strings.ReplaceAll(replaced, "<a target=\"_self\" href=\"?p0#page4\">余下全文</a>\n", "")
}

// newHarness 构造集成环境；调用方随后用 script* 方法安排浏览器脚本。
func newHarness(t *testing.T) *harness {
	t.Helper()

	cfg := config.Default()
	cfg.URL = issueURL
	cfg.OutDir = "/out"
	cfg.DelayMin, cfg.DelayMax = 0, 0
	cfg.ClickDelayMin, cfg.ClickDelayMax = 0, 0
	cfg.RetryBackoff = []time.Duration{0, 0}

	fakeEPUB := &testutil.FakeEPUBBuilder{}
	h := &harness{
		t:           t,
		reporter:    &testutil.FakeReporter{},
		opener:      &testutil.FakeOpener{},
		login:       &testutil.FakeLogin{},
		nav:         &testutil.FakeNavigator{},
		waiter:      &testutil.FakeWaiter{},
		states:      &testutil.FakeStateRepository{},
		artifacts:   &testutil.FakeArtifactStore{Dir: cfg.OutDir},
		epub:        fakeEPUB,
		epubBuilder: fakeEPUB,
		converter:   &testutil.FakeConverter{},
		scanner:     &testutil.FakeVolumeScanner{},
		device:      &testutil.FakeDeviceWriter{},
		cfg:         cfg,
	}
	h.rebuild()
	// 模拟 calibre 成功转换后真正写出 MOBI：否则「构建后重跑」会因 MOBI 始终缺失而重建。
	h.converter.OnRun = func(plan model.ConvertPlan) {
		h.artifacts.Seed(plan.OutputPath, []byte("mobi-bytes"))
	}
	return h
}

// rebuild 按当前字段重新装配 App，便于用例替换某个依赖（如换成真实 EPUB 生成器）。
func (h *harness) rebuild() {
	set := selector.Default()
	h.app = New(Deps{
		Reporter:  h.reporter,
		Opener:    h.opener,
		Login:     h.login,
		Nav:       h.nav,
		Locator:   issue.NewLocator(set),
		Fetcher:   article.NewFetcher(set, h.cfg, h.waiter),
		Waiter:    h.waiter,
		States:    h.states,
		Artifacts: h.artifacts,
		EPUB:      h.epubBuilder,
		Converter: h.converter,
		Volumes:   h.scanner,
		Device:    h.device,
		Kindle:    kindle.NewSelector(),
	})
}

// issue 返回 fixture 解析出的期（24 条合成列表中的 6 篇）。
func (h *harness) issue() model.Issue {
	h.t.Helper()

	parsed, err := issue.ParseIssue(mustFixture(h.t, "issue-settled.html"), selector.Default(), issueURL)
	if err != nil {
		h.t.Fatalf("解析 issue fixture 失败：%v", err)
	}
	return parsed
}

// scriptIssue 安排期号页快照。
func (h *harness) scriptIssue() {
	if h.nav.Pages == nil {
		h.nav.Pages = map[string][]string{}
	}
	h.nav.Pages[issueURL] = []string{mustFixture(h.t, "issue-settled.html")}
}

// scriptHappyArticles 为全部文章安排可成功抓取的脚本：
// 第 1 篇走「余下全文」路线，第 2 篇走纯「下一页」路线，其余为单页完整正文。
func (h *harness) scriptHappyArticles() {
	h.scriptIssue()
	for index, article := range h.issue().Articles {
		title := fmt.Sprintf("示例文章%d", article.Order)
		switch index {
		case 0:
			h.nav.Pages[article.URL] = []string{
				titledFixture(h.t, "article-fulltext-before.html", title),
				titledFixture(h.t, "article-fulltext-after.html", title),
			}
		case 1:
			h.nav.Pages[article.URL] = []string{
				withoutExpand(titledFixture(h.t, "article-paged-01.html", title)),
				withoutExpand(titledFixture(h.t, "article-paged-02.html", title)),
				withoutExpand(titledFixture(h.t, "article-paged-03.html", title)),
				withoutExpand(titledFixture(h.t, "article-paged-04.html", title)),
			}
		default:
			h.nav.Pages[article.URL] = []string{titledFixture(h.t, "article-fulltext-after.html", title)}
		}
	}
}

// scriptAllArticlesAs 让每一篇都回放同一 fixture（用于失败/登录用例）。
func (h *harness) scriptAllArticlesAs(fixture string) {
	h.scriptIssue()
	for _, article := range h.issue().Articles {
		h.nav.Pages[article.URL] = []string{mustFixture(h.t, fixture)}
	}
}

// issueDir 返回输出目录内的期目录。
func (h *harness) issueDir(parsed model.Issue) string {
	return h.artifacts.Path(parsed.DirName)
}

// seedAllSuccess 预置「全部成功 + 产物已构建」的增量现场。
func (h *harness) seedAllSuccess(parsed model.Issue) {
	h.scriptIssue()
	current := state.NewState(parsed)
	for _, article := range parsed.Articles {
		body := fmt.Sprintf("本地正文-%d", article.Order)
		textPath := filepath.Join(parsed.DirName, "articles", fmt.Sprintf("%03d.txt", article.Order))
		h.artifacts.Seed(h.artifacts.Path(textPath), []byte(body))
		current.Articles = append(current.Articles, model.ArticleState{
			Order:         article.Order,
			URL:           article.URL,
			NormalizedURL: article.Normalized,
			Title:         article.Title,
			Author:        "示例记者",
			Status:        model.StatusSuccess,
			TextPath:      textPath,
			Hash:          state.Hash(body),
		})
	}
	current = state.MarkBuilt(current, parsed.ID, parsed.DirName+".epub", parsed.DirName+".mobi")
	dir := h.issueDir(parsed)
	if h.states.States == nil {
		h.states.States = map[string]model.State{}
	}
	h.states.States[dir] = current
	h.artifacts.Seed(filepath.Join(dir, parsed.DirName+".epub"), []byte("旧 EPUB"))
	h.artifacts.Seed(filepath.Join(dir, parsed.DirName+".mobi"), []byte("旧 MOBI"))
}

// kindleVolume 返回一个可直接命中的 Kindle 卷。
func kindleVolume() model.Volume {
	return model.Volume{Path: "/Volumes/Kindle", HasDocuments: true, Writable: true}
}

func TestRunHappyPath(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.scanner.List = []model.Volume{kindleVolume()}

	// Act
	result, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("Run 返回错误：%v", err)
	}
	parsed := h.issue()
	dir := h.issueDir(parsed)
	if result.OutputDir != dir {
		t.Errorf("OutputDir = %q，期望 %q", result.OutputDir, dir)
	}
	if result.EPUBPath != filepath.Join(dir, parsed.DirName+".epub") {
		t.Errorf("EPUBPath = %q", result.EPUBPath)
	}
	if result.MOBIPath != filepath.Join(dir, parsed.DirName+".mobi") {
		t.Errorf("MOBIPath = %q", result.MOBIPath)
	}
	if !result.KindleCopied {
		t.Error("KindleCopied 应为 true")
	}
	if result.Skipped {
		t.Error("首次运行不应标记 Skipped")
	}

	if h.opener.Calls != 1 {
		t.Errorf("浏览器启动次数 = %d，期望 1", h.opener.Calls)
	}
	if h.epub.Calls != 1 {
		t.Errorf("EPUB 构建次数 = %d，期望 1", h.epub.Calls)
	}
	if len(h.converter.Plans) != 1 {
		t.Fatalf("ebook-convert 调用次数 = %d，期望 1", len(h.converter.Plans))
	}
	plan := h.converter.Plans[0]
	if plan.InputPath != result.EPUBPath || plan.OutputPath != result.MOBIPath || plan.OutputProfile != "kindle" {
		t.Errorf("转换计划不正确：%+v", plan)
	}

	// 篇内/篇间等待：6 篇 → 5 次篇间等待
	if h.waiter.ArticleCalls != 5 {
		t.Errorf("篇间等待次数 = %d，期望 5", h.waiter.ArticleCalls)
	}

	// 进度分母必须是去重后的 6 篇
	if len(h.reporter.ProgressLines) != 6 {
		t.Fatalf("进度行数 = %d，期望 6", len(h.reporter.ProgressLines))
	}
	wantFirst := "第 1/6 篇：封面报道｜示例文章1"
	if h.reporter.ProgressLines[0] != wantFirst {
		t.Errorf("首行进度 = %q，期望 %q", h.reporter.ProgressLines[0], wantFirst)
	}
	if h.reporter.ProgressLines[5] != "第 6/6 篇：封面报道｜示例文章6" {
		t.Errorf("末行进度 = %q", h.reporter.ProgressLines[5])
	}

	// EPUB 入参覆盖全部 6 篇且按目录顺序
	if len(h.epub.LastTexts) != 6 {
		t.Fatalf("EPUB 入参篇数 = %d，期望 6", len(h.epub.LastTexts))
	}
	for index, text := range h.epub.LastTexts {
		if text.Order != index+1 {
			t.Errorf("第 %d 个入参 Order = %d", index, text.Order)
		}
		if len(text.Paragraphs) == 0 {
			t.Errorf("第 %d 篇正文为空", index+1)
		}
	}

	// 落盘状态：全部 success、正文路径与哈希齐备、产物已标记
	saved := h.states.States[dir]
	if saved.Artifacts.BuiltIssueID != parsed.ID {
		t.Errorf("BuiltIssueID = %q，期望 %q", saved.Artifacts.BuiltIssueID, parsed.ID)
	}
	if len(saved.Articles) != 6 {
		t.Fatalf("状态文章数 = %d，期望 6", len(saved.Articles))
	}
	for _, entry := range saved.Articles {
		if entry.Status != model.StatusSuccess || entry.TextPath == "" || entry.Hash == "" || entry.Author == "" {
			t.Errorf("状态记录不完整：%+v", entry)
		}
	}

	// Kindle 拷贝
	if len(h.device.Copies) != 1 {
		t.Fatalf("拷贝次数 = %d，期望 1", len(h.device.Copies))
	}
	if h.device.Copies[0].Src != result.MOBIPath || h.device.Copies[0].Name != parsed.DirName+".mobi" {
		t.Errorf("拷贝参数不正确：%+v", h.device.Copies[0])
	}
	if len(h.device.Ensured) != 0 {
		t.Errorf("自动扫描命中时不应创建 documents/，实际 %d 次", len(h.device.Ensured))
	}
}

func TestRunIncrementalSkipsWhenNothingChanged(t *testing.T) {
	// Arrange
	h := newHarness(t)
	parsed := h.issue()
	h.seedAllSuccess(parsed)
	h.scanner.List = []model.Volume{kindleVolume()}

	// Act
	result, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("Run 返回错误：%v", err)
	}
	if !result.Skipped {
		t.Error("全部可跳过时 Skipped 应为 true")
	}
	if h.epub.Calls != 0 || len(h.converter.Plans) != 0 {
		t.Error("增量跳过时不应重建 EPUB/MOBI")
	}
	if !slices.Equal(h.nav.NavigatedURLs, []string{issueURL}) {
		t.Errorf("增量跳过时只应访问期号页，实际 %v", h.nav.NavigatedURLs)
	}
	if len(h.reporter.ProgressLines) != 0 {
		t.Errorf("增量跳过时不应输出抓取进度，实际 %v", h.reporter.ProgressLines)
	}
}

func TestRunRefetchesOnlyUnusableArticle(t *testing.T) {
	// Arrange
	h := newHarness(t)
	parsed := h.issue()
	h.seedAllSuccess(parsed)
	h.scriptHappyArticles()
	h.scanner.List = []model.Volume{kindleVolume()}
	// 破坏第 3 篇的本地正文，触发哈希不符
	target := parsed.Articles[2]
	h.artifacts.Files[h.artifacts.Path(filepath.Join(parsed.DirName, "articles", "003.txt"))] = []byte("被篡改")

	// Act
	result, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("Run 返回错误：%v", err)
	}
	if result.Skipped {
		t.Error("有文章需重抓时不应 Skipped")
	}
	if h.epub.Calls != 1 {
		t.Errorf("重抓后必须重建 EPUB，实际构建 %d 次", h.epub.Calls)
	}
	if len(h.reporter.ProgressLines) != 1 {
		t.Fatalf("只应重抓 1 篇，实际进度 %v", h.reporter.ProgressLines)
	}
	visited := slices.Contains(h.nav.NavigatedURLs, target.URL)
	if !visited {
		t.Errorf("应重抓第 3 篇，实际访问 %v", h.nav.NavigatedURLs)
	}
}

func TestRunConversionFailureDoesNotMarkBuilt(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.converter.Err = model.ErrConvert

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrConvert) {
		t.Fatalf("错误应包装 model.ErrConvert，实际：%v", err)
	}
	for _, saved := range h.states.Saves {
		if saved.Artifacts.BuiltIssueID != "" {
			t.Errorf("转换失败时不得标记已构建：%+v", saved.Artifacts)
		}
	}
}

func TestRunNoKindleStillRebuildsWhenMobiMissing(t *testing.T) {
	// Arrange
	h := newHarness(t)
	parsed := h.issue()
	h.seedAllSuccess(parsed)
	h.cfg.NoKindle = true
	// 删掉 MOBI：--no-kindle 不参与重建判定，必须重建
	delete(h.artifacts.Files, filepath.Join(h.issueDir(parsed), parsed.DirName+".mobi"))

	// Act
	result, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("Run 返回错误：%v", err)
	}
	if h.epub.Calls != 1 || len(h.converter.Plans) != 1 {
		t.Error("--no-kindle 且 MOBI 缺失时必须重建 EPUB + MOBI")
	}
	if h.scanner.Calls != 0 || len(h.device.Copies) != 0 {
		t.Error("--no-kindle 时不得扫描或拷贝设备")
	}
	if result.KindleCopied {
		t.Error("--no-kindle 时 KindleCopied 应为 false")
	}
}

func TestRunKindleNotDetectedIsSuccess(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.scanner.List = []model.Volume{{Path: "/Volumes/USB", HasDocuments: false, Writable: true}}

	// Act
	result, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("未检测到设备属正常路径，不应报错：%v", err)
	}
	if result.KindleCopied {
		t.Error("未检测到设备时 KindleCopied 应为 false")
	}
	if len(h.reporter.Hints) == 0 {
		t.Error("应提示成品路径供用户自行拷入")
	}
}

func TestRunExplicitKindleMissingFails(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.cfg.KindleMount = "/Volumes/Nope"
	h.cfg.KindleMountExplicit = true
	h.scanner.List = []model.Volume{kindleVolume()}

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrCopy) {
		t.Fatalf("显式挂载点不存在应包装 model.ErrCopy，实际：%v", err)
	}
}

func TestRunExplicitKindleCreatesDocuments(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.cfg.KindleMount = "/Volumes/MyKindle"
	h.cfg.KindleMountExplicit = true
	h.scanner.List = []model.Volume{{Path: "/Volumes/MyKindle", HasDocuments: false, Writable: true}}

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("Run 返回错误：%v", err)
	}
	if len(h.device.Ensured) != 1 {
		t.Fatalf("显式挂载点应创建 documents/，实际 %d 次", len(h.device.Ensured))
	}
	if len(h.device.Copies) != 1 {
		t.Errorf("应完成拷贝，实际 %d 次", len(h.device.Copies))
	}
}

func TestRunCopyFailurePropagates(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.scanner.List = []model.Volume{kindleVolume()}
	h.device.CopyErr = model.ErrCopy

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrCopy) {
		t.Fatalf("错误应包装 model.ErrCopy，实际：%v", err)
	}
}

func TestRunConsecutiveFailuresStop(t *testing.T) {
	// Arrange：正文始终不完整 → 每篇重试 3 次后失败，连续 3 篇即熔断
	h := newHarness(t)
	h.scriptAllArticlesAs("article-incomplete.html")

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("错误应包装 model.ErrFetch，实际：%v", err)
	}
	if len(h.reporter.ProgressLines) != 0 {
		t.Errorf("全部失败时不应有成功进度，实际 %v", h.reporter.ProgressLines)
	}
	failedEntries := 0
	for _, entry := range h.states.States[h.issueDir(h.issue())].Articles {
		if entry.Status == model.StatusFail {
			failedEntries++
		}
	}
	if failedEntries != 3 {
		t.Errorf("应有 3 篇记为 fail，实际 %d", failedEntries)
	}
	if len(h.reporter.Hints) == 0 {
		t.Error("熔断时应给出排查提示")
	}
}

func TestRunLoginRecoveryExhausted(t *testing.T) {
	// Arrange：付费墙常驻 → ErrLogin，恢复次数用尽后退出 2
	h := newHarness(t)
	h.scriptAllArticlesAs("article-paywall.html")
	h.cfg.LoginRecoveryLimit = 2

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrLogin) {
		t.Fatalf("错误应包装 model.ErrLogin，实际：%v", err)
	}
	if h.login.Calls != 2 {
		t.Errorf("登录恢复次数 = %d，期望上限 2", h.login.Calls)
	}
}

// TestRunBuildsPartialWhenOneArticleUnavailable 覆盖降级策略：
// 个别篇目重试仍失败时，用其余成功篇目完成构建，把缺失篇目录入警告与结果，不再整期白跑。
func TestRunBuildsPartialWhenOneArticleUnavailable(t *testing.T) {
	// Arrange
	h := newHarness(t)
	parsed := h.issue()
	h.scriptHappyArticles()
	unavailable := parsed.Articles[2]
	h.nav.Pages[unavailable.URL] = []string{mustFixture(t, "article-incomplete.html")}
	h.scanner.List = []model.Volume{kindleVolume()}

	// Act
	result, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("部分失败时仍应完成构建，实际：%v", err)
	}
	if h.epub.Calls != 1 {
		t.Fatalf("应构建 EPUB，实际 %d 次", h.epub.Calls)
	}
	if len(h.epub.LastTexts) != len(parsed.Articles)-1 {
		t.Errorf("EPUB 入参篇数 = %d，期望 %d", len(h.epub.LastTexts), len(parsed.Articles)-1)
	}
	wantMissing := fmt.Sprintf("第 %d 篇 %s", unavailable.Order, unavailable.Title)
	if !slices.Contains(result.MissingArticles, wantMissing) {
		t.Errorf("MissingArticles = %v，应包含 %q", result.MissingArticles, wantMissing)
	}
	if len(h.reporter.Warnings) == 0 {
		t.Error("缺失篇目应至少产生一条警告")
	}
	if h.states.States[h.issueDir(parsed)].Artifacts.BuiltIssueID != parsed.ID {
		t.Error("降级构建后仍应标记产物已构建")
	}
}

// TestRunSkipsImageColumnArticle 覆盖「图片型栏目放弃抓取」：
// 标题以「显影」开头的篇目记为 skipped，不访问其页面，在结果与提示里说明，且不算缺失。
func TestRunSkipsImageColumnArticle(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	issueHTML := strings.Replace(mustFixture(t, "issue-settled.html"), "示例标题丙", "显影｜示例标题丙", 1)
	h.nav.Pages[issueURL] = []string{issueHTML}
	h.scanner.List = []model.Volume{kindleVolume()}
	imageColumnURL := "https://weekly.caixin.com/2026-09-19/900000003.html"

	// Act
	result, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("Run 返回错误：%v", err)
	}
	if slices.Contains(h.nav.NavigatedURLs, imageColumnURL) {
		t.Errorf("图片型栏目不应被抓取，实际访问了 %s", imageColumnURL)
	}
	if h.epub.Calls != 1 {
		t.Fatalf("应构建 EPUB，实际 %d 次", h.epub.Calls)
	}
	if len(h.epub.LastTexts) != 5 {
		t.Errorf("EPUB 入参篇数 = %d，期望 5（24 篇中跳过 1 篇）", len(h.epub.LastTexts))
	}
	wantSkipped := "第 3 篇 显影｜示例标题丙"
	if !slices.Contains(result.SkippedArticles, wantSkipped) {
		t.Errorf("SkippedArticles = %v，应包含 %q", result.SkippedArticles, wantSkipped)
	}
	if result.SkipNote == "" {
		t.Error("应给出跳过说明")
	}
	if len(result.MissingArticles) != 0 {
		t.Errorf("有意跳过不得计入缺失，实际 %v", result.MissingArticles)
	}
	if !slices.Contains(h.nav.NavigatedURLs, issueURL) {
		t.Error("仍应访问期号页")
	}

	saved := h.states.States[h.issueDir(h.issue())]
	var status model.Status
	for _, entry := range saved.Articles {
		if entry.NormalizedURL == imageColumnURL {
			status = entry.Status
		}
	}
	if status != model.StatusSkipped {
		t.Errorf("状态记录 = %q，期望 %q", status, model.StatusSkipped)
	}
	if len(h.reporter.Hints) == 0 {
		t.Error("应输出跳过说明（Reporter.Hint）")
	}

	// 再跑一次：跳过项不得触发重建（增量生效）
	second, err := h.app.Run(context.Background(), h.cfg)
	if err != nil {
		t.Fatalf("第二次 Run 返回错误：%v", err)
	}
	if !second.Skipped {
		t.Error("第二次运行应增量跳过")
	}
	if h.epub.Calls != 1 {
		t.Errorf("第二次运行不应重建 EPUB，实际构建 %d 次", h.epub.Calls)
	}
}

func TestRunFullResetsAndRefetches(t *testing.T) {
	// Arrange
	h := newHarness(t)
	parsed := h.issue()
	h.seedAllSuccess(parsed)
	h.scriptHappyArticles()
	h.cfg.Full = true
	h.scanner.List = []model.Volume{kindleVolume()}

	// Act
	result, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("Run 返回错误：%v", err)
	}
	if !slices.Contains(h.artifacts.Cleans, parsed.DirName) {
		t.Errorf("--full 应清理本工具产物，实际 %v", h.artifacts.Cleans)
	}
	if result.Skipped {
		t.Error("--full 不应跳过")
	}
	if len(h.reporter.ProgressLines) != 6 {
		t.Errorf("--full 应重抓全部 6 篇，实际 %v", h.reporter.ProgressLines)
	}
}
