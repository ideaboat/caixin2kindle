package model

// SchemaVersion 是当前支持的 state.json 结构版本；不匹配即视为无状态、走全量。
const SchemaVersion = 1

// State 是 .caixin2kindle/state.json 的落盘结构，是进度的唯一依据。
type State struct {
	SchemaVersion int            `json:"schema_version"`
	IssueID       string         `json:"issue_id"`
	Articles      []ArticleState `json:"articles"`
	Artifacts     Artifact       `json:"artifacts"`
}

// ArticleState 是单篇文章的进度记录。
type ArticleState struct {
	Order         int    `json:"order"`
	URL           string `json:"url"`
	NormalizedURL string `json:"normalized_url"`
	Title         string `json:"title"`
	Status        Status `json:"status"`
	TextPath      string `json:"text_path"`
	Hash          string `json:"hash"` // 对提取后的正文纯文本计算，形如 "sha256:..."
}

// Artifact 记录已构建的产物。BuiltIssueID 只在 EPUB 与 MOBI 都成功产出后写入，
// 是“产物已构建”的唯一标记（§7 H6 重建判定）。
type Artifact struct {
	EPUB         string `json:"epub"`
	MOBI         string `json:"mobi"`
	BuiltIssueID string `json:"built_issue_id"`
}
