package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
)

const testIssueURL = "https://weekly.caixin.com/2026/cw1224/"

func TestParseFlags_Errors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "缺少位置参数", args: nil},
		{name: "多余位置参数", args: []string{testIssueURL, "https://weekly.caixin.com/2026/cw1225/"}},
		{name: "未知 flag", args: []string{"--nope", testIssueURL}},
		{name: "flag 缺少取值", args: []string{"--out"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var stderr bytes.Buffer

			// Act
			_, help, err := parseFlags(tc.args, &stderr)

			// Assert
			if help {
				t.Fatalf("不应进入帮助分支")
			}
			if !errors.Is(err, model.ErrUsage) {
				t.Fatalf("应返回 ErrUsage，实际：%v", err)
			}
		})
	}
}

func TestParseFlags_Help(t *testing.T) {
	// Act
	overrides, help, err := parseFlags([]string{"--help"}, io.Discard)

	// Assert
	if err != nil {
		t.Fatalf("--help 不应报错：%v", err)
	}
	if !help {
		t.Fatalf("--help 应置 help=true")
	}
	if overrides.URL != nil || overrides.Out != nil || overrides.Kindle != nil {
		t.Fatalf("--help 不应产生覆盖项：%+v", overrides)
	}
}

func TestParseFlags_ExplicitFlags(t *testing.T) {
	// Arrange
	args := []string{
		"--out", "/tmp/out",
		"--kindle", "/Volumes/Kindle",
		"--browser", "/opt/chrome",
		"--delay", "5",
		"--no-kindle", "--full", "--headless",
		testIssueURL,
	}

	// Act
	overrides, help, err := parseFlags(args, io.Discard)

	// Assert
	if err != nil || help {
		t.Fatalf("解析失败：err=%v help=%v", err, help)
	}
	checks := []struct {
		name string
		got  bool
	}{
		{"URL", overrides.URL != nil && *overrides.URL == testIssueURL},
		{"Out", overrides.Out != nil && *overrides.Out == "/tmp/out"},
		{"Kindle", overrides.Kindle != nil && *overrides.Kindle == "/Volumes/Kindle"},
		{"Browser", overrides.Browser != nil && *overrides.Browser == "/opt/chrome"},
		{"Delay", overrides.Delay != nil && *overrides.Delay == "5"},
		{"NoKindle", overrides.NoKindle != nil && *overrides.NoKindle},
		{"Full", overrides.Full != nil && *overrides.Full},
		{"Headless", overrides.Headless != nil && *overrides.Headless},
	}
	for _, check := range checks {
		if !check.got {
			t.Fatalf("覆盖项 %s 未按显式参数设置：%+v", check.name, overrides)
		}
	}
}

func TestParseFlags_UnspecifiedStayNil(t *testing.T) {
	// Act
	overrides, _, err := parseFlags([]string{testIssueURL}, io.Discard)

	// Assert
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if overrides.URL == nil || *overrides.URL != testIssueURL {
		t.Fatalf("URL 覆盖项缺失：%+v", overrides)
	}
	if overrides.Out != nil || overrides.Kindle != nil || overrides.Browser != nil ||
		overrides.Delay != nil || overrides.NoKindle != nil || overrides.Full != nil || overrides.Headless != nil {
		t.Fatalf("未出现的参数不应产生覆盖项：%+v", overrides)
	}
}

func TestParseFlags_ConfigLoadFromOverrides(t *testing.T) {
	// Arrange
	overrides, _, err := parseFlags([]string{"--delay", "5", testIssueURL}, io.Discard)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}

	// Act
	cfg, err := config.Load(overrides, config.Env{HomeDir: "/home/demo", WorkDir: "/work"})

	// Assert
	if err != nil {
		t.Fatalf("Load 失败：%v", err)
	}
	if cfg.URL != testIssueURL {
		t.Fatalf("URL = %q", cfg.URL)
	}
	if cfg.DelayMin.Seconds() != 5 || cfg.DelayMax.Seconds() != 5 {
		t.Fatalf("单值 --delay 应等价 5-5，实际 %v-%v", cfg.DelayMin, cfg.DelayMax)
	}
}

func TestUsageText_RequiredSnippets(t *testing.T) {
	// Arrange
	snippets := []string{
		"用法：",
		"参数：",
		"默认值：",
		"--out",
		"--kindle",
		"--browser",
		"--delay",
		"--no-kindle",
		"--full",
		"--headless",
		"运行期间请勿操作自动化窗口",
		"首次运行需在浏览器中手动登录",
		"单值等价",
		"N-N",
		"--browser 显式指定但路径无效会报错且不回退自动探测",
		"跳过判定只证明本地正文文件未被改动，不检测线上更新；怀疑线上有更新请用 --full",
	}

	// Act / Assert
	for _, snippet := range snippets {
		if !strings.Contains(usageText, snippet) {
			t.Fatalf("--help 文案缺少 %q", snippet)
		}
	}
}
