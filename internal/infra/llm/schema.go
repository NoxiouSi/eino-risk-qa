package llm

import "github.com/cloudwego/eino/schema"

// judgementToolName 结构化输出所绑定的工具（Function Calling）名称。
const judgementToolName = "submit_risk_judgement"

// judgementToolInfo 定义结构化输出的参数 Schema：
//
//	completeness      bool    完整性
//	reasonableness    bool    合理性
//	extracted_info    object  本轮提取到的结构化信息（开放 map，字段依风险要素类型而异）
//	reasoning_summary string  判断依据摘要（审计用）
//	follow_up_question string 追问问题
//
// 字段顺序约定：follow_up_question 放在参数 JSON 的最后一个字段。多数 Provider（含 OpenAI 兼容协议）
// 会按 Schema 声明顺序生成参数 JSON 的字段，这样当该字段开始输出时，其余结构化字段已经确定，
// 简化 infra/llm 增量扫描器的实现（见 stream_adapter.go）。
func judgementToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: judgementToolName,
		Desc: "提交本轮针对用户回答的风险要素判断结果，包括完整性、合理性、提取到的结构化信息，以及（若信息不完整）需要追问的问题。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"completeness": {
				Type:     schema.Boolean,
				Desc:     "本轮回答（含历史累积信息）是否已完整覆盖主问题涉及的所有信息点，无遗漏",
				Required: true,
			},
			"reasonableness": {
				Type:     schema.Boolean,
				Desc:     "回答内容本身是否合理、无矛盾、无可疑之处",
				Required: true,
			},
			"extracted_info": {
				Type:     schema.Object,
				Desc:     "从本轮回答中新提取到的结构化信息，字段名依风险要素类型而定",
				Required: true,
			},
			"reasoning_summary": {
				Type:     schema.String,
				Desc:     "本轮判断依据的简要摘要，供审计使用",
				Required: false,
			},
			"follow_up_question": {
				Type:     schema.String,
				Desc:     "若 completeness 为 false，需要向用户提出的追问问题；completeness 为 true 时应为空字符串",
				Required: false,
			},
		}),
	}
}
