package page

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/selector"
)

// fakeFileInfo 是 os.FileInfo 的最小实现，供 stat 注入使用。
type fakeFileInfo struct {
	name  string
	isDir bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0o755 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return nil }

func TestResolveBrowserPath(t *testing.T) {
	const (
		explicit   = "/opt/custom/chrome"
		candidate1 = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		candidate2 = "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
		envChrome  = "google-chrome"
	)

	tests := []struct {
		name     string
		cfg      config.Config
		existing map[string]bool // path → isDir
		pathHits map[string]string
		want     string
		wantErr  error
		wantMsg  []string
	}{
		{
			name:     "显式指定且存在时直接采用",
			cfg:      config.Config{BrowserPath: explicit, BrowserCandidates: []string{candidate1}, BrowserEnvFallbacks: []string{envChrome}},
			existing: map[string]bool{explicit: false},
			want:     explicit,
		},
		{
			name:     "显式指定不存在则报错且不回退",
			cfg:      config.Config{BrowserPath: explicit, BrowserCandidates: []string{candidate1}, BrowserEnvFallbacks: []string{envChrome}},
			existing: map[string]bool{candidate1: false},
			pathHits: map[string]string{envChrome: "/usr/local/bin/google-chrome"},
			wantErr:  model.ErrDependency,
			wantMsg:  []string{explicit, "不回退"},
		},
		{
			name:     "显式指定是目录则报错",
			cfg:      config.Config{BrowserPath: explicit},
			existing: map[string]bool{explicit: true},
			wantErr:  model.ErrDependency,
			wantMsg:  []string{explicit},
		},
		{
			name:     "自动探测跳过目录取第一个存在的非目录",
			cfg:      config.Config{BrowserCandidates: []string{candidate1, candidate2}},
			existing: map[string]bool{candidate1: true, candidate2: false},
			want:     candidate2,
		},
		{
			name:     "自动探测未命中时走 PATH 兜底",
			cfg:      config.Config{BrowserCandidates: []string{candidate1}, BrowserEnvFallbacks: []string{"chromium", envChrome}},
			pathHits: map[string]string{envChrome: "/usr/local/bin/google-chrome"},
			want:     "/usr/local/bin/google-chrome",
		},
		{
			name:    "三级全部未命中返回依赖缺失",
			cfg:     config.Config{BrowserCandidates: []string{candidate1}, BrowserEnvFallbacks: []string{envChrome}},
			wantErr: model.ErrDependency,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stat := func(path string) (os.FileInfo, error) {
				isDir, ok := test.existing[path]
				if !ok {
					return nil, os.ErrNotExist
				}
				return fakeFileInfo{name: filepath.Base(path), isDir: isDir}, nil
			}
			lookPath := func(name string) (string, error) {
				hit, ok := test.pathHits[name]
				if !ok {
					return "", errors.New("not found in PATH")
				}
				return hit, nil
			}

			got, err := resolveBrowserPath(test.cfg, stat, lookPath)

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("resolveBrowserPath() error = %v，期望 errors.Is(_, %v)", err, test.wantErr)
				}
				for _, fragment := range test.wantMsg {
					if !strings.Contains(err.Error(), fragment) {
						t.Fatalf("resolveBrowserPath() error = %q，期望包含 %q", err.Error(), fragment)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveBrowserPath() 意外错误 = %v", err)
			}
			if got != test.want {
				t.Fatalf("resolveBrowserPath() = %q，期望 %q", got, test.want)
			}
		})
	}
}

func TestBrowserFlagSpecs(t *testing.T) {
	const profileDir = "/Users/tester/.caixin/browser-profile"

	mandatory := map[string]any{
		"user-data-dir":            profileDir,
		"window-size":              "1440,1000",
		"disable-blink-features":   "AutomationControlled",
		"test-type":                true,
		"no-first-run":             true,
		"no-default-browser-check": true,
	}

	tests := []struct {
		name    string
		cfg     config.Config
		wantLen int
		extra   map[string]any
		absent  []string
	}{
		{
			name:    "默认有头且不暴露调试端口",
			cfg:     config.Config{},
			wantLen: len(mandatory),
			absent:  []string{"headless", "remote-debugging-port"},
		},
		{
			name:    "无头模式追加 headless=new",
			cfg:     config.Config{Headless: true},
			wantLen: len(mandatory) + 1,
			extra:   map[string]any{"headless": "new"},
		},
		{
			name:    "开启远程调试时追加 9222 端口",
			cfg:     config.Config{EnableRemoteDebug: true},
			wantLen: len(mandatory) + 1,
			extra:   map[string]any{"remote-debugging-port": "9222"},
		},
		{
			name:    "无头且开启远程调试时两条都追加",
			cfg:     config.Config{Headless: true, EnableRemoteDebug: true},
			wantLen: len(mandatory) + 2,
			extra:   map[string]any{"headless": "new", "remote-debugging-port": "9222"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			specs := browserFlagSpecs(test.cfg, profileDir)

			if len(specs) != test.wantLen {
				t.Fatalf("browserFlagSpecs() 长度 = %d，期望 %d（%+v）", len(specs), test.wantLen, specs)
			}
			for name, want := range mandatory {
				got, ok := specValue(specs, name)
				if !ok {
					t.Fatalf("browserFlagSpecs() 缺少必选参数 %q", name)
				}
				if got != want {
					t.Fatalf("browserFlagSpecs()[%q] = %#v，期望 %#v", name, got, want)
				}
			}
			for name, want := range test.extra {
				got, ok := specValue(specs, name)
				if !ok || got != want {
					t.Fatalf("browserFlagSpecs()[%q] = %#v（存在=%v），期望 %#v", name, got, ok, want)
				}
			}
			for _, name := range test.absent {
				if _, ok := specValue(specs, name); ok {
					t.Fatalf("browserFlagSpecs() 不应包含参数 %q", name)
				}
			}
			if _, ok := specValue(specs, "enable-automation"); ok {
				t.Fatalf("browserFlagSpecs() 不得包含 enable-automation")
			}
			for _, arg := range renderFlagSpecs(specs) {
				if strings.HasPrefix(arg, "--enable-automation") {
					t.Fatalf("渲染后的参数不得包含 --enable-automation：%v", renderFlagSpecs(specs))
				}
			}
		})
	}
}

func TestRenderFlagSpecs(t *testing.T) {
	specs := []flagSpec{
		{name: "user-data-dir", value: "/tmp/profile"},
		{name: "test-type", value: true},
		{name: "enable-automation", value: false},
		{name: "window-size", value: "1440,1000"},
	}

	got := renderFlagSpecs(specs)

	want := []string{
		"--user-data-dir=/tmp/profile",
		"--test-type",
		"--window-size=1440,1000",
	}
	if len(got) != len(want) {
		t.Fatalf("renderFlagSpecs() = %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("renderFlagSpecs()[%d] = %q，期望 %q", i, got[i], want[i])
		}
	}
}

func TestParseSingletonLockTarget(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		wantHost string
		wantPID  int
		wantOK   bool
	}{
		{name: "常规 host-pid", target: "macbook-1234", wantHost: "macbook", wantPID: 1234, wantOK: true},
		{name: "host 含连字符", target: "charlie-macbook-pro.local-98765", wantHost: "charlie-macbook-pro.local", wantPID: 98765, wantOK: true},
		{name: "缺少 pid", target: "macbook", wantOK: false},
		{name: "pid 非数字", target: "macbook-abc", wantOK: false},
		{name: "pid 为空", target: "macbook-", wantOK: false},
		{name: "缺少 host", target: "-42", wantOK: false},
		{name: "空串", target: "", wantOK: false},
		{name: "pid 为零", target: "macbook-0", wantOK: false},
		{name: "host 以连字符结尾时按最后一个连字符切分", target: "macbook--7", wantHost: "macbook-", wantPID: 7, wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host, pid, ok := parseSingletonLockTarget(test.target)

			if ok != test.wantOK {
				t.Fatalf("parseSingletonLockTarget(%q) ok = %v，期望 %v", test.target, ok, test.wantOK)
			}
			if host != test.wantHost || pid != test.wantPID {
				t.Fatalf("parseSingletonLockTarget(%q) = (%q, %d)，期望 (%q, %d)", test.target, host, pid, test.wantHost, test.wantPID)
			}
		})
	}
}

func TestNormalizeExitType(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		want        string
		wantChanged bool
	}{
		{
			name:        "精确字面替换且不重排 JSON",
			content:     `{"profile":{"exit_type":"Crashed","exit_count":1},"other":true}`,
			want:        `{"profile":{"exit_type":"Normal","exit_count":1},"other":true}`,
			wantChanged: true,
		},
		{
			name:        "带空格的变体同样归一化",
			content:     `{"profile":{"exit_type": "Crashed"}}`,
			want:        `{"profile":{"exit_type": "Normal"}}`,
			wantChanged: true,
		},
		{
			name:        "已为 Normal 时不动",
			content:     `{"profile":{"exit_type":"Normal"}}`,
			want:        `{"profile":{"exit_type":"Normal"}}`,
			wantChanged: false,
		},
		{
			name:        "不存在 exit_type 时原样返回",
			content:     `{"profile":{"exit_count":2}}`,
			want:        `{"profile":{"exit_count":2}}`,
			wantChanged: false,
		},
		{
			name:        "只替换第一次出现",
			content:     `"exit_type":"Crashed"-"exit_type":"Crashed"`,
			want:        `"exit_type":"Normal"-"exit_type":"Crashed"`,
			wantChanged: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, changed := normalizeExitType(test.content)

			if changed != test.wantChanged {
				t.Fatalf("normalizeExitType() changed = %v，期望 %v", changed, test.wantChanged)
			}
			if got != test.want {
				t.Fatalf("normalizeExitType() = %q，期望 %q", got, test.want)
			}
		})
	}
}

func TestProcessAlive(t *testing.T) {
	t.Run("当前进程存活", func(t *testing.T) {
		if !processAlive(os.Getpid()) {
			t.Fatalf("processAlive(os.Getpid()) = false，期望 true")
		}
	})

	t.Run("不可能的 pid 不存活", func(t *testing.T) {
		const impossiblePID = 1 << 30

		if processAlive(impossiblePID) {
			t.Skipf("pid %d 在本机意外存活，跳过", impossiblePID)
		}
	})

	t.Run("非正 pid 不存活", func(t *testing.T) {
		if processAlive(0) || processAlive(-1) {
			t.Fatalf("processAlive(0/-1) = true，期望 false")
		}
	})
}

func TestLogoutSignalJS(t *testing.T) {
	set := selector.Default()

	snippet := logoutSignalJS(set)

	for _, want := range append(append([]string{}, set.Login.Form...), set.Login.Captcha...) {
		if !strings.Contains(snippet, jsLiteralFor(t, want)) {
			t.Fatalf("logoutSignalJS() 缺少 selector %q：%s", want, snippet)
		}
	}
	if !strings.Contains(snippet, jsLiteralFor(t, set.Login.Paywall)) {
		t.Fatalf("logoutSignalJS() 缺少付费墙 selector %q", set.Login.Paywall)
	}
	for _, signal := range []string{"paywall", "login", "captcha"} {
		if !strings.Contains(snippet, "'"+signal+"'") {
			t.Fatalf("logoutSignalJS() 缺少信号返回值 %q：%s", signal, snippet)
		}
	}
	for _, fragment := range []string{"display:none", "toLowerCase", "replace(/\\s+/g,'')"} {
		if !strings.Contains(snippet, fragment) {
			t.Fatalf("logoutSignalJS() 缺少判据片段 %q：%s", fragment, snippet)
		}
	}
}

func TestLogoutSignalJS_EmptySelectorSet(t *testing.T) {
	snippet := logoutSignalJS(selector.Set{})

	if strings.Contains(snippet, `querySelector("")`) {
		t.Fatalf("空 selector 集合不得生成 querySelector(\"\")：%s", snippet)
	}
	if !strings.Contains(snippet, "return ''") {
		t.Fatalf("无命中时应返回空串：%s", snippet)
	}
}

func TestDOMClickJS(t *testing.T) {
	const target = `#pageBtn > a:nth-of-type(2)`

	snippet := domClickJS(target)

	if !strings.Contains(snippet, jsLiteralFor(t, target)) {
		t.Fatalf("domClickJS() 未引用目标 selector：%s", snippet)
	}
	if !strings.Contains(snippet, "el.click()") {
		t.Fatalf("domClickJS() 缺少 el.click()：%s", snippet)
	}
	if !strings.Contains(snippet, "return false") || !strings.Contains(snippet, "return true") {
		t.Fatalf("domClickJS() 未区分元素缺失与点击成功：%s", snippet)
	}
}

func TestDOMFingerprintJS(t *testing.T) {
	for _, fragment := range []string{"querySelectorAll('*').length", "document.body.textContent.length"} {
		if !strings.Contains(domFingerprintJS, fragment) {
			t.Fatalf("domFingerprintJS 缺少片段 %q：%s", fragment, domFingerprintJS)
		}
	}
}

func TestScrollToBottomJS(t *testing.T) {
	if !strings.Contains(scrollToBottomJS, "window.scrollTo(0, document.body.scrollHeight)") {
		t.Fatalf("scrollToBottomJS = %q", scrollToBottomJS)
	}
}

func TestStableChecksAfter(t *testing.T) {
	tests := []struct {
		name           string
		previous       string
		current        string
		previousChecks int
		want           int
	}{
		{name: "首次观察计为 1", previous: "", current: "10:20", previousChecks: 0, want: 1},
		{name: "指纹不变则累加", previous: "10:20", current: "10:20", previousChecks: 1, want: 2},
		{name: "指纹变化则重置为 1", previous: "10:20", current: "11:20", previousChecks: 2, want: 1},
		{name: "空指纹也能累加", previous: "", current: "", previousChecks: 1, want: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := stableChecksAfter(test.previous, test.current, test.previousChecks)

			if got != test.want {
				t.Fatalf("stableChecksAfter(%q, %q, %d) = %d，期望 %d",
					test.previous, test.current, test.previousChecks, got, test.want)
			}
		})
	}
}

func TestIsNetworkIdle(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	tests := []struct {
		name       string
		pending    int
		lastChange time.Time
		quiet      time.Duration
		want       bool
	}{
		{name: "无在途请求且静默足够久则空闲", pending: 0, lastChange: now.Add(-time.Second), quiet: 500 * time.Millisecond, want: true},
		{name: "无在途请求但刚有事件不算空闲", pending: 0, lastChange: now, quiet: 500 * time.Millisecond, want: false},
		{name: "有在途请求一律不空闲", pending: 2, lastChange: now.Add(-time.Hour), quiet: time.Millisecond, want: false},
		{name: "静默期为零时无在途即空闲", pending: 0, lastChange: now, quiet: 0, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := isNetworkIdle(test.pending, test.lastChange, now, test.quiet)

			if got != test.want {
				t.Fatalf("isNetworkIdle(%d, %v, %v, %s) = %v，期望 %v",
					test.pending, test.lastChange, now, test.quiet, got, test.want)
			}
		})
	}
}

// specValue 在 flagSpec 列表中查找指定名称的参数值（纯测试辅助）。
func specValue(specs []flagSpec, name string) (any, bool) {
	for _, spec := range specs {
		if spec.name == name {
			return spec.value, true
		}
	}
	return nil, false
}

// jsLiteralFor 用标准库独立算出字符串在 JS 源码中应有的 JSON 字面量（纯测试辅助）。
func jsLiteralFor(t *testing.T, value string) string {
	t.Helper()

	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		t.Fatalf("编码 %q 失败：%v", value, err)
	}
	return strings.TrimRight(buffer.String(), "\n")
}
