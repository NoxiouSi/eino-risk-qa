package llm

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// MockChatModel 是一个固定规则的 model.ToolCallingChatModel 实现，不依赖真实网络/API Key，
// 专供本地开发与自动化测试使用（configs 中 llm.provider=mock 时启用）。
//
// 判定规则（简单启发式，仅用于打通链路/演示，非真实语义理解）：
//   - 若用户最新回答的字符长度 >= MinAnswerLength（默认 20），视为 completeness=true；
//   - 若回答中包含 UnreasonableKeywords 中的任意关键词，视为 reasonableness=false，否则为 true；
//   - completeness=false 时，生成一个基于主问题的固定追问文本。
type MockChatModel struct {
	MinAnswerLength      int
	UnreasonableKeywords []string
	tools                []*schema.ToolInfo
}

// NewMockChatModel 创建一个带默认规则的 MockChatModel。
func NewMockChatModel() *MockChatModel {
	return &MockChatModel{
		MinAnswerLength:      20,
		UnreasonableKeywords: []string{"不知道", "随便", "无法说明"},
	}
}

var _ model.ToolCallingChatModel = (*MockChatModel)(nil)

// WithTools 返回绑定了给定工具列表的新实例（不修改原实例，符合 ToolCallingChatModel 约定）。
func (m *MockChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	clone := *m
	clone.tools = tools
	return &clone, nil
}

// Generate 一次性返回完整的工具调用结果。
func (m *MockChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	args := m.decide(input)
	b, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{
				ID:   "mock-call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      judgementToolName,
					Arguments: string(b),
				},
			},
		},
	}, nil
}

// Stream 以固定的小片段方式流式返回工具调用参数字符串，模拟真实 Provider 的逐字生成行为，
// 便于验证 infra/llm 层的增量 JSON 扫描逻辑。
func (m *MockChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	args := m.decide(input)
	b, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	full := string(b)

	sr, sw := schema.Pipe[*schema.Message](8)
	go func() {
		defer sw.Close()
		const chunkSize = 3
		for i := 0; i < len(full); i += chunkSize {
			end := i + chunkSize
			if end > len(full) {
				end = len(full)
			}
			index := 0
			msg := &schema.Message{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{
					{
						Index: &index,
						ID:    "mock-call-1",
						Type:  "function",
						Function: schema.FunctionCall{
							Name:      judgementToolName,
							Arguments: full[i:end],
						},
					},
				},
			}
			if closed := sw.Send(msg, nil); closed {
				return
			}
		}
	}()
	return sr, nil
}

// judgementArgs 是 mock 决策结果的 JSON 结构，字段顺序与 schema.go 中的约定保持一致
// （follow_up_question 放最后，便于验证增量扫描器）。
type itemJudgementArgs struct {
	QuestionKey    string `json:"question_key"`
	Completeness   bool   `json:"completeness"`
	Reasonableness bool   `json:"reasonableness"`
	Note           string `json:"note"`
}

type judgementArgs struct {
	Items            []itemJudgementArgs    `json:"items,omitempty"`
	Completeness     bool                   `json:"completeness,omitempty"`
	Reasonableness   bool                   `json:"reasonableness,omitempty"`
	ExtractedInfo    map[string]interface{} `json:"extracted_info"`
	ReasoningSummary string                 `json:"reasoning_summary"`
	FollowUpQuestion string                 `json:"follow_up_question"`
}

// decide 从输入消息中提取"本轮回答"文本（BuildMessages 拼装的最后一条 user 消息中的"回答："部分），
// 应用固定规则给出判断结果。
func (m *MockChatModel) decide(input []*schema.Message) judgementArgs {
	answer := extractLatestAnswer(input)

	completeness := len([]rune(answer)) >= m.MinAnswerLength
	reasonableness := true
	for _, kw := range m.UnreasonableKeywords {
		if strings.Contains(answer, kw) {
			reasonableness = false
			break
		}
	}

	args := judgementArgs{
		Completeness:     completeness,
		Reasonableness:   reasonableness,
		ExtractedInfo:    map[string]interface{}{"raw_answer_length": len([]rune(answer))},
		ReasoningSummary: "mock provider: 基于回答长度与关键词的固定规则判断",
	}
	for _, key := range extractQuestionKeys(input) {
		args.Items = append(args.Items, itemJudgementArgs{QuestionKey: key, Completeness: completeness, Reasonableness: reasonableness, Note: "mock provider固定规则判断"})
	}
	if !completeness {
		args.FollowUpQuestion = "请提供更详细的信息以便完成核实。"
	}
	return args
}

// extractLatestAnswer 从 user 消息内容中解析出"本轮\n问题：...\n回答：...\n"里的回答文本。
func extractLatestAnswer(input []*schema.Message) string {
	var userContent string
	for _, msg := range input {
		if msg.Role == schema.User {
			userContent = msg.Content
			for _, part := range msg.UserInputMultiContent {
				if part.Type == schema.ChatMessagePartTypeText {
					userContent += part.Text
				}
			}
		}
	}
	marker := "回答："
	idx := strings.LastIndex(userContent, marker)
	if idx == -1 {
		marker = "回答摘要："
		idx = strings.LastIndex(userContent, marker)
	}
	if idx == -1 {
		return ""
	}
	rest := userContent[idx+len(marker):]

	rest = strings.TrimRight(rest, "\n")
	return rest
}

var questionKeyPattern = regexp.MustCompile(`\[([a-z0-9_]+)\]`)

func extractQuestionKeys(input []*schema.Message) []string {
	seen := map[string]bool{}
	var result []string
	for _, msg := range input {
		if msg.Role != schema.System {
			continue
		}
		for _, match := range questionKeyPattern.FindAllStringSubmatch(msg.Content, -1) {
			if len(match) == 2 && !seen[match[1]] {
				seen[match[1]] = true
				result = append(result, match[1])
			}
		}
	}
	return result
}
