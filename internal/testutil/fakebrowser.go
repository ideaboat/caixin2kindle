// Package testutil 提供基于 interface 的 fake 与测试辅助，隔离外部 API、网络与文件系统
// （go-rules/testing.md 隔离要求）。本包只导入 model/port/config，不得导入 service/app/cli，
// 以免测试支撑包反向依赖业务层。
package testutil

import (
	"context"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/port"
)

// FakeNavigator 是按脚本回放页面 HTML 的 port.Navigator 实现：
// Navigate 定位到第一帧；每次 Execute 前进一帧，抵达末帧后停在末帧
// （便于模拟“按钮点了不消失”这类需要触达上限的场景）。
//
// 两种回放方式：Pages 按 URL 分别回放（多篇文章的集成用例），Frames 则是一条线性脚本
// （单篇文章的状态机用例）。Pages 非 nil 时优先。
type FakeNavigator struct {
	Frames         []string            // 线性脚本，按访问/动作顺序排列
	Pages          map[string][]string // 按 URL 回放：URL → 帧序列
	ScrollAdvances bool                // ScrollToBottom 是否前进一帧（模拟列表懒加载增长）

	NavigateErr error // 非 nil 时 Navigate 直接失败
	HTMLErr     error // 非 nil 时 HTML 直接失败
	ExecuteErr  error // 非 nil 时 Execute 直接失败
	ScrollErr   error // 非 nil 时 ScrollToBottom 直接失败
	Logout      bool  // LogoutDetected 的返回值
	LogoutErr   error // 非 nil 时 LogoutDetected 直接失败

	NavigatedURL  string             // 记录最后一次 Navigate 的 URL
	NavigatedURLs []string           // 记录全部 Navigate 的 URL
	Actions       []model.PageAction // 记录 Execute 收到的动作
	ScrollCount   int                // 记录滚动次数

	index int
}

var _ port.Navigator = (*FakeNavigator)(nil)

// Navigate 记录 URL 并把当前帧序列复位到第一帧。
func (f *FakeNavigator) Navigate(_ context.Context, url string) error {
	if f.NavigateErr != nil {
		return f.NavigateErr
	}
	f.NavigatedURL = url
	f.NavigatedURLs = append(f.NavigatedURLs, url)
	f.index = 0
	return nil
}

// HTML 返回当前帧的 HTML。
func (f *FakeNavigator) HTML(_ context.Context) (string, error) {
	if f.HTMLErr != nil {
		return "", f.HTMLErr
	}
	frames := f.currentFrames()
	if len(frames) == 0 {
		return "", nil
	}
	return frames[f.index], nil
}

// ScrollToBottom 记录滚动次数，并按需推进一帧。
func (f *FakeNavigator) ScrollToBottom(_ context.Context) error {
	if f.ScrollErr != nil {
		return f.ScrollErr
	}
	f.ScrollCount++
	if f.ScrollAdvances {
		f.advance()
	}
	return nil
}

// WaitDOMStable 恒为就绪（fake 回放的快照已是稳定态）。
func (f *FakeNavigator) WaitDOMStable(context.Context) error { return nil }

// WaitNetworkIdle 恒为就绪。
func (f *FakeNavigator) WaitNetworkIdle(context.Context) error { return nil }

// WaitVisible 恒为可见。
func (f *FakeNavigator) WaitVisible(context.Context, string) error { return nil }

// LogoutDetected 返回脚本设定的登出状态。
func (f *FakeNavigator) LogoutDetected(context.Context) (bool, error) {
	if f.LogoutErr != nil {
		return false, f.LogoutErr
	}
	return f.Logout, nil
}

// Execute 记录动作并前进一帧。
func (f *FakeNavigator) Execute(_ context.Context, action model.PageAction) error {
	if f.ExecuteErr != nil {
		return f.ExecuteErr
	}
	f.Actions = append(f.Actions, action)
	f.advance()
	return nil
}

// ActionKinds 返回已执行动作的种类序列，供断言点击路线的优先级与次数。
func (f *FakeNavigator) ActionKinds() []model.ActionKind {
	kinds := make([]model.ActionKind, 0, len(f.Actions))
	for _, action := range f.Actions {
		kinds = append(kinds, action.Kind)
	}
	return kinds
}

// currentFrames 返回当前 URL 对应的帧序列（无 Pages 时退化为线性脚本）。
func (f *FakeNavigator) currentFrames() []string {
	if f.Pages == nil {
		return f.Frames
	}
	return f.Pages[f.NavigatedURL]
}

// advance 前进一帧，末帧后保持不动。
func (f *FakeNavigator) advance() {
	if f.index < len(f.currentFrames())-1 {
		f.index++
	}
}
