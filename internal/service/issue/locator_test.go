package issue

import (
	"context"
	"errors"
	"testing"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/selector"
	"caixin2kindle/internal/testutil"
)

func TestLocateFixture(t *testing.T) {
	// Arrange
	nav := &testutil.FakeNavigator{Frames: []string{mustFixture(t, "issue-settled.html")}}
	locator := NewLocator(selector.Default())

	// Act
	issue, err := locator.Locate(context.Background(), nav, issueURL)

	// Assert
	if err != nil {
		t.Fatalf("Locate 返回错误：%v", err)
	}
	if nav.NavigatedURL != issueURL {
		t.Errorf("导航 URL = %q，期望 %q", nav.NavigatedURL, issueURL)
	}
	if issue.ID != "财新周刊第1224期" {
		t.Errorf("ID = %q，期望 %q", issue.ID, "财新周刊第1224期")
	}
	if len(issue.Articles) != 6 {
		t.Errorf("文章数 = %d，期望 6", len(issue.Articles))
	}
}

func TestLocateErrorClassification(t *testing.T) {
	locator := NewLocator(selector.Default())
	cases := []struct {
		name string
		nav  *testutil.FakeNavigator
		want error
	}{
		{
			name: "导航遇到登录失败",
			nav:  &testutil.FakeNavigator{NavigateErr: model.ErrLogin},
			want: model.ErrLogin,
		},
		{
			name: "导航失败归为抓取失败",
			nav:  &testutil.FakeNavigator{NavigateErr: errors.New("network down")},
			want: model.ErrFetch,
		},
		{
			name: "无文章归为抓取失败",
			nav:  &testutil.FakeNavigator{Frames: []string{`<html><head><title>《财新周刊》第1期</title></head><body></body></html>`}},
			want: model.ErrFetch,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			_, err := locator.Locate(context.Background(), tc.nav, issueURL)

			// Assert
			if !errors.Is(err, tc.want) {
				t.Fatalf("错误应包装 %v，实际：%v", tc.want, err)
			}
		})
	}
}
