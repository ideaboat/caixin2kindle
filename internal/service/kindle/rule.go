// Package kindle 只做 Kindle 挂载点的纯规则：给定 --kindle 与候选卷列表，选出目标卷。
// 读盘由 adapter/publish.VolumeScanner 负责，创建目录与复制由 DeviceWriter 负责（§7）。
package kindle

import (
	"fmt"
	"path/filepath"

	"caixin2kindle/internal/model"
)

// Selector 实现 app.KindleSelector。类型本身无状态，方法为纯规则，可直接单测。
type Selector struct{}

// NewSelector 构造 Kindle 挂载点选择器。
func NewSelector() *Selector {
	return &Selector{}
}

// Select 按 Kindle 挂载点判定表选出目标卷（M7 / C2 已定案）：
//   - 显式指定且路径存在 → 命中该卷（即使缺 documents/，由调用方创建）；
//   - 显式指定但路径不存在 → 返回包装 model.ErrCopy 的错误，且**不回退扫描**
//     （用户写错的路径若被静默改写到别的卷，可能把杂志拷进错误设备，是最坏结果）；
//   - 未显式指定 → 取候选卷中第一个含 documents/ 者，命中或未命中均不报错。
func (s *Selector) Select(req model.KindleRequest, volumes []model.Volume) (model.Volume, bool, error) {
	if req.Explicit {
		for _, volume := range volumes {
			if sameMount(volume.Path, req.Mount) {
				return volume, true, nil
			}
		}
		return model.Volume{}, false, fmt.Errorf("%w：指定的挂载点不存在：%s", model.ErrCopy, req.Mount)
	}

	for _, volume := range volumes {
		if volume.HasDocuments {
			return volume, true, nil
		}
	}
	return model.Volume{}, false, nil
}

// NeedDocumentsDir 报告是否应在目标卷上创建 documents/ 目录。
// 仅在用户**显式指定**挂载点时创建，自动扫描阶段保持只读探测（C2）。
func NeedDocumentsDir(req model.KindleRequest) bool {
	return req.Explicit
}

// sameMount 比较两个挂载点是否相同：规范化路径后比较，兼容尾部斜杠等写法。
func sameMount(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
