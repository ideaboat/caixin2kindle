// Package issue 负责期号页的解析：期号、目录名与去重后的文章列表（spec 3.6）。
package issue

import (
	"context"
	"errors"
	"fmt"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/port"
	"caixin2kindle/internal/selector"
)

// Locator 实现 app.IssueLocator：导航到期号页、滚动至列表稳定、再解析期号与文章列表。
type Locator struct {
	Set selector.Set // selector 表，唯一定义处在 internal/selector
}

// NewLocator 构造期号页定位器。
func NewLocator(set selector.Set) *Locator {
	return &Locator{Set: set}
}

// Locate 打开期号页、滚动至列表稳定（spec 3.6），再解析期号与去重后的文章列表（§6.1 步骤 3）。
// 期号页不设登录门槛，未登录同样返回完整列表，故此处不做认证探测：登录判据只落在文章页。
func (l *Locator) Locate(ctx context.Context, page port.PageSource, rawURL string) (model.Issue, error) {
	if err := page.Navigate(ctx, rawURL); err != nil {
		if errors.Is(err, model.ErrLogin) {
			return model.Issue{}, fmt.Errorf("打开期号页：%w", err)
		}
		return model.Issue{}, fmt.Errorf("%w：打开期号页失败：%w", model.ErrFetch, err)
	}
	if err := ScrollUntilStable(ctx, page, l.Set); err != nil {
		return model.Issue{}, err
	}
	pageHTML, err := page.HTML(ctx)
	if err != nil {
		return model.Issue{}, fmt.Errorf("%w：读取期号页失败：%w", model.ErrFetch, err)
	}
	return ParseIssue(pageHTML, l.Set, rawURL)
}
