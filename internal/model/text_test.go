package model

import "testing"

func TestArticleTextBody(t *testing.T) {
	cases := []struct {
		name       string
		paragraphs []string
		want       string
	}{
		{name: "空正文", paragraphs: nil, want: ""},
		{name: "单段", paragraphs: []string{"第一段"}, want: "第一段"},
		{name: "多段以空行连接", paragraphs: []string{"甲", "乙", "丙"}, want: "甲\n\n乙\n\n丙"},
		{name: "保留段内换行", paragraphs: []string{"上\n下", "尾"}, want: "上\n下\n\n尾"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			text := ArticleText{Paragraphs: tc.paragraphs}

			// Act
			got := text.Body()

			// Assert
			if got != tc.want {
				t.Fatalf("Body() = %q，期望 %q", got, tc.want)
			}
		})
	}
}
