package article

import (
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/selector"
)

// Plan 探测当前页面存在哪种全文机制，返回**一个**最高优先级动作（§6.2 核心不变式）：
// 优先级 1「余下全文」→ 优先级 2「下一页」→ 无动作时 done 为 true。
// 返回的 Selector 直接指向目标按钮（:nth-of-type 定位），执行方无需再判断文案。
func Plan(pageHTML string, set selector.Set, maxWait time.Duration) (model.PageAction, bool, error) {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
	if err != nil {
		return model.PageAction{}, false, fmt.Errorf("%w：解析文章页失败：%w", model.ErrFetch, err)
	}

	buttons := document.Find(set.Article.PageButtons)
	if index, found := findButtonIndex(buttons, set.Article.LabelExpand); found {
		return action(model.ActionExpandFullText, set.Article.PageBtnContainer, index, maxWait), false, nil
	}
	if index, found := findButtonIndex(buttons, set.Article.LabelNextPage); found {
		return action(model.ActionNextPage, set.Article.PageBtnContainer, index, maxWait), false, nil
	}
	return model.PageAction{}, true, nil
}

// findButtonIndex 在按钮集内按折叠后文本精确匹配 label，返回 1 起算的序号。
func findButtonIndex(buttons *goquery.Selection, label string) (int, bool) {
	found := 0
	buttons.EachWithBreak(func(index int, button *goquery.Selection) bool {
		if foldWhitespace(selectionText(button)) == label {
			found = index + 1
			return false
		}
		return true
	})
	return found, found > 0
}

// action 组装一个页面动作；selector 限定在 selector 子集内（标签/#id/.class/[attr]/直接子 >）。
func action(kind model.ActionKind, container string, index int, maxWait time.Duration) model.PageAction {
	return model.PageAction{
		Kind:     kind,
		Selector: fmt.Sprintf("%s > a:nth-of-type(%d)", container, index),
		MaxWait:  maxWait,
	}
}
