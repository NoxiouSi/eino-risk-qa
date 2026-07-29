package llm

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// followUpQuestionKeyPattern 匹配结构化输出参数 JSON 中 follow_up_question 字段的
// "键 + 冒号 + 值起始引号"，直到匹配到值的开始引号为止（分组结束位置即 valueStart）。
//
// 不同 Provider/序列化实现输出的 JSON 空白风格并不一致：有的是紧凑格式 `"key":"value"`
// （如本项目 MockChatModel），有的会在冒号后带一个空格 `"key": "value"`（如 DeepSeek 官方 API
// 实测输出）。因此这里必须用可容忍任意数量空白字符的正则，而非固定字符串匹配，
// 否则在冒号后带空格的 Provider 上会导致该标记永远匹配不上、增量输出完全失效。
var followUpQuestionKeyPattern = regexp.MustCompile(`"follow_up_question"\s*:\s*"`)

// ArgumentScanner 是一个增量 JSON 扫描器：随着工具调用参数字符串（arguments）不断增长的分片被喂入，
// 一旦识别出 follow_up_question 字段的值已经开始输出，就能持续产出该字段值的"新增部分"，
// 用于在 follow_up_question 完整生成之前就将其逐字转发给用户。
//
// UTF-8 安全性：上游分片（无论是真实 Provider 的 SSE 分片，还是本项目 MockChatModel 的固定字节
// 分片）都可能在多字节字符（如中文，UTF-8 编码为3字节）内部断开。若直接按字节切分并输出，
// 会产生不完整、非法的 UTF-8 字节序列（JSON 序列化时会被替换为 U+FFFD）。因此 Feed 只会输出
// "以完整字符结尾"的安全前缀，任何尾部不完整的多字节字符会被暂存（pendingRune），拼接到下一次
// Feed 的开头再尝试输出，从不向调用方吐出截断的字符。
//
// 简化说明（适用于本项目场景，非通用 JSON 流式解析器）：
//   - 不依赖 follow_up_question 在参数 JSON 中的具体字段顺序——由于 schema.go 中
//     ParamsOneOf 基于 Go map 构建、序列化时字段顺序不受代码书写顺序保证（实测各 Provider
//     可能按字段名字母序等规则重排），因此本扫描器通过正则匹配该字段的键名+冒号+起始引号，
//     无论它出现在参数 JSON 的哪个位置都能正确识别；
//   - 不处理字段值内出现转义引号（\"）的极端情况——若模型输出中出现转义字符，字符串会在
//     遇到该转义引号处被误判为提前结束，这是一个已知的简化边界，可在未来演进为完整的
//     JSON tokenizer（如使用 encoding/json.Decoder 的 Token() 流式接口）。
type ArgumentScanner struct {
	buffer      strings.Builder
	emittedLen  int
	pendingRune string // 上次输出后暂存的、尚不构成完整字符的尾部字节
	started     bool
	closed      bool
}

// NewArgumentScanner 创建一个新的扫描器实例。
func NewArgumentScanner() *ArgumentScanner {
	return &ArgumentScanner{}
}

// Feed 喂入新到达的参数字符串分片，返回本次新识别出的、UTF-8 安全的 follow_up_question 增量文本
// （若字段尚未开始输出，或本次未产生新的可安全输出内容，返回空字符串）。
func (s *ArgumentScanner) Feed(chunk string) string {
	// 即使字段值已闭合（closed），仍需继续写入 buffer，以保证 FullArguments() 能返回
	// 完整的参数字符串（包含闭合引号之后的剩余 JSON，如末尾的 "}"）。
	s.buffer.WriteString(chunk)
	if s.closed {
		return ""
	}
	full := s.buffer.String()

	loc := followUpQuestionKeyPattern.FindStringIndex(full)
	if loc == nil {
		return ""
	}
	s.started = true
	valueStart := loc[1]
	rest := full[valueStart:]

	closeIdx := strings.IndexByte(rest, '"')
	var available string
	justClosed := false
	if closeIdx == -1 {
		available = rest
	} else {
		available = rest[:closeIdx]
		s.closed = true
		justClosed = true
	}

	if len(available) <= s.emittedLen {
		if justClosed && s.pendingRune != "" {
			// 字段已闭合，但仍有暂存的尾部字节未输出：不再等待更多数据补全，直接吐出（兜底）。
			out := s.pendingRune
			s.pendingRune = ""
			return out
		}
		return ""
	}

	rawDelta := available[s.emittedLen:]
	s.emittedLen = len(available)
	combined := s.pendingRune + rawDelta

	if justClosed {
		// 字段值已经结束，没有更多字节会到来，不再保留任何 pending，全部输出。
		s.pendingRune = ""
		return combined
	}

	safe, pending := splitAtLastCompleteRune(combined)
	s.pendingRune = pending
	return safe
}

// splitAtLastCompleteRune 将 s 从末尾最长的"不完整 UTF-8 字符"处截断，返回
// (可安全输出的前缀, 暂存待下次拼接的尾部不完整字节)。若 s 本身已是合法 UTF-8，
// pending 为空字符串。
func splitAtLastCompleteRune(s string) (safe, pending string) {
	if s == "" || utf8.ValidString(s) {
		return s, ""
	}
	// UTF-8 编码的字符最多4字节，只需检查末尾最多3个字节是否构成了被截断的多字节序列起始。
	maxCheck := 3
	if len(s) < maxCheck {
		maxCheck = len(s)
	}
	for cut := 1; cut <= maxCheck; cut++ {
		candidate := s[:len(s)-cut]
		leadByte := s[len(s)-cut]
		if leadByte >= 0xC0 && utf8.ValidString(candidate) {
			// leadByte 是多字节序列的起始字节（0xC0-0xF7），说明 [len(s)-cut:] 是一个
			// 尚未凑齐的多字节字符，candidate 部分已确认合法，可安全输出。
			return candidate, s[len(s)-cut:]
		}
	}
	// 未找到清晰的截断点（可能是真正非法的字节，而非"不完整"），不做特殊处理，原样输出。
	return s, ""
}

// Started 返回 follow_up_question 字段是否已经开始被输出。
func (s *ArgumentScanner) Started() bool {
	return s.started
}

// FullArguments 返回目前累积到的完整参数字符串（供最终整体反序列化使用）。
func (s *ArgumentScanner) FullArguments() string {
	return s.buffer.String()
}
