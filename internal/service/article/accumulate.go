package article

// Accumulator 累积一篇的多页正文：段落与小节各自按「折叠空白后的文本」去重，保持首次出现顺序。
// 它是「不重不漏」的第二道保险——即使同一页被归档两次，也不会产生重复段落（§6.2 H5）。
type Accumulator struct {
	paragraphs    []string
	sections      []string
	paragraphKeys map[string]bool
	sectionKeys   map[string]bool
}

// NewAccumulator 构造空的正文累积器。
func NewAccumulator() *Accumulator {
	return &Accumulator{
		paragraphKeys: make(map[string]bool),
		sectionKeys:   make(map[string]bool),
	}
}

// Append 把一页的终态正文段落与小节标题并入累积器，重复项只保留首次出现。
func (a *Accumulator) Append(page Page) {
	for _, paragraph := range page.Paragraphs {
		if key := foldWhitespace(paragraph); key != "" && !a.paragraphKeys[key] {
			a.paragraphKeys[key] = true
			a.paragraphs = append(a.paragraphs, key)
		}
	}
	for _, section := range page.Facts.BodySections {
		if key := foldWhitespace(section); key != "" && !a.sectionKeys[key] {
			a.sectionKeys[key] = true
			a.sections = append(a.sections, key)
		}
	}
}

// Paragraphs 返回已累积的段落副本（调用方只读，避免外部改动内部状态）。
func (a *Accumulator) Paragraphs() []string {
	return append([]string(nil), a.paragraphs...)
}

// Sections 返回已累积的小节副本。
func (a *Accumulator) Sections() []string {
	return append([]string(nil), a.sections...)
}

// BodySectionsWith 返回「已归档小节 ∪ extra」的并集副本，供完成判定的判据 c 使用：
// 判据必须在整篇正文拼接完成后才成立，故当前页的小节也要计入（§6.2 步骤 6）。
func (a *Accumulator) BodySectionsWith(extra []string) []string {
	merged := make([]string, 0, len(a.sections)+len(extra))
	seen := make(map[string]bool, len(a.sections)+len(extra))
	for _, section := range append(a.Sections(), extra...) {
		key := foldWhitespace(section)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, key)
	}
	return merged
}
