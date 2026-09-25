package app

import (
	"context"
	"errors"
	"testing"

	"caixin2kindle/internal/model"
)

// 本文件覆盖主流程各环节的失败路径：外部依赖出错时必须分类正确、及时中断，而不是静默继续。

func TestRunOpenFailurePropagates(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.opener.Err = model.ErrDependency

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrDependency) {
		t.Fatalf("错误应包装 model.ErrDependency，实际：%v", err)
	}
	if len(h.nav.NavigatedURLs) != 0 {
		t.Errorf("浏览器启动失败时不应导航，实际 %v", h.nav.NavigatedURLs)
	}
}

func TestRunWorkspaceCreationFailureIsUsageError(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.artifacts.EnsureErr = errors.New("mkdir denied")

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrUsage) {
		t.Fatalf("工作区创建失败应包装 model.ErrUsage（退出码 1），实际：%v", err)
	}
}

func TestRunFullCleanFailureIsUsageError(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.cfg.Full = true
	h.artifacts.CleanErr = errors.New("remove denied")

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrUsage) {
		t.Fatalf("--full 清理失败应包装 model.ErrUsage（退出码 1），实际：%v", err)
	}
}

func TestRunStateLoadFailureIsFetchError(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.states.LoadErr = errors.New("state io")

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("读取进度失败应包装 model.ErrFetch，实际：%v", err)
	}
}

func TestRunBuildFailureIsConvertError(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.epub.Err = errors.New("zip boom")

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrConvert) {
		t.Fatalf("EPUB 生成失败应包装 model.ErrConvert（退出码 4），实际：%v", err)
	}
}

func TestRunWriteArticleFailureIsFetchError(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.artifacts.WriteErr = errors.New("disk full")

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("写正文失败应包装 model.ErrFetch，实际：%v", err)
	}
	if h.epub.Calls != 0 {
		t.Error("写正文失败时不应继续构建 EPUB")
	}
}

func TestRunBetweenArticlesFailurePropagates(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.waiter.ArticleErr = errors.New("interrupted")

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("篇间等待失败应包装 model.ErrFetch，实际：%v", err)
	}
}

func TestRunVolumeScanFailureIsCopyError(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.scanner.Err = errors.New("cannot read /Volumes")

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrCopy) {
		t.Fatalf("扫描挂载卷失败应包装 model.ErrCopy（退出码 5），实际：%v", err)
	}
}

func TestRunLoginRecoveryFailureIsLoginError(t *testing.T) {
	// Arrange：付费墙常驻且人工确认失败
	h := newHarness(t)
	h.scriptAllArticlesAs("article-paywall.html")
	h.login.Err = model.ErrLogin

	// Act
	_, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if !errors.Is(err, model.ErrLogin) {
		t.Fatalf("登录恢复失败应包装 model.ErrLogin（退出码 2），实际：%v", err)
	}
	if h.login.Calls != 1 {
		t.Errorf("恢复失败后应立即中断，实际调用 %d 次", h.login.Calls)
	}
}
