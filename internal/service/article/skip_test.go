package article

import (
	"strings"
	"testing"
)

func TestSkipReason(t *testing.T) {
	cases := []struct {
		name     string
		title    string
		wantSkip bool
	}{
		{name: "显影栏目跳过", title: "显影｜亲历德国式家庭照护", wantSkip: true},
		{name: "带空白仍匹配", title: "  显影｜示例", wantSkip: true},
		{name: "普通文字稿不跳过", title: "财新周刊｜巨灾如何保险", wantSkip: false},
		{name: "其他栏目不跳过", title: "阅读｜人间游戏", wantSkip: false},
		{name: "空标题不跳过", title: "", wantSkip: false},
		{name: "仅标题开头匹配", title: "回声｜显影故事", wantSkip: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			reason := SkipReason(tc.title)

			// Assert
			if (reason != "") != tc.wantSkip {
				t.Fatalf("SkipReason(%q) = %q，期望跳过=%v", tc.title, reason, tc.wantSkip)
			}
			if reason != "" && !strings.Contains(reason, "图片") {
				t.Errorf("跳过原因应说明是图片型栏目，实际 %q", reason)
			}
		})
	}
}
