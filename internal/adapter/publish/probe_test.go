package publish

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"caixin2kindle/internal/config"
)

// newTestProbe 用给定浏览器配置构造探测器，字段注入留给各用例。
func newTestProbe(browserPath string, candidates, fallbacks []string) *Probe {
	return NewProbe(config.Config{
		BrowserPath:         browserPath,
		BrowserCandidates:   candidates,
		BrowserEnvFallbacks: fallbacks,
	})
}

func TestProbeCheck_BrowserExplicit(t *testing.T) {
	const explicitPath = "/opt/custom/chrome"
	cases := []struct {
		name        string
		statInfo    os.FileInfo
		statErr     error
		wantMissing bool
	}{
		{name: "显式路径存在且是普通文件", statInfo: fakeFile("chrome")},
		{name: "显式路径不存在", statErr: os.ErrNotExist, wantMissing: true},
		{name: "显式路径是目录", statInfo: fakeDir("chrome"), wantMissing: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var statPaths, lookPathNames []string
			probe := newTestProbe(explicitPath, []string{"/Applications/Chrome"}, []string{"google-chrome"})
			probe.Stat = func(path string) (os.FileInfo, error) {
				statPaths = append(statPaths, path)
				if path == explicitPath {
					return tc.statInfo, tc.statErr
				}
				return nil, os.ErrNotExist
			}
			probe.LookPath = func(name string) (string, error) {
				lookPathNames = append(lookPathNames, name)
				return availableLookPath(ebookConvertExecutable)(name)
			}

			// Act
			missing := probe.Check(context.Background())

			// Assert
			for _, path := range statPaths {
				if path != explicitPath && path != calibreBundlePath {
					t.Errorf("显式指定时不得探测候选路径，却 Stat 了 %q", path)
				}
			}
			for _, name := range lookPathNames {
				if name == "google-chrome" {
					t.Error("显式指定时不得回退到 PATH 探测")
				}
			}
			if !tc.wantMissing {
				if len(missing) != 0 {
					t.Fatalf("显式路径可用时不应报告缺失，实际：%+v", missing)
				}
				return
			}
			if len(missing) != 1 {
				t.Fatalf("应只报告浏览器缺失，实际：%+v", missing)
			}
			if missing[0].Name != browserExplicitName {
				t.Errorf("Name = %q，期望 %q", missing[0].Name, browserExplicitName)
			}
			if !strings.Contains(missing[0].Hint, explicitPath) || !strings.Contains(missing[0].Hint, "不回退") {
				t.Errorf("Hint 应含具体路径与不回退说明，实际：%q", missing[0].Hint)
			}
		})
	}
}

func TestProbeCheck_BrowserAutoDetect(t *testing.T) {
	candidates := []string{"/Applications/Google Chrome", "/Applications/Brave Browser"}
	fallbacks := []string{"google-chrome", "chromium"}

	cases := []struct {
		name            string
		statResults     map[string]fakeFileInfo
		lookPathHits    map[string]bool
		wantStatPaths   []string
		wantLookPathLog []string
		wantMissing     bool
	}{
		{
			name:            "第一个候选存在即停止探测",
			statResults:     map[string]fakeFileInfo{candidates[0]: fakeFile("chrome")},
			wantStatPaths:   []string{candidates[0]},
			wantLookPathLog: []string{ebookConvertExecutable},
		},
		{
			name: "候选是目录则跳过取下一个",
			statResults: map[string]fakeFileInfo{
				candidates[0]: fakeDir("Chrome"),
				candidates[1]: fakeFile("brave"),
			},
			wantStatPaths:   []string{candidates[0], candidates[1]},
			wantLookPathLog: []string{ebookConvertExecutable},
		},
		{
			name:            "候选全未命中时 PATH 兜底命中",
			statResults:     map[string]fakeFileInfo{},
			lookPathHits:    map[string]bool{"chromium": true},
			wantStatPaths:   candidates,
			wantLookPathLog: []string{"google-chrome", "chromium", ebookConvertExecutable},
		},
		{
			name:            "候选与兜底全失败",
			statResults:     map[string]fakeFileInfo{},
			wantStatPaths:   candidates,
			wantLookPathLog: []string{"google-chrome", "chromium", ebookConvertExecutable},
			wantMissing:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var statPaths, lookPathLog []string
			probe := newTestProbe("", candidates, fallbacks)
			probe.Stat = func(path string) (os.FileInfo, error) {
				statPaths = append(statPaths, path)
				if info, ok := tc.statResults[path]; ok {
					return info, nil
				}
				return nil, os.ErrNotExist
			}
			probe.LookPath = func(name string) (string, error) {
				lookPathLog = append(lookPathLog, name)
				if name == ebookConvertExecutable || tc.lookPathHits[name] {
					return "/usr/bin/" + name, nil
				}
				return "", os.ErrNotExist
			}

			// Act
			missing := probe.Check(context.Background())

			// Assert
			if !equalStrings(statPaths, tc.wantStatPaths) {
				t.Errorf("Stat 调用序列 = %v，期望 %v", statPaths, tc.wantStatPaths)
			}
			if !equalStrings(lookPathLog, tc.wantLookPathLog) {
				t.Errorf("LookPath 调用序列 = %v，期望 %v", lookPathLog, tc.wantLookPathLog)
			}
			if tc.wantMissing {
				if len(missing) != 1 || missing[0].Name != browserName {
					t.Fatalf("应报告 %q 缺失，实际：%+v", browserName, missing)
				}
				return
			}
			if len(missing) != 0 {
				t.Fatalf("浏览器可用时不应报告缺失，实际：%+v", missing)
			}
		})
	}
}

func TestProbeCheck_EbookConvert(t *testing.T) {
	const explicitChrome = "/opt/chrome"
	cases := []struct {
		name            string
		lookPathOK      bool
		bundleInfo      os.FileInfo
		bundleErr       error
		wantBundleStat  bool
		wantMissing     bool
		wantName        string
		wantHintKeyword string
	}{
		{name: "PATH 命中则不查应用包", lookPathOK: true},
		{
			name:           "PATH 未命中但应用包存在",
			bundleInfo:     fakeFile("ebook-convert"),
			wantBundleStat: true,
		},
		{
			name:            "应用包是目录视为缺失",
			bundleInfo:      fakeDir("ebook-convert"),
			bundleErr:       nil,
			wantBundleStat:  true,
			wantMissing:     true,
			wantName:        ebookConvertName,
			wantHintKeyword: "calibre",
		},
		{
			name:            "两者都未命中",
			bundleErr:       os.ErrNotExist,
			wantBundleStat:  true,
			wantMissing:     true,
			wantName:        ebookConvertName,
			wantHintKeyword: "calibre",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			bundleStatCalled := false
			probe := newTestProbe(explicitChrome, nil, nil)
			probe.Stat = func(path string) (os.FileInfo, error) {
				if path == explicitChrome {
					return fakeFile("chrome"), nil
				}
				bundleStatCalled = true
				return tc.bundleInfo, tc.bundleErr
			}
			probe.LookPath = func(name string) (string, error) {
				if tc.lookPathOK && name == ebookConvertExecutable {
					return "/usr/local/bin/ebook-convert", nil
				}
				return "", os.ErrNotExist
			}

			// Act
			missing := probe.Check(context.Background())

			// Assert
			if bundleStatCalled != tc.wantBundleStat {
				t.Errorf("应用包 Stat 调用 = %v，期望 %v", bundleStatCalled, tc.wantBundleStat)
			}
			if !tc.wantMissing {
				if len(missing) != 0 {
					t.Fatalf("依赖齐备时不应报告缺失，实际：%+v", missing)
				}
				return
			}
			if len(missing) != 1 {
				t.Fatalf("应只报告 ebook-convert 缺失，实际：%+v", missing)
			}
			if missing[0].Name != tc.wantName {
				t.Errorf("Name = %q，期望 %q", missing[0].Name, tc.wantName)
			}
			if !strings.Contains(missing[0].Hint, tc.wantHintKeyword) {
				t.Errorf("Hint 应含 %q，实际：%q", tc.wantHintKeyword, missing[0].Hint)
			}
		})
	}
}

func TestProbeCheck_AllMissingInOrder(t *testing.T) {
	// Arrange
	probe := newTestProbe("", []string{"/Applications/Chrome"}, []string{"google-chrome"})
	probe.Stat = statFromMap(nil)
	probe.LookPath = availableLookPath()

	// Act
	missing := probe.Check(context.Background())

	// Assert
	if len(missing) != 2 {
		t.Fatalf("应报告两项缺失，实际：%+v", missing)
	}
	if missing[0].Name != browserName || missing[1].Name != ebookConvertName {
		t.Errorf("缺失项顺序应为浏览器在前、ebook-convert 在后，实际：%+v", missing)
	}
}

func TestProbeCheck_HealthyReturnsEmptyNonNil(t *testing.T) {
	// Arrange
	probe := newTestProbe("/opt/chrome", nil, nil)
	probe.Stat = statFromMap(map[string]fakeFileInfo{
		"/opt/chrome":     fakeFile("chrome"),
		calibreBundlePath: fakeFile("ebook-convert"),
	})
	probe.LookPath = availableLookPath()

	// Act
	missing := probe.Check(context.Background())

	// Assert
	if missing == nil {
		t.Fatal("依赖齐备时应返回非 nil 空切片，便于调用方统一处理")
	}
	if len(missing) != 0 {
		t.Fatalf("依赖齐备时不应报告缺失，实际：%+v", missing)
	}
}

func TestProbeCheck_CancelledContext(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probe := &Probe{}

	// Act
	missing := probe.Check(ctx)

	// Assert
	if missing == nil || len(missing) != 0 {
		t.Fatalf("上下文已取消时应返回非 nil 空切片，实际：%+v", missing)
	}
}

func TestNewProbe_CapturesConfig(t *testing.T) {
	// Arrange
	candidates := []string{"/Applications/Chrome", "/Applications/Brave"}
	fallbacks := []string{"google-chrome"}
	wantCandidates := []string{"/Applications/Chrome", "/Applications/Brave"}
	wantFallbacks := []string{"google-chrome"}
	cfg := config.Config{
		BrowserPath:         "/opt/browser",
		BrowserCandidates:   candidates,
		BrowserEnvFallbacks: fallbacks,
	}

	// Act
	probe := NewProbe(cfg)
	cfg.BrowserCandidates[0] = "/mutated"

	// Assert
	if probe.browserPath != "/opt/browser" {
		t.Errorf("browserPath = %q，期望 %q", probe.browserPath, "/opt/browser")
	}
	if !equalStrings(probe.browserCandidates, wantCandidates) {
		t.Errorf("浏览器候选应被复制而非别名引用，实际 %v", probe.browserCandidates)
	}
	if !equalStrings(probe.browserFallbacks, wantFallbacks) {
		t.Errorf("PATH 兜底 = %v，期望 %v", probe.browserFallbacks, wantFallbacks)
	}
	if !equalStrings(probe.ebookCandidates, []string{calibreBundlePath}) {
		t.Errorf("ebook 候选 = %v，期望 [%s]", probe.ebookCandidates, calibreBundlePath)
	}
}

func TestProbe_DefaultFuncs(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	path := filepath.Join(dir, "chrome")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备临时文件失败：%v", err)
	}
	probe := &Probe{}

	// Act
	info, statErr := probe.stat()(path)
	_, lookErr := probe.lookPath()("")

	// Assert
	if statErr != nil {
		t.Fatalf("默认 Stat 应使用 os.Stat，实际错误：%v", statErr)
	}
	if info.IsDir() {
		t.Error("普通文件不应被判为目录")
	}
	if lookErr == nil {
		t.Error("空命令名应返回错误")
	}
}

// equalStrings 比较两个字符串切片是否逐元素相等（顺序敏感）。
func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
