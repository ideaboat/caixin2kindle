package article

import (
	"strings"
	"testing"
	"time"

	"caixin2kindle/internal/model"
)

const planMaxWait = 5 * time.Second

func TestPlanPrefersExpandOverNextPage(t *testing.T) {
	// Arrange：并存页面 #pageBtn = [下一页][余下全文]（§8 复核项 4）
	html := mustFixture(t, "article-fulltext-before.html")
	set := defaultSet()

	// Act
	action, done, err := Plan(html, set, planMaxWait)

	// Assert
	if err != nil {
		t.Fatalf("Plan 返回错误：%v", err)
	}
	if done {
		t.Fatal("并存页面不应判为 done")
	}
	if action.Kind != model.ActionExpandFullText {
		t.Errorf("Kind = %v，期望 ActionExpandFullText（展开优先）", action.Kind)
	}
	wantSelector := "#the_content div#pageNext div#pageBtn > a:nth-of-type(2)"
	if action.Selector != wantSelector {
		t.Errorf("Selector = %q，期望 %q", action.Selector, wantSelector)
	}
	if action.MaxWait != planMaxWait {
		t.Errorf("MaxWait = %v，期望 %v", action.MaxWait, planMaxWait)
	}
}

func TestPlanRouteByFixture(t *testing.T) {
	set := defaultSet()
	cases := []struct {
		name         string
		html         string
		wantKind     model.ActionKind
		wantDone     bool
		wantSelector string
	}{
		{
			name:         "展开后按钮穷尽即 done",
			html:         mustFixture(t, "article-fulltext-after.html"),
			wantDone:     true,
			wantSelector: "",
		},
		{
			name:         "纯下一页首屏",
			html:         withoutExpand(t, "article-paged-01.html"),
			wantKind:     model.ActionNextPage,
			wantSelector: "#the_content div#pageNext div#pageBtn > a:nth-of-type(1)",
		},
		{
			name:         "纯下一页中间页（跳过上一页）",
			html:         withoutExpand(t, "article-paged-02.html"),
			wantKind:     model.ActionNextPage,
			wantSelector: "#the_content div#pageNext div#pageBtn > a:nth-of-type(2)",
		},
		{
			name:         "末页只剩上一页即 done",
			html:         mustFixture(t, "article-paged-04.html"),
			wantDone:     true,
			wantSelector: "",
		},
		{
			name:         "末页（去掉并存按钮）仍为 done",
			html:         withoutExpand(t, "article-paged-04.html"),
			wantDone:     true,
			wantSelector: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			action, done, err := Plan(tc.html, set, planMaxWait)

			// Assert
			if err != nil {
				t.Fatalf("Plan 返回错误：%v", err)
			}
			if done != tc.wantDone {
				t.Fatalf("done = %v，期望 %v", done, tc.wantDone)
			}
			if done {
				return
			}
			if action.Kind != tc.wantKind {
				t.Errorf("Kind = %v，期望 %v", action.Kind, tc.wantKind)
			}
			if action.Selector != tc.wantSelector {
				t.Errorf("Selector = %q，期望 %q", action.Selector, tc.wantSelector)
			}
		})
	}
}

// TestPlanEmptyHTMLIsDone 覆盖空输入边界：无按钮时判为 done，不产生动作。
func TestPlanEmptyHTMLIsDone(t *testing.T) {
	// Act
	_, done, err := Plan("", defaultSet(), planMaxWait)

	// Assert
	if err != nil {
		t.Fatalf("空 HTML 不应返回错误：%v", err)
	}
	if !done {
		t.Fatal("空 HTML 无按钮，应判为 done")
	}
}

// TestPlanSelectorStaysInAllowedSubset 钉住 §8.1 的 selector 子集约定：
// 点击目标只允许 标签/#id/.class/[attr]/直接子 >，禁止 :visible / :has() 等计算态写法。
func TestPlanSelectorStaysInAllowedSubset(t *testing.T) {
	// Act
	action, _, err := Plan(mustFixture(t, "article-fulltext-before.html"), defaultSet(), planMaxWait)

	// Assert
	if err != nil {
		t.Fatalf("Plan 返回错误：%v", err)
	}
	for _, forbidden := range []string{":visible", ":has(", ":contains("} {
		if strings.Contains(action.Selector, forbidden) {
			t.Errorf("Selector %q 含禁止写法 %q", action.Selector, forbidden)
		}
	}
}
