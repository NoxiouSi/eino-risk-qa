package llm

import "github.com/cloudwego/eino/schema"

const judgementToolName = "submit_risk_judgement"

func judgementToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: judgementToolName,
		Desc: "按question_key提交每个问题的完整性和合理性判断，并给出缺失问题的定向追问。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"items": {
				Type: schema.Array, Desc: "逐问题判断，必须覆盖审核清单中的每个question_key", Required: true,
				ElemInfo: &schema.ParameterInfo{Type: schema.Object, SubParams: map[string]*schema.ParameterInfo{
					"question_key":   {Type: schema.String, Desc: "问题稳定标识", Required: true},
					"completeness":   {Type: schema.Boolean, Desc: "该问题是否达到必需提交数量且信息完整", Required: true},
					"reasonableness": {Type: schema.Boolean, Desc: "该问题的提交是否符合审核规则", Required: true},
					"note":           {Type: schema.String, Desc: "简要判断依据", Required: true},
				}},
			},
			"extracted_info":     {Type: schema.Object, Desc: "本轮新提取的结构化信息", Required: true},
			"reasoning_summary":  {Type: schema.String, Desc: "整体判断摘要", Required: false},
			"follow_up_question": {Type: schema.String, Desc: "存在不完整问题时的简洁定向追问，否则为空", Required: false},
		}),
	}
}
