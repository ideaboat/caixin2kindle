// Package article 负责单篇正文的获取，拆为两半：
//   - 纯解析（extract / accumulate / Plan / verify）：输入为页面 HTML，可在单测中按 fixture 复现（D1/D4）；
//   - 浏览器驱动的状态机（Fetcher.Fetch）：只做编排与步进，不解析 DOM（§6.2）。
package article

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/port"
	"caixin2kindle/internal/selector"
)

// ClickWaiter 提供篇内两次点击之间的随机等待（spec 3.4）；由 adapter/clock 实现。
// 接口定义在消费方包内（§5 判定规则）。
type ClickWaiter interface {
	// BetweenClicks 在两次点击之间等待 1–3 秒随机时长。
	BetweenClicks(ctx context.Context) error
}

// Fetcher 实现 app.ArticleFetcher：把一篇文章的所有分页/展开步骤走完，返回拼接后的正文。
type Fetcher struct {
	Set    selector.Set  // selector 表
	Cfg    config.Config // 点击上限、步数上限与各类等待超时
	Waiter ClickWaiter   // 篇内点击间等待
}

// NewFetcher 构造单篇获取器。
func NewFetcher(set selector.Set, cfg config.Config, waiter ClickWaiter) *Fetcher {
	return &Fetcher{Set: set, Cfg: cfg, Waiter: waiter}
}

// Fetch 按 §6.2 的状态机获取一篇正文：
// 导航 → 循环（完成判定 / Plan 取一个动作 / 执行 / 归档）→ 收尾归档 → 最终强制成功判定。
// 任一判据不通过返回包装 model.ErrFetch 的错误；登录失效返回 model.ErrLogin 供 app 走恢复流程。
func (f *Fetcher) Fetch(ctx context.Context, nav port.Navigator, art model.Article) (model.ArticleText, error) {
	session := &fetchSession{fetcher: f, navigator: nav, accumulator: NewAccumulator()}
	session.progress.PageIndex = 1
	session.progress.Steps = append(session.progress.Steps, model.Step{Kind: model.StepNavigate, PageIndex: 1})

	if err := session.open(ctx, art.URL); err != nil {
		return model.ArticleText{}, err
	}
	if err := session.run(ctx); err != nil {
		return model.ArticleText{}, err
	}
	return session.result(art), nil
}

// fetchSession 承载单篇抓取的全部可变状态，让每个步骤都保持短小可读（§6.2 步骤 1–7）。
type fetchSession struct {
	fetcher     *Fetcher
	navigator   port.Navigator
	accumulator *Accumulator
	progress    model.NavProgress
	pageHTML    string
	current     Page
	first       Page
	hasFirst    bool
}

// open 导航到文章页、取首帧快照，并在**文章页**判定登录态（期号页不设门槛，§6.1 职责表）。
func (s *fetchSession) open(ctx context.Context, url string) error {
	if err := s.navigator.Navigate(ctx, url); err != nil {
		return classifyFailure("导航", err)
	}
	if err := s.reload(ctx); err != nil {
		return err
	}
	loggedOut, err := s.navigator.LogoutDetected(ctx)
	if err != nil {
		return classifyFailure("登出探测", err)
	}
	if loggedOut {
		return fmt.Errorf("%w：页面跳转到登录页或出现验证码", model.ErrLogin)
	}
	return nil
}

// reload 取当前页面 HTML 快照。Execute 之后 adapter 已完成点击后等待，此处只负责读取。
func (s *fetchSession) reload(ctx context.Context) error {
	pageHTML, err := s.navigator.HTML(ctx)
	if err != nil {
		return classifyFailure("读取页面", err)
	}
	s.pageHTML = pageHTML
	return nil
}

// run 执行导航循环：每轮先做完成判定（覆盖「余下全文单击即完整」），再取一个动作并执行。
func (s *fetchSession) run(ctx context.Context) error {
	for {
		if err := s.limitsReached(); err != nil {
			return err
		}
		page, err := Extract(s.pageHTML, s.fetcher.Set)
		if err != nil {
			return err
		}
		if !s.hasFirst {
			s.first, s.hasFirst = page, true
		}
		s.current = page
		if page.Facts.PaywallVisible {
			return fmt.Errorf("%w：付费墙可见，正文可能不完整（未登录或权限不足）", model.ErrLogin)
		}
		if s.complete() {
			break
		}
		pageAction, done, err := Plan(s.pageHTML, s.fetcher.Set, s.fetcher.Cfg.ElementVisibleTimeout)
		if err != nil {
			return err
		}
		if done {
			break
		}
		if err := s.advance(ctx, pageAction); err != nil {
			return err
		}
	}
	return s.verifyFinal()
}

// complete 用「已归档小节 ∪ 当前页小节」判定当前是否已可提前收尾（§6.2 步骤 3a）。
func (s *fetchSession) complete() bool {
	return Complete(s.fetcher.Set, s.mergedFacts()) == nil
}

// verifyFinal 对整篇正文做最终强制判定；未通过即返回带原因的 model.ErrFetch（§6.2 步骤 6）。
// 除三条 DOM 事实外，另加一条空正文防线：页面事实可能全部满足但没有任何段落，
// 此时若写 success 会产出空章节，故同样判失败。
func (s *fetchSession) verifyFinal() error {
	if err := Complete(s.fetcher.Set, s.mergedFacts()); err != nil {
		return err
	}
	if s.totalParagraphs() == 0 {
		return fmt.Errorf("%w：未提取到任何正文段落", model.ErrFetch)
	}
	return nil
}

// totalParagraphs 统计「已归档段落 + 尚未归档的当前页段落」，用于空正文判定。
func (s *fetchSession) totalParagraphs() int {
	total := len(s.accumulator.Paragraphs())
	if s.progress.PageIndex > s.progress.LastArchivedPage {
		total += len(s.current.Paragraphs)
	}
	return total
}

// mergedFacts 把当前页事实与已归档小节合并，供判据 c 使用。
func (s *fetchSession) mergedFacts() model.PageFacts {
	facts := s.current.Facts
	facts.BodySections = s.accumulator.BodySectionsWith(s.current.Facts.BodySections)
	return facts
}

// limitsReached 检查单篇点击合计上限与动作总步数兜底闸（spec 3.7-5、架构 §6.2 H7）。
func (s *fetchSession) limitsReached() error {
	if s.progress.ClicksOnArticle >= s.fetcher.Cfg.ArticleClickLimit {
		return fmt.Errorf("%w：单篇点击次数达到上限 %d", model.ErrFetch, s.fetcher.Cfg.ArticleClickLimit)
	}
	if len(s.progress.Steps) >= s.fetcher.Cfg.ArticleTotalSteps {
		return fmt.Errorf("%w：单篇动作步数达到上限 %d", model.ErrFetch, s.fetcher.Cfg.ArticleTotalSteps)
	}
	return nil
}

// advance 执行一个动作并推进状态：先按需归档当前页终态，再取下一页快照并推进页码。
// 展开动作原地生效，不换页、不归档（同一页只在离开时归档一次）。
func (s *fetchSession) advance(ctx context.Context, pageAction model.PageAction) error {
	if err := s.fetcher.Waiter.BetweenClicks(ctx); err != nil {
		return classifyFailure("篇内等待", err)
	}
	if err := s.navigator.Execute(ctx, pageAction); err != nil {
		return classifyFailure("执行动作", err)
	}
	s.recordStep(pageAction)
	s.progress.ClicksOnArticle++

	if pageAction.Kind == model.ActionNextPage {
		s.accumulator.Append(s.current)
		s.progress.LastArchivedPage = s.progress.PageIndex
		s.progress.PageIndex++
	}
	return s.reload(ctx)
}

// recordStep 把动作记入可审计的 NavProgress（§4 Step 定义）。
func (s *fetchSession) recordStep(pageAction model.PageAction) {
	kind := model.StepExpand
	if pageAction.Kind == model.ActionNextPage {
		kind = model.StepNextPage
	}
	s.progress.Steps = append(s.progress.Steps, model.Step{
		Kind:      kind,
		PageIndex: s.progress.PageIndex,
		Selector:  pageAction.Selector,
	})
}

// result 收尾归档当前页（覆盖早退 / done / 末页三种出口），并返回标题、作者与拼接后的正文。
// 标题与作者优先取第 1 页快照（§6.2 步骤 5）；页面缺失这些节点时回退到列表页条目信息，
// 保证「结构略有差异的文章」不会因标题缺失而整篇失败（2026-09-25 实测）。
func (s *fetchSession) result(art model.Article) model.ArticleText {
	if s.progress.PageIndex > s.progress.LastArchivedPage {
		s.accumulator.Append(s.current)
		s.progress.LastArchivedPage = s.progress.PageIndex
	}
	s.progress.Steps = append(s.progress.Steps, model.Step{Kind: model.StepDone, PageIndex: s.progress.PageIndex})

	title := s.first.Title
	if strings.TrimSpace(title) == "" {
		title = art.Title
	}
	author := s.first.Author
	if strings.TrimSpace(author) == "" {
		author = art.Author
	}

	return model.ArticleText{
		Order:      art.Order,
		Title:      title,
		Author:     author,
		Paragraphs: s.accumulator.Paragraphs(),
	}
}

// classifyFailure 统一错误分类：登录类错误保持 ErrLogin 供上层恢复，其余一律归类为 ErrFetch。
// 用 %w 包装保留底层错误，errors.Is 仍可命中（§5 约定）。
func classifyFailure(stage string, err error) error {
	if errors.Is(err, model.ErrLogin) {
		return fmt.Errorf("%s失败：%w", stage, err)
	}
	return fmt.Errorf("%w：%s失败：%w", model.ErrFetch, stage, err)
}
