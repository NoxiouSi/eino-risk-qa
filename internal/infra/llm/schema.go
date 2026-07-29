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
// 注意：ParamsOneOf 底层基于 Go map 构建，序列化为 JSON Schema 时字段顺序不受此处代码书写顺序
// 保证（不同 Provider 的实际输出顺序可能不同，例如按字段名字母序排列）。因此 infra/llm 的增量
// 扫描器（见 argument_scanner.go）不依赖 follow_up_question 处于参数 JSON 中的固定位置。
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
