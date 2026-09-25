package article

import (
	"slices"
	"testing"

	"caixin2kindle/internal/model"
)

func TestAccumulatorMergesAndDeduplicates(t *testing.T) {
	// Arrange
	acc := NewAccumulator()
	first := Page{
		Paragraphs: []string{"甲", "乙"},
		Facts:      model.PageFacts{BodySections: []string{"小节一"}},
	}
	second := Page{
		Paragraphs: []string{"乙", "丙", "乙"},
		Facts:      model.PageFacts{BodySections: []string{"小节一", "小节二"}},
	}

	// Act
	acc.Append(first)
	acc.Append(second)

	// Assert
	if want := []string{"甲", "乙", "丙"}; !slices.Equal(acc.Paragraphs(), want) {
		t.Errorf("Paragraphs = %q，期望 %q", acc.Paragraphs(), want)
	}
	if want := []string{"小节一", "小节二"}; !slices.Equal(acc.Sections(), want) {
		t.Errorf("Sections = %q，期望 %q", acc.Sections(), want)
	}
}

// TestAccumulatorNormalizesBeforeDeduplicating 覆盖段落级归一化：
// 折叠空白、去首尾后再哈希去重，保持首次出现顺序。
func TestAccumulatorNormalizesBeforeDeduplicating(t *testing.T) {
	// Arrange
	acc := NewAccumulator()

	// Act
	acc.Append(Page{Paragraphs: []string{" 重复 段落 "}})
	acc.Append(Page{Paragraphs: []string{"重复 段落", "新段落\t带制表符"}})

	// Assert
	want := []string{"重复 段落", "新段落 带制表符"}
	if !slices.Equal(acc.Paragraphs(), want) {
		t.Errorf("Paragraphs = %q，期望 %q", acc.Paragraphs(), want)
	}
}

func TestAccumulatorEmptyPageIsNoop(t *testing.T) {
	// Arrange
	acc := NewAccumulator()

	// Act
	acc.Append(Page{})

	// Assert
	if len(acc.Paragraphs()) != 0 {
		t.Errorf("空页面不应产生段落，实际：%q", acc.Paragraphs())
	}
	if len(acc.Sections()) != 0 {
		t.Errorf("空页面不应产生小节，实际：%q", acc.Sections())
	}
}
