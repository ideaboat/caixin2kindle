package article

import (
	"fmt"
	"strings"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/selector"
)

// Complete 对整篇正文做最终成功判定（§6.2 步骤 6，spec 3.7-7），三条 DOM 事实全部满足才返回 nil：
//
//	a. 无付费墙：div#chargeWallContent 不可见；
//	b. 按钮穷尽：#pageBtn 内不再有「余下全文」或「下一页」；
//	c. 小节齐全：正文小节覆盖 #pageNav 的全部小节标题（无 #pageNav 时自然成立）。
//
// 任一条件不满足返回包装 model.ErrFetch 的错误，调用方据此记 fail 并重试，绝不写 success。
func Complete(set selector.Set, facts model.PageFacts) error {
	if facts.PaywallVisible {
		return fmt.Errorf("%w：付费墙可见，正文可能不完整（未登录或权限不足）", model.ErrFetch)
	}

	for _, button := range facts.Buttons {
		switch button {
		case set.Article.LabelExpand:
			return fmt.Errorf("%w：仍有「%s」按钮，展开未穷尽", model.ErrFetch, button)
		case set.Article.LabelNextPage:
			return fmt.Errorf("%w：仍有「%s」按钮，未翻到末页", model.ErrFetch, button)
		}
	}

	if missing := missingSections(facts.NavSections, facts.BodySections); len(missing) > 0 {
		return fmt.Errorf("%w：缺少小节：%s", model.ErrFetch, strings.Join(missing, "、"))
	}
	return nil
}

// missingSections 返回小节表中存在、但正文小节里缺失的标题（折叠空白后按集合比对，保持期望顺序）。
func missingSections(want, got []string) []string {
	present := make(map[string]bool, len(got))
	for _, section := range got {
		present[foldWhitespace(section)] = true
	}

	var missing []string
	for _, section := range want {
		if key := foldWhitespace(section); !present[key] {
			missing = append(missing, key)
		}
	}
	return missing
}
