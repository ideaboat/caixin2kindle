package config

import (
	"errors"
	"testing"
	"time"

	"caixin2kindle/internal/model"
)

func TestParseDelay(t *testing.T) {
	cases := []struct {
		name    string
		spec    string
		wantMin time.Duration
		wantMax time.Duration
		wantErr bool
	}{
		{name: "区间", spec: "3-8", wantMin: 3 * time.Second, wantMax: 8 * time.Second},
		{name: "单值等价 N-N", spec: "5", wantMin: 5 * time.Second, wantMax: 5 * time.Second},
		{name: "全零合法", spec: "0-0", wantMin: 0, wantMax: 0},
		{name: "相等上下界", spec: "2-2", wantMin: 2 * time.Second, wantMax: 2 * time.Second},
		{name: "容忍空白", spec: " 3 - 8 ", wantMin: 3 * time.Second, wantMax: 8 * time.Second},
		{name: "上下界颠倒", spec: "8-3", wantErr: true},
		{name: "负数", spec: "-1-3", wantErr: true},
		{name: "缺上界", spec: "3-", wantErr: true},
		{name: "缺下界", spec: "-3", wantErr: true},
		{name: "非整数", spec: "a-b", wantErr: true},
		{name: "小数", spec: "1.5-3", wantErr: true},
		{name: "三段", spec: "1-2-3", wantErr: true},
		{name: "空串", spec: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			gotMin, gotMax, err := ParseDelay(tc.spec)

			// Assert
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseDelay(%q) 期望报错，实际得到 %v/%v", tc.spec, gotMin, gotMax)
				}
				if !errors.Is(err, model.ErrUsage) {
					t.Fatalf("ParseDelay(%q) 错误未包装 ErrUsage：%v", tc.spec, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDelay(%q) 意外报错：%v", tc.spec, err)
			}
			if gotMin != tc.wantMin || gotMax != tc.wantMax {
				t.Fatalf("ParseDelay(%q) = %v/%v，期望 %v/%v", tc.spec, gotMin, gotMax, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	env := Env{HomeDir: "/Users/tester", WorkDir: "/work"}

	cases := []struct {
		name string
		path string
		env  Env
		want string
	}{
		{name: "展开波浪号", path: "~/Downloads/caixin", env: env, want: "/Users/tester/Downloads/caixin"},
		{name: "波浪号单独成路径", path: "~", env: env, want: "/Users/tester"},
		{name: "相对路径按工作目录", path: "rel/out", env: env, want: "/work/rel/out"},
		{name: "绝对路径清理", path: "/abs/./x/../y", env: env, want: "/abs/y"},
		{name: "波浪号后回退", path: "~/a/../b", env: env, want: "/Users/tester/b"},
		{name: "已是绝对路径", path: "/Volumes/Kindle", env: env, want: "/Volumes/Kindle"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := ResolvePath(tc.path, tc.env)

			// Assert
			if got != tc.want {
				t.Fatalf("ResolvePath(%q) = %q，期望 %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	env := Env{HomeDir: "/Users/tester", WorkDir: "/work"}

	t.Run("无覆盖时保持默认并归一化路径", func(t *testing.T) {
		// Act
		cfg, err := Load(Overrides{}, env)

		// Assert
		if err != nil {
			t.Fatalf("Load 意外报错：%v", err)
		}
		if cfg.DelayMin != 3*time.Second || cfg.DelayMax != 8*time.Second {
			t.Fatalf("默认延迟区间错误：%v-%v", cfg.DelayMin, cfg.DelayMax)
		}
		if cfg.OutDir != "/Users/tester/Downloads/caixin" {
			t.Fatalf("OutDir 未归一化：%q", cfg.OutDir)
		}
		if cfg.BrowserProfileDir != "/Users/tester/.caixin/browser-profile" {
			t.Fatalf("BrowserProfileDir 未归一化：%q", cfg.BrowserProfileDir)
		}
		if cfg.KindleMountExplicit {
			t.Fatal("未给 --kindle 时 KindleMountExplicit 应为 false")
		}
		if cfg.ArticleClickLimit != 50 {
			t.Fatalf("ArticleClickLimit 默认值错误：%d", cfg.ArticleClickLimit)
		}
	})

	t.Run("覆盖项生效", func(t *testing.T) {
		// Arrange
		out := "/tmp/caixin-out"
		kindle := "/Volumes/Paperwhite"
		browser := "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
		delay := "10-20"
		noKindle := true
		full := true
		headless := true

		// Act
		cfg, err := Load(Overrides{
			Out:      &out,
			Kindle:   &kindle,
			Browser:  &browser,
			Delay:    &delay,
			NoKindle: &noKindle,
			Full:     &full,
			Headless: &headless,
		}, env)

		// Assert
		if err != nil {
			t.Fatalf("Load 意外报错：%v", err)
		}
		if cfg.OutDir != out || cfg.KindleMount != kindle || cfg.BrowserPath != browser {
			t.Fatalf("路径类覆盖未生效：out=%q kindle=%q browser=%q", cfg.OutDir, cfg.KindleMount, cfg.BrowserPath)
		}
		if !cfg.KindleMountExplicit {
			t.Fatal("给出 --kindle 后 KindleMountExplicit 应为 true")
		}
		if cfg.DelayMin != 10*time.Second || cfg.DelayMax != 20*time.Second {
			t.Fatalf("--delay 覆盖未生效：%v-%v", cfg.DelayMin, cfg.DelayMax)
		}
		if !cfg.NoKindle || !cfg.Full || !cfg.Headless {
			t.Fatalf("开关类覆盖未生效：no-kindle=%v full=%v headless=%v", cfg.NoKindle, cfg.Full, cfg.Headless)
		}
	})

	t.Run("相对输出目录按工作目录解析", func(t *testing.T) {
		// Arrange
		out := "rel/out"

		// Act
		cfg, err := Load(Overrides{Out: &out}, env)

		// Assert
		if err != nil {
			t.Fatalf("Load 意外报错：%v", err)
		}
		if cfg.OutDir != "/work/rel/out" {
			t.Fatalf("相对路径未按工作目录解析：%q", cfg.OutDir)
		}
	})

	t.Run("非法 delay 返回 ErrUsage", func(t *testing.T) {
		// Arrange
		delay := "8-3"

		// Act
		_, err := Load(Overrides{Delay: &delay}, env)

		// Assert
		if !errors.Is(err, model.ErrUsage) {
			t.Fatalf("期望包装 ErrUsage，实际：%v", err)
		}
	})
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "默认配置合法", mutate: func(*Config) {}},
		{name: "延迟下界为负", mutate: func(c *Config) { c.DelayMin = -1 }, wantErr: true},
		{name: "延迟上下界颠倒", mutate: func(c *Config) { c.DelayMin, c.DelayMax = 9*time.Second, 3*time.Second }, wantErr: true},
		{name: "点击上限为 0", mutate: func(c *Config) { c.ArticleClickLimit = 0 }, wantErr: true},
		{name: "总步数为 0", mutate: func(c *Config) { c.ArticleTotalSteps = 0 }, wantErr: true},
		{name: "重试次数为 0", mutate: func(c *Config) { c.ArticleRetryLimit = 0 }, wantErr: true},
		{name: "退避不足", mutate: func(c *Config) { c.RetryBackoff = nil }, wantErr: true},
		{name: "熔断阈值为 0", mutate: func(c *Config) { c.ConsecutiveFailureLimit = 0 }, wantErr: true},
		{name: "DOM 稳定判据次数为 0", mutate: func(c *Config) { c.DOMStableChecks = 0 }, wantErr: true},
		{name: "DOM 稳定轮询间隔为 0", mutate: func(c *Config) { c.DOMStablePollInterval = 0 }, wantErr: true},
		{name: "关闭等待上限为 0", mutate: func(c *Config) { c.BrowserCloseWait = 0 }, wantErr: true},
		{name: "输出 profile 为空", mutate: func(c *Config) { c.OutputProfile = "" }, wantErr: true},
		{name: "输出目录为空", mutate: func(c *Config) { c.OutDir = "" }, wantErr: true},
		{name: "显式挂载点为空", mutate: func(c *Config) { c.KindleMountExplicit = true; c.KindleMount = "" }, wantErr: true},
		{name: "浏览器候选与兜底全空且未显式指定", mutate: func(c *Config) {
			c.BrowserCandidates = nil
			c.BrowserEnvFallbacks = nil
			c.BrowserPath = ""
		}, wantErr: true},
		{name: "仅显式指定浏览器时合法", mutate: func(c *Config) {
			c.BrowserCandidates = nil
			c.BrowserEnvFallbacks = nil
			c.BrowserPath = "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := Default()
			tc.mutate(&cfg)

			// Act
			err := cfg.Validate()

			// Assert
			if tc.wantErr {
				if err == nil {
					t.Fatal("期望校验失败，实际通过")
				}
				if !errors.Is(err, model.ErrUsage) {
					t.Fatalf("校验错误未包装 ErrUsage：%v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("期望校验通过，实际：%v", err)
			}
		})
	}
}
