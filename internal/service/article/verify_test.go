package article

import (
	"errors"
	"strings"
	"testing"

	"caixin2kindle/internal/model"
)

func TestCompleteByFacts(t *testing.T) {
	set := defaultSet()
	cases := []struct {
		name    string
		facts   model.PageFacts
		wantErr bool
		wantSub string
	}{
		{
			name: "全部满足：无付费墙、按钮穷尽、小节齐全",
			facts: model.PageFacts{
				NavSections:  []string{"一", "二"},
				BodySections: []string{"一", "二"},
			},
		},
		{
			name: "无 #pageNav 时判据 c 自然成立",
			facts: model.PageFacts{
				BodySections: []string{"甲"},
			},
		},
		{
			name: "付费墙可见即失败（判据 a）",
			facts: model.PageFacts{
				PaywallVisible: true,
				NavSections:    []string{"一"},
				BodySections:   []string{"一"},
			},
			wantErr: true,
			wantSub: "付费墙",
		},
		{
			name: "仍有余下全文按钮即失败（判据 b）",
			facts: model.PageFacts{
				Buttons:      []string{"余下全文"},
				NavSections:  []string{"一"},
				BodySections: []string{"一"},
			},
			wantErr: true,
			wantSub: "余下全文",
		},
		{
			name: "仍有下一页按钮即失败（判据 b）",
			facts: model.PageFacts{
				Buttons:      []string{"上一页", "下一页"},
				NavSections:  []string{"一"},
				BodySections: []string{"一"},
			},
			wantErr: true,
			wantSub: "下一页",
		},
		{
			name: "只剩上一页不构成失败",
			facts: model.PageFacts{
				Buttons:      []string{"上一页"},
				NavSections:  []string{"一"},
				BodySections: []string{"一"},
			},
		},
		{
			name: "小节缺失即失败（判据 c）",
			facts: model.PageFacts{
				NavSections:  []string{"一", "二", "三", "四"},
				BodySections: []string{"一", "二", "三"},
			},
			wantErr: true,
			wantSub: "四",
		},
		{
			name: "小节空白折叠后可比对",
			facts: model.PageFacts{
				NavSections:  []string{" 一 "},
				BodySections: []string{"一"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			err := Complete(set, tc.facts)

			// Assert
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("期望通过，实际：%v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("期望失败，实际为 nil")
			}
			if !errors.Is(err, model.ErrFetch) {
				t.Errorf("错误应包装 model.ErrFetch，实际：%v", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("错误信息 %q 应包含 %q", err.Error(), tc.wantSub)
			}
		})
	}
}
