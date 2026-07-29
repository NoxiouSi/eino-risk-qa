package llm_test

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/NoxiouSi/eino-risk-qa/internal/infra/llm"
)

func feedInChunks(scanner *llm.ArgumentScanner, full string, chunkSize int) string {
	var got string
	for i := 0; i < len(full); i += chunkSize {
		end := i + chunkSize
		if end > len(full) {
			end = len(full)
		}
		got += scanner.Feed(full[i:end])
	}
	return got
}

// feedInChunksCollectingParts 与 feedInChunks 类似，但额外返回每一次 Feed 调用产出的独立片段，
// 用于校验"每一个单独的增量片段本身都是合法 UTF-8"，而不仅仅是拼接后的整体合法。
// 这正是曾经出现过的 bug：拼接结果正确，但中间某次调用的片段截断了一个多字节字符。
func feedInChunksCollectingParts(scanner *llm.ArgumentScanner, full string, chunkSize int) []string {
	var parts []string
	for i := 0; i < len(full); i += chunkSize {
		end := i + chunkSize
		if end > len(full) {
			end = len(full)
		}
		if d := scanner.Feed(full[i:end]); d != "" {
			parts = append(parts, d)
		}
	}
	return parts
}

func TestArgumentScanner_ExtractsFollowUpQuestionIncrementally(t *testing.T) {
	full := `{"completeness":false,"reasonableness":true,"extracted_info":{},"reasoning_summary":"x","follow_up_question":"您的任职时间是？"}`

	scanner := llm.NewArgumentScanner()
	got := feedInChunks(scanner, full, 3)

	assert.Equal(t, "您的任职时间是？", got)
	assert.True(t, scanner.Started())
	assert.Equal(t, full, scanner.FullArguments())
}

func TestArgumentScanner_NoFollowUpQuestion_ProducesNoDelta(t *testing.T) {
	full := `{"completeness":true,"reasonableness":true,"extracted_info":{},"reasoning_summary":"x","follow_up_question":""}`

	scanner := llm.NewArgumentScanner()
	got := feedInChunks(scanner, full, 4)

	assert.Equal(t, "", got)
	assert.True(t, scanner.Started())
}

// TestArgumentScanner_ColonWithSpace_StillMatches 是回归测试：真实 DeepSeek API 输出的
// 参数 JSON 在冒号后带一个空格（如 `"follow_up_question": "..."`），而非本项目 MockChatModel
// 使用的紧凑格式（`"follow_up_question":"..."`，无空格）。曾经的 bug——固定字符串标记不容忍
// 该空格，导致对接真实 DeepSeek 时增量输出完全失效（delta_count 恒为 0），而 mock/单测从未
// 覆盖这一格式差异。
func TestArgumentScanner_ColonWithSpace_StillMatches(t *testing.T) {
	full := `{"completeness": false, "reasonableness": true, "extracted_info": {}, "follow_up_question": "您的任职时间是？", "reasoning_summary": "x"}`

	scanner := llm.NewArgumentScanner()
	got := feedInChunks(scanner, full, 3)

	assert.Equal(t, "您的任职时间是？", got)
	assert.True(t, scanner.Started())
}

// TestArgumentScanner_FollowUpQuestionNotLastField 验证 follow_up_question 并非参数 JSON
// 最后一个字段时（本项目已不再假设固定字段顺序），扫描器仍能正确提取其增量内容，
// 且字段闭合后不会被后续字段（如本例中的 reasoning_summary）的内容污染。
func TestArgumentScanner_FollowUpQuestionNotLastField(t *testing.T) {
	full := `{"completeness":false,"extracted_info":{},"follow_up_question":"追问内容","reasonableness":true,"reasoning_summary":"审计摘要"}`

	scanner := llm.NewArgumentScanner()
	got := feedInChunks(scanner, full, 3)

	assert.Equal(t, "追问内容", got)
	assert.Equal(t, full, scanner.FullArguments())
}

func TestArgumentScanner_FeedBeforeMarkerAppears_ReturnsEmpty(t *testing.T) {
	scanner := llm.NewArgumentScanner()

	got := scanner.Feed(`{"completeness":false,`)

	assert.Equal(t, "", got)
	assert.False(t, scanner.Started())
}

func TestArgumentScanner_SingleByteChunks(t *testing.T) {
	full := `{"completeness":false,"reasonableness":true,"extracted_info":{},"reasoning_summary":"x","follow_up_question":"追问内容"}`

	scanner := llm.NewArgumentScanner()
	got := feedInChunks(scanner, full, 1)

	assert.Equal(t, "追问内容", got)
}

// TestArgumentScanner_ByteAlignedChunks_NeverEmitsInvalidUTF8 是回归测试：验证以固定字节数
// （不考虑字符边界，如真实Provider的SSE分片或本项目MockChatModel的分片方式）逐块喂入时，
// 每一次 Feed 返回的增量片段本身都必须是合法 UTF-8，不允许出现截断多字节字符产生的
// U+FFFD/非法字节序列（曾经的 bug：整体拼接正确，但单次输出把中文字符从中间切开）。
func TestArgumentScanner_ByteAlignedChunks_NeverEmitsInvalidUTF8(t *testing.T) {
	full := `{"completeness":false,"reasonableness":true,"extracted_info":{"k":"v"},"reasoning_summary":"内部推理摘要文本","follow_up_question":"您提到的职业背景中，具体的任职时间是？请补充说明。"}`

	for chunkSize := 1; chunkSize <= 5; chunkSize++ {
		scanner := llm.NewArgumentScanner()
		parts := feedInChunksCollectingParts(scanner, full, chunkSize)

		var joined string
		for _, p := range parts {
			if !utf8.ValidString(p) {
				t.Fatalf("chunkSize=%d: emitted an invalid UTF-8 fragment: %q (bytes: %v)", chunkSize, p, []byte(p))
			}
			joined += p
		}
		assert.Truef(t, utf8.ValidString(joined), "chunkSize=%d: joined result should be valid UTF-8", chunkSize)
	}
}

// TestArgumentScanner_Concurrent2ByteChunks_ClosingQuoteRightAfterMultiByteChar 覆盖一个边界场景：
// follow_up_question 值以多字节字符结尾、紧接着闭合引号，且该多字节字符恰好被分片切断。
func TestArgumentScanner_ClosingRightAfterSplitMultiByteChar(t *testing.T) {
	full := `{"completeness":true,"reasonableness":true,"extracted_info":{},"reasoning_summary":"x","follow_up_question":"你好"}`

	scanner := llm.NewArgumentScanner()
	// "你" 的 UTF-8 编码是 E4 BD A0（3字节），故意用 2 字节的 chunk 切分，
	// 使得"你"字被拆成 [E4 BD] + [A0 ...] 两次 Feed 调用。
	got := feedInChunks(scanner, full, 2)

	assert.Equal(t, "你好", got)
}
