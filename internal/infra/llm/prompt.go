package llm

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

const systemPromptTemplate = `你是一名风控尽调助手，负责判断用户针对某个风险要素问题的回答是否已经排除合理怀疑。

风险要素类型：%s
主问题：%s

请针对用户到目前为止的全部回答，分别判断：
1. completeness（完整性）：回答（含历史累积信息）是否已完整覆盖主问题涉及的所有信息点，无遗漏。
2. reasonableness（合理性）：回答内容本身是否合理、无矛盾、无可疑之处。
3. 如果 completeness 为 false，请针对缺失的信息点生成一个简洁、具体的追问问题（follow_up_question）；如果 completeness 为 true，则 follow_up_question 留空。
4. 提取本轮回答中新出现的结构化信息（extracted_info），字段名请使用简洁的英文 snake_case。

你必须调用 %s 工具来提交结构化的判断结果，不要以自由文本回复。`

// BuildMessages 拼装本轮判断所需的完整对话历史：系统提示词 + 历史问答（不含本轮）+ 本轮问题与回答。
func BuildMessages(input riskfactor.JudgeInput) []*schema.Message {
	sys := fmt.Sprintf(systemPromptTemplate, input.RiskFactorType, input.MainQuestion, judgementToolName)

	var b strings.Builder
	for _, qa := range input.History {
		fmt.Fprintf(&b, "第%d轮\n问题：%s\n回答：%s\n\n", qa.Round, qa.Question, qa.Answer)
	}
	fmt.Fprintf(&b, "本轮\n问题：%s\n回答：%s\n", input.CurrentQuestion, input.LatestAnswer)

	return []*schema.Message{
		schema.SystemMessage(sys),
		schema.UserMessage(b.String()),
	}
}
