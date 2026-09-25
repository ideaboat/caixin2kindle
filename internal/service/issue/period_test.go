package issue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// fixturesDir 指向仓库 testdata/fixtures（spec 3.5-3：解析不得靠猜，一律以快照为准）。
const fixturesDir = "../../../testdata/fixtures"

// mustFixture 读取一个已脱敏的 fixture。
func mustFixture(t *testing.T, name string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(fixturesDir, name))
	if err != nil {
		t.Fatalf("读取 fixture %s 失败：%v", name, err)
	}
	return string(raw)
}

func TestMatchPeriod(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		want  string
		found bool
	}{
		{name: "总第写法", text: "《财新周刊》总第1224期", want: "财新周刊第1224期", found: true},
		{name: "第 N 期写法", text: "《财新周刊》第1224期", want: "财新周刊第1224期", found: true},
		{name: "带分隔符", text: "财新周刊 | 第1234期", want: "财新周刊第1234期", found: true},
		{name: "容忍全角空格", text: "《财新周刊》总第\u30001224\u3000期", want: "财新周刊第1224期", found: true},
		{name: "容忍换行空白", text: "《财新周刊》\n 总第 1224 期", want: "财新周刊第1224期", found: true},
		{name: "无期号", text: "财新周刊频道目录", want: "", found: false},
		{name: "卷期不配总第但配第N期（调用方限定作用节点）", text: "2026年第37期", want: "财新周刊第37期", found: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got, found := MatchPeriod(tc.text)

			// Assert
			if found != tc.found {
				t.Fatalf("found = %v，期望 %v", found, tc.found)
			}
			if got != tc.want {
				t.Errorf("got = %q，期望 %q", got, tc.want)
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	longCJK := strings.Repeat("财", 100)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "正常期号保持不变", in: "财新周刊第1224期", want: "财新周刊第1224期"},
		{name: "非法字符替换为下划线", in: `a/b\c:d*e?f"g<h>i|j`, want: "a_b_c_d_e_f_g_h_i_j"},
		{name: "折叠连续空白与下划线", in: "a  b__c\t\td", want: "a_b_c_d"},
		{name: "去首尾空白与下划线", in: "  _abc_  ", want: "abc"},
		{name: "丢弃尾部点", in: "abc...", want: "abc"},
		{name: "NFC 归一化", in: "e\u0301", want: "é"},
		{name: "空串拒绝", in: "   ", want: ""},
		{name: "单点拒绝", in: ".", want: ""},
		{name: "双点拒绝", in: "..", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := Sanitize(tc.in)

			// Assert
			if got != tc.want {
				t.Errorf("Sanitize(%q) = %q，期望 %q", tc.in, got, tc.want)
			}
		})
	}

	t.Run("超长按 rune 边界截断到 120 字节", func(t *testing.T) {
		// Act
		got := Sanitize(longCJK)

		// Assert
		if len(got) > 120 {
			t.Errorf("长度 = %d 字节，期望 ≤ 120", len(got))
		}
		if !utf8.ValidString(got) {
			t.Errorf("截断破坏了 UTF-8：%q", got)
		}
		if got != strings.Repeat("财", 40) {
			t.Errorf("期望恰好 40 个汉字，实际 %q", got)
		}
	})
}

func TestSlugFromURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{name: "年期两段", url: "https://weekly.caixin.com/2026/cw1224/", want: "2026-cw1224"},
		{name: "无尾斜杠", url: "https://weekly.caixin.com/2026/cw1224", want: "2026-cw1224"},
		{name: "带 query", url: "https://weekly.caixin.com/2026/cw1224/?from=x", want: "2026-cw1224"},
		{name: "无年份取末两段", url: "https://example.com/a/b/c/", want: "b-c"},
		{name: "单段", url: "https://example.com/weekly/", want: "weekly"},
		{name: "空路径", url: "https://example.com/", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := SlugFromURL(tc.url)

			// Assert
			if got != tc.want {
				t.Errorf("SlugFromURL(%q) = %q，期望 %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "去 query", raw: "https://weekly.caixin.com/x/1.html?from=weekly", want: "https://weekly.caixin.com/x/1.html"},
		{name: "去 fragment", raw: "https://weekly.caixin.com/x/1.html#page2", want: "https://weekly.caixin.com/x/1.html"},
		{name: "去尾斜杠", raw: "https://weekly.caixin.com/x/1.html/", want: "https://weekly.caixin.com/x/1.html"},
		{name: "不做大小写折叠", raw: "https://Weekly.Caixin.com/X/1.html", want: "https://Weekly.Caixin.com/X/1.html"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := NormalizeURL(tc.raw)

			// Assert
			if got != tc.want {
				t.Errorf("NormalizeURL(%q) = %q，期望 %q", tc.raw, got, tc.want)
			}
		})
	}
}
