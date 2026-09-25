package issue

import (
	"context"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/port"
	"caixin2kindle/internal/selector"
)

// maxScrollRounds 是滚动加载的兜底轮数：即使列表始终在增长，也必须在有限轮内收敛（spec 3.6）。
const maxScrollRounds = 10

// ScrollUntilStable 滚动期号页直至文章列表稳定：列表条目数连续两次滚动后不再增加即视为稳定（§8.1 I13）。
// 达到 maxScrollRounds 仍未稳定时按稳定处理并返回，避免无限循环。
func ScrollUntilStable(ctx context.Context, page port.PageSource, set selector.Set) error {
	previous := -1
	stableRounds := 0

	for round := 0; round < maxScrollRounds; round++ {
		pageHTML, err := page.HTML(ctx)
		if err != nil {
			return fmt.Errorf("%w：读取期号页失败：%w", model.ErrFetch, err)
		}
		count, err := countEntries(pageHTML, set)
		if err != nil {
			return err
		}
		if count == previous {
			stableRounds++
			if stableRounds >= 2 {
				return nil
			}
		} else {
			stableRounds = 0
		}
		previous = count

		if err := page.ScrollToBottom(ctx); err != nil {
			return fmt.Errorf("%w：滚动期号页失败：%w", model.ErrFetch, err)
		}
	}
	return nil
}

// countEntries 统计页面中条目链接的数量，作为列表是否增长的判据（复用 I6，不另立选择器）。
func countEntries(pageHTML string, set selector.Set) (int, error) {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
	if err != nil {
		return 0, fmt.Errorf("%w：解析期号页失败：%w", model.ErrFetch, err)
	}
	return document.Find(set.Issue.EntryLink).Length(), nil
}
