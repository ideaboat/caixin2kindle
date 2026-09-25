package issue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/selector"
	"caixin2kindle/internal/testutil"
)

// listingHTML 生成含 count 个条目的期号页 HTML，用于滚动稳定性用例。
func listingHTML(count int) string {
	var builder strings.Builder
	builder.WriteString(`<html><head><title>《财新周刊》第1224期</title></head><body>`)
	for index := 0; index < count; index++ {
		fmt.Fprintf(&builder,
			`<dl><dt><a href="https://weekly.caixin.com/2026-09-18/9000000%02d.html">示例标题%02d</a></dt></dl>`,
			index, index)
	}
	builder.WriteString(`</body></html>`)
	return builder.String()
}

func TestScrollUntilStableStopsAfterTwoUnchangedRounds(t *testing.T) {
	// Arrange：先增长再稳定
	nav := &testutil.FakeNavigator{
		Frames:         []string{listingHTML(3), listingHTML(6), listingHTML(6)},
		ScrollAdvances: true,
	}

	// Act
	err := ScrollUntilStable(context.Background(), nav, selector.Default())

	// Assert
	if err != nil {
		t.Fatalf("ScrollUntilStable 返回错误：%v", err)
	}
	if nav.ScrollCount != 3 {
		t.Errorf("滚动次数 = %d，期望 3（增长一轮 + 连续两轮不增）", nav.ScrollCount)
	}
}

func TestScrollUntilStableAlreadyStable(t *testing.T) {
	// Arrange：列表一开始就是全量
	nav := &testutil.FakeNavigator{Frames: []string{listingHTML(6)}}

	// Act
	err := ScrollUntilStable(context.Background(), nav, selector.Default())

	// Assert
	if err != nil {
		t.Fatalf("ScrollUntilStable 返回错误：%v", err)
	}
	if nav.ScrollCount != 2 {
		t.Errorf("滚动次数 = %d，期望 2（连续两轮计数不变即稳定）", nav.ScrollCount)
	}
}

// TestScrollUntilStableBounded 覆盖持续增长的兜底：10 轮内不收敛也必须正常返回，不得死循环。
func TestScrollUntilStableBounded(t *testing.T) {
	// Arrange
	frames := make([]string, 0, maxScrollRounds+1)
	for round := 1; round <= maxScrollRounds+1; round++ {
		frames = append(frames, listingHTML(round))
	}
	nav := &testutil.FakeNavigator{Frames: frames, ScrollAdvances: true}

	// Act
	err := ScrollUntilStable(context.Background(), nav, selector.Default())

	// Assert
	if err != nil {
		t.Fatalf("滚动轮数达上限时应正常返回，实际：%v", err)
	}
	if nav.ScrollCount != maxScrollRounds {
		t.Errorf("滚动次数 = %d，期望上限 %d", nav.ScrollCount, maxScrollRounds)
	}
}

func TestScrollUntilStablePropagatesErrors(t *testing.T) {
	cases := []struct {
		name string
		nav  *testutil.FakeNavigator
	}{
		{name: "取 HTML 失败", nav: &testutil.FakeNavigator{HTMLErr: errors.New("boom")}},
		{name: "滚动失败", nav: &testutil.FakeNavigator{Frames: []string{listingHTML(1)}, ScrollErr: errors.New("boom")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			err := ScrollUntilStable(context.Background(), tc.nav, selector.Default())

			// Assert
			if !errors.Is(err, model.ErrFetch) {
				t.Fatalf("错误应包装 model.ErrFetch，实际：%v", err)
			}
		})
	}
}
