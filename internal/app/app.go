// Package app 是应用层：编排用例的顺序与分支，不解析 DOM、不直接碰磁盘（§3 依赖表）。
package app

import (
	"context"
	"fmt"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/port"
	"caixin2kindle/internal/service/state"
)

// BrowserOpener 启动浏览器（有头/无头、持久化 profile、SingletonLock 与指纹抑制）；由 adapter/page 实现。
type BrowserOpener interface {
	Open(ctx context.Context, cfg config.Config) error
}

// LoginHandler 确保登录态就绪（付费墙不可见、无登录页、无验证码）；不含启动；由 adapter/page 实现。
type LoginHandler interface {
	EnsureReady(ctx context.Context) error
}

// IssueLocator 定位期号与去重后的文章列表；由 issue.Locator 实现。
type IssueLocator interface {
	Locate(ctx context.Context, page port.PageSource, url string) (model.Issue, error)
}

// ArticleFetcher 获取单篇正文；由 article.Fetcher 实现。
type ArticleFetcher interface {
	Fetch(ctx context.Context, nav port.Navigator, art model.Article) (model.ArticleText, error)
}

// Waiter 提供篇间随机等待（编排层职责，spec 3.4）；由 adapter/clock 实现。
type Waiter interface {
	BetweenArticles(ctx context.Context) error
}

// StateRepository 读写 state.json，是进度的唯一来源；由 adapter/storage 实现。
type StateRepository interface {
	Load(ctx context.Context, dir string) (model.State, error)
	Save(ctx context.Context, dir string, s model.State) error
}

// ArtifactStore 读写输出目录内的正文与产物；由 adapter/storage 实现。
// 相对路径一律相对输出根目录（--out）解析，state.json 里的 text_path 即用这种相对形式。
type ArtifactStore interface {
	// WriteArticleText 写入一篇正文（自动创建父目录）。
	WriteArticleText(path, body string) error
	// ReadArticleText 读取一篇正文。
	ReadArticleText(path string) (string, error)
	// Exists 报告路径是否存在。
	Exists(path string) (bool, error)
	// WriteEPUB 写入 EPUB，返回其完整路径。
	WriteEPUB(issueDirName string, data []byte) (string, error)
	// Path 拼接输出根目录下的路径。
	Path(elem ...string) string
	// EnsureWorkspace 创建 <out>/<期号>/{,.caixin2kindle/,articles/}（§6.1 步骤 4）。
	EnsureWorkspace(issueDirName string) error
	// CleanWorkspace 删除本工具在该期目录内生成的已知产物，供 --full 使用（§7）。
	// 只删 state.json、articles/*.txt 与 <期号>.epub/.mobi，不做目录级 RemoveAll。
	CleanWorkspace(issueDirName string) error
}

// EPUBBuilder 生成 EPUB 字节（不落盘）；由 ebook.Builder 实现。
type EPUBBuilder interface {
	Build(issue model.Issue, texts []model.ArticleText) (name string, data []byte, err error)
}

// Converter 执行 ebook-convert，并把非零退出归类为 model.ErrConvert；由 adapter/publish 实现。
type Converter interface {
	ToMOBI(ctx context.Context, plan model.ConvertPlan) error
}

// VolumeScanner 只读枚举候选挂载卷；由 adapter/publish 实现。
type VolumeScanner interface {
	Volumes(ctx context.Context) ([]model.Volume, error)
}

// DeviceWriter 在目标卷上按需创建 documents/ 并复制文件；由 adapter/publish 实现。
type DeviceWriter interface {
	EnsureDocuments(ctx context.Context, v model.Volume) error
	Copy(ctx context.Context, v model.Volume, src, name string) error
}

// KindleSelector 是 Kindle 挂载点的纯规则；由 kindle.Selector 实现。
type KindleSelector interface {
	Select(req model.KindleRequest, vols []model.Volume) (model.Volume, bool, error)
}

// Deps 汇聚 App 的全部依赖，由 main.go（唯一组合根）注入。
// 适配器以接口注入；纯计算服务（issue/article/ebook/state/kindle）以具体类型满足接口后注入。
// Opener/Login/Nav 在生产中指向同一个 adapter/page 客户端（它同时实现三者）。
type Deps struct {
	Reporter  Reporter
	Opener    BrowserOpener
	Login     LoginHandler
	Nav       port.Navigator
	Locator   IssueLocator
	Fetcher   ArticleFetcher
	Waiter    Waiter
	States    StateRepository
	Artifacts ArtifactStore
	EPUB      EPUBBuilder
	Converter Converter
	Volumes   VolumeScanner
	Device    DeviceWriter
	Kindle    KindleSelector
}

// App 承载一次运行的依赖，实现 cli.AppRunner。
type App struct {
	deps Deps
}

// New 用给定依赖构造 App。
func New(deps Deps) *App {
	return &App{deps: deps}
}

// Run 执行主流程 §6.1 步骤 2–10：开浏览器 → 定位期号 → 增量判定 → 抓取循环
// → 构建前复核与产物生成 → Kindle 定位与拷贝，返回 model.Result 供 cli 渲染。
func (a *App) Run(ctx context.Context, cfg config.Config) (model.Result, error) {
	issue, issueDir, err := a.openAndLocate(ctx, cfg)
	if err != nil {
		return model.Result{}, err
	}
	current, err := a.loadState(ctx, issue, issueDir, cfg)
	if err != nil {
		return model.Result{}, err
	}
	return a.execute(ctx, cfg, issue, issueDir, current)
}

// openAndLocate 启动浏览器、定位期号并创建工作区（§6.1 步骤 2–4）。
func (a *App) openAndLocate(ctx context.Context, cfg config.Config) (model.Issue, string, error) {
	if err := a.deps.Opener.Open(ctx, cfg); err != nil {
		return model.Issue{}, "", err
	}
	issue, err := a.deps.Locator.Locate(ctx, a.deps.Nav, cfg.URL)
	if err != nil {
		return model.Issue{}, "", err
	}
	issueDir := a.deps.Artifacts.Path(issue.DirName)
	if err := a.deps.Artifacts.EnsureWorkspace(issue.DirName); err != nil {
		return model.Issue{}, "", fmt.Errorf("%w：创建工作区失败：%w", model.ErrUsage, err)
	}
	return issue, issueDir, nil
}

// loadState 读取本次运行的起点状态：--full 先清理已知产物，schema/期号不匹配视为无状态（§4）。
func (a *App) loadState(ctx context.Context, issue model.Issue, issueDir string, cfg config.Config) (model.State, error) {
	if cfg.Full {
		if err := a.deps.Artifacts.CleanWorkspace(issue.DirName); err != nil {
			return model.State{}, fmt.Errorf("%w：--full 清理失败：%w", model.ErrUsage, err)
		}
		return state.NewState(issue), nil
	}

	current, err := a.deps.States.Load(ctx, issueDir)
	if err != nil {
		return model.State{}, fetchError("读取进度", err)
	}
	if current.SchemaVersion != model.SchemaVersion || current.IssueID != issue.ID {
		return state.NewState(issue), nil
	}
	return current, nil
}

// execute 完成增量判定、抓取与发布（§6.1 步骤 5–10）。
func (a *App) execute(ctx context.Context, cfg config.Config, issue model.Issue, issueDir string, current model.State) (model.Result, error) {
	// 先登记「有意跳过」的篇目，增量判定便不会再把它们当待抓项（v1.9.2）。
	current, skippedArticles, skipNote, err := a.markSkipped(ctx, issue, issueDir, current)
	if err != nil {
		return model.Result{}, err
	}

	epubPath, mobiPath := a.artifactPaths(issue)
	needsBuild, err := a.needsBuild(issue, current, epubPath, mobiPath)
	if err != nil {
		return model.Result{}, err
	}

	pending, err := state.Decide(issue, current, a.deps.Artifacts)
	if err != nil {
		return model.Result{}, fetchError("增量判定", err)
	}
	if len(pending) > 0 {
		current, err = a.acquire(ctx, issue, issueDir, current, pending, cfg)
		if err != nil {
			return model.Result{}, err
		}
	}

	skipped := len(pending) == 0 && !needsBuild
	var missing []string
	if !skipped {
		built, err := a.buildArtifacts(ctx, issue, issueDir, current, cfg)
		if err != nil {
			return model.Result{}, err
		}
		epubPath, mobiPath = built.EPUBPath, built.MOBIPath
		missing = articleTitles(built.Missing)
	}

	copied, err := a.deliver(ctx, cfg, mobiPath)
	if err != nil {
		return model.Result{}, err
	}
	return model.Result{
		OutputDir:       issueDir,
		EPUBPath:        epubPath,
		MOBIPath:        mobiPath,
		KindleCopied:    copied,
		Skipped:         skipped,
		MissingArticles: missing,
		SkippedArticles: articleTitles(skippedArticles),
		SkipNote:        skipNote,
	}, nil
}

// needsBuild 判定是否必须重建产物：全部成功 + built_issue_id 一致 + EPUB/MOBI 都在 才可跳过（§7 H6）。
func (a *App) needsBuild(issue model.Issue, current model.State, epubPath, mobiPath string) (bool, error) {
	epubExists, err := a.deps.Artifacts.Exists(epubPath)
	if err != nil {
		return false, fetchError("检查既有 EPUB", err)
	}
	mobiExists, err := a.deps.Artifacts.Exists(mobiPath)
	if err != nil {
		return false, fetchError("检查既有 MOBI", err)
	}
	return state.NeedRebuild(issue, current, epubExists, mobiExists), nil
}

// artifactPaths 返回期目录内的 EPUB 与 MOBI 约定路径。
func (a *App) artifactPaths(issue model.Issue) (string, string) {
	base := a.deps.Artifacts.Path(issue.DirName, issue.DirName)
	return base + ".epub", base + ".mobi"
}

// fetchError 把非哨兵错误统一归类为 model.ErrFetch，并保留底层错误供 errors.Is 判断。
func fetchError(stage string, err error) error {
	return fmt.Errorf("%w：%s失败：%w", model.ErrFetch, stage, err)
}

// stateEntry 在既有状态里按归一化 URL 找到某一篇的进度记录。
func stateEntry(current model.State, article model.Article) (model.ArticleState, bool) {
	for _, entry := range current.Articles {
		if entry.NormalizedURL == article.Normalized {
			return entry, true
		}
	}
	return model.ArticleState{}, false
}

// successCount 统计状态中已成功的篇数，用作进度分子。
func successCount(current model.State) int {
	count := 0
	for _, entry := range current.Articles {
		if entry.Status == model.StatusSuccess {
			count++
		}
	}
	return count
}
