package memory

import "unicode"

// Tokenizer是Token计数的抽象接口
type Tokenizer interface {
	Count(text string) int
}

// SimpleTokenizer基于字符数估算Token
// 粗略估算：1个中文字符≈2 tokens，1个英文单词≈1.3 tokens
type SimpleTokenizer struct{}

// Count估算文本的Token数
func (t *SimpleTokenizer) Count(text string) int {
	if text == "" {
		return 0
	}

	tokens := 0
	inWord := false

	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			// 中文字符：每个≈2 tokens
			tokens += 2
			inWord = false
		} else if unicode.IsSpace(r) {
			// 空格：分词边界
			inWord = false
		} else {
			// 英文字符：按单词计数
			if !inWord {
				tokens++ // 新单词开始
				inWord = true
			}
		}
	}

	return tokens
}
