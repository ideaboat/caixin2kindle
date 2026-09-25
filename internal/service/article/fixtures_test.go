package article

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/selector"
)

// fixturesDir 指向仓库 testdata/fixtures：selector 与判定回归的唯一输入来源（spec 3.5、架构 §8）。
const fixturesDir = "../../../testdata/fixtures"

// mustFixture 读取一个已脱敏的 fixture；缺失即测试失败（fixtures 已入库，必须存在）。
func mustFixture(t *testing.T, name string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(fixturesDir, name))
	if err != nil {
		t.Fatalf("读取 fixture %s 失败：%v", name, err)
	}
	return string(raw)
}

// expandAnchorPattern 匹配并存页面里的「余下全文」按钮，用于在测试内派生纯「下一页」路线
// （testdata/README.md §6.3）。
var expandAnchorPattern = regexp.MustCompile(`(?m)^<a [^>]*>余下全文</a>\s*\n?`)

// withoutExpand 去掉 fixture 中的「余下全文」按钮。
func withoutExpand(t *testing.T, name string) string {
	t.Helper()

	return expandAnchorPattern.ReplaceAllString(mustFixture(t, name), "")
}

// defaultSet 返回定稿 selector 表。
func defaultSet() selector.Set { return selector.Default() }

// testConfig 返回默认配置，并按需覆盖单篇上限（保持其余字段与生产一致）。
func testConfig(mutate func(*config.Config)) config.Config {
	cfg := config.Default()
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

// noopClickWaiter 是 ClickWaiter 的测试替身：不真正等待，避免测试被拟人化延迟拖慢。
type noopClickWaiter struct {
	calls int
	err   error
}

// BetweenClicks 记录调用次数后立即返回。
func (w *noopClickWaiter) BetweenClicks(context.Context) error {
	w.calls++
	return w.err
}
