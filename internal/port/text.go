package port

// TextStore 是正文文件的只读能力，供 service/state 在构建前复核正文。
// 消费方为 service/state，实现方为 adapter/storage。
type TextStore interface {
	// Exists 报告 path 处的正文文件是否存在。
	Exists(path string) (bool, error)
	// ReadArticleText 读取 path 处的正文纯文本。
	ReadArticleText(path string) (string, error)
}
