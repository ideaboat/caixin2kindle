package kindle

import (
	"errors"
	"testing"

	"caixin2kindle/internal/model"
)

// volumesFixture 模拟 /Volumes 扫描结果：Kindle 卷带 documents/，另一个卷没有。
func volumesFixture() []model.Volume {
	return []model.Volume{
		{Path: "/Volumes/USB", HasDocuments: false, Writable: true},
		{Path: "/Volumes/Kindle", HasDocuments: true, Writable: true},
		{Path: "/Volumes/Paperwhite", HasDocuments: true, Writable: true},
	}
}

func TestSelectExplicitMount(t *testing.T) {
	cases := []struct {
		name       string
		request    model.KindleRequest
		wantPath   string
		wantFound  bool
		wantError  bool
		wantDocDir bool
	}{
		{
			name:       "显式指定且存在",
			request:    model.KindleRequest{Mount: "/Volumes/Kindle", Explicit: true},
			wantPath:   "/Volumes/Kindle",
			wantFound:  true,
			wantDocDir: true,
		},
		{
			name:       "显式指定带尾斜杠也匹配",
			request:    model.KindleRequest{Mount: "/Volumes/Kindle/", Explicit: true},
			wantPath:   "/Volumes/Kindle",
			wantFound:  true,
			wantDocDir: true,
		},
		{
			name:       "显式指定不含 documents 的卷仍命中",
			request:    model.KindleRequest{Mount: "/Volumes/USB", Explicit: true},
			wantPath:   "/Volumes/USB",
			wantFound:  true,
			wantDocDir: true,
		},
		{
			name:      "显式指定但不存在即报错，不回退扫描",
			request:   model.KindleRequest{Mount: "/Volumes/Nope", Explicit: true},
			wantFound: false,
			wantError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			selector := NewSelector()

			// Act
			volume, found, err := selector.Select(tc.request, volumesFixture())

			// Assert
			if tc.wantError {
				if !errors.Is(err, model.ErrCopy) {
					t.Fatalf("错误应包装 model.ErrCopy，实际：%v", err)
				}
				if found {
					t.Error("报错时 found 应为 false")
				}
				return
			}
			if err != nil {
				t.Fatalf("Select 返回错误：%v", err)
			}
			if found != tc.wantFound {
				t.Fatalf("found = %v，期望 %v", found, tc.wantFound)
			}
			if volume.Path != tc.wantPath {
				t.Errorf("Path = %q，期望 %q", volume.Path, tc.wantPath)
			}
			if got := NeedDocumentsDir(tc.request); got != tc.wantDocDir {
				t.Errorf("NeedDocumentsDir = %v，期望 %v", got, tc.wantDocDir)
			}
		})
	}
}

func TestSelectScansWhenNotExplicit(t *testing.T) {
	// Arrange
	selector := NewSelector()
	request := model.KindleRequest{Mount: "/Volumes/Kindle", Explicit: false}

	// Act
	volume, found, err := selector.Select(request, volumesFixture())

	// Assert
	if err != nil {
		t.Fatalf("Select 返回错误：%v", err)
	}
	if !found {
		t.Fatal("应扫描到含 documents/ 的卷")
	}
	if volume.Path != "/Volumes/Kindle" {
		t.Errorf("Path = %q，期望第一个含 documents/ 的卷 %q", volume.Path, "/Volumes/Kindle")
	}
	if NeedDocumentsDir(request) {
		t.Error("自动扫描阶段不得创建 documents/")
	}
}

// TestSelectScanIgnoresDefaultMountWithoutDocuments 覆盖情形 3：
// 默认挂载点存在但缺 documents/ 时，应继续找真正带 documents/ 的卷，而不是误用默认点。
func TestSelectScanIgnoresDefaultMountWithoutDocuments(t *testing.T) {
	// Arrange
	selector := NewSelector()
	request := model.KindleRequest{Mount: "/Volumes/Kindle", Explicit: false}
	volumes := []model.Volume{
		{Path: "/Volumes/Kindle", HasDocuments: false, Writable: true},
		{Path: "/Volumes/Paperwhite", HasDocuments: true, Writable: true},
	}

	// Act
	volume, found, err := selector.Select(request, volumes)

	// Assert
	if err != nil {
		t.Fatalf("Select 返回错误：%v", err)
	}
	if !found || volume.Path != "/Volumes/Paperwhite" {
		t.Errorf("应选中含 documents/ 的卷，实际 found=%v path=%q", found, volume.Path)
	}
}

func TestSelectNotFoundIsNotAnError(t *testing.T) {
	// Arrange
	selector := NewSelector()
	request := model.KindleRequest{Mount: "/Volumes/Kindle", Explicit: false}
	volumes := []model.Volume{{Path: "/Volumes/USB", HasDocuments: false, Writable: true}}

	// Act
	volume, found, err := selector.Select(request, volumes)

	// Assert
	if err != nil {
		t.Fatalf("未检测到设备属正常路径，不应报错，实际：%v", err)
	}
	if found {
		t.Errorf("不应命中任何卷，实际 %q", volume.Path)
	}
}

func TestSelectEmptyVolumes(t *testing.T) {
	// Arrange
	request := model.KindleRequest{Mount: "/Volumes/Kindle"}

	// Act
	_, found, err := NewSelector().Select(request, nil)

	// Assert
	if err != nil {
		t.Fatalf("空卷列表不应报错：%v", err)
	}
	if found {
		t.Error("空卷列表不应命中")
	}
}
