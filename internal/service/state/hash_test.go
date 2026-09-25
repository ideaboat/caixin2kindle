package state

import (
	"testing"
)

func TestHash(t *testing.T) {
	// Arrange：sha256("abc") 的已知向量
	const abcDigest = "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

	// Act
	got := Hash("abc")

	// Assert
	if got != abcDigest {
		t.Errorf("Hash(\"abc\") = %q，期望 %q", got, abcDigest)
	}
}

func TestHashDeterministicAndSensitive(t *testing.T) {
	// Arrange
	body := "示例正文·第一段\n\n示例正文·第二段"

	// Act
	first := Hash(body)
	second := Hash(body)
	changed := Hash(body + "。")

	// Assert
	if first != second {
		t.Errorf("同一输入两次哈希不一致：%q vs %q", first, second)
	}
	if first == changed {
		t.Error("内容变化后哈希应随之变化")
	}
	if len(first) != len("sha256:")+64 {
		t.Errorf("哈希长度 = %d，期望 sha256 的 7+64", len(first))
	}
}
