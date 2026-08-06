package llm

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/NoxiouSi/eino-risk-qa/internal/domain/riskfactor"
)

const systemPromptTemplate = `你是一名风控尽调审核助手。请严格按照数据库配置的审核清单，逐个question_key判断用户提交。

风险要素类型：%s
主问题：%s

审核清单：
%s

要求：
1. items必须覆盖清单中的每个question_key，不能新增或遗漏。
2. completeness只判断该问题是否已提交足够且清晰的信息；reasonableness判断提交是否满足该问题引用的全部审核规则。
3. 图片问题必须实际查看对应图片，判断证据类型、清晰度、编辑/P图痕迹及规则要求的一致性，不能仅依据文件名。
4. 首轮缺少提交的问题 completeness=false；追问轮中，历史判断已完整（completeness=true）且本轮未重新提交的问题，必须保持历史结论不变，completeness设true，且该问题不得出现在follow_up_question中。
5. 有任一必填问题不完整时，follow_up_question应明确指出需补充哪些question的具体信息；所有必填问题均完整时follow_up_question留空。
6. extracted_info仅提取本轮可确认的信息，不输出敏感原文。
7. 【关键约束】follow_up_question生成必须严格限定在当前审核清单所列问题的范围内。禁止引入清单之外的问题，尤其禁止引入其他风险要素类型（如交易场景、资金来源等）的问题。任何未在审核清单中出现的提问方向均视为违规。

必须调用 %s 工具提交结构化结果，不要自由文本回复。`

// BuildMessages 构造动态Skill提示词和真实多模态图片输入。
func BuildMessages(input riskfactor.JudgeInput) ([]*schema.Message, error) {
	var checklist strings.Builder
	for i, question := range input.Questions {
		fmt.Fprintf(&checklist, "%d. [%s] %s（类型=%s，必填=%t，最少提交=%d，最多提交=%d）\n", i+1, question.QuestionKey, question.QuestionText, question.AnswerType, question.Required, question.MinSubmitCount, question.MaxSubmitCount)
		for j, rule := range question.Rules {
			fmt.Fprintf(&checklist, "   规则%d：%s\n", j+1, rule)
		}
	}
	if len(input.Questions) == 0 {
		checklist.WriteString("按主问题判断完整性与合理性。")
	}
	sys := fmt.Sprintf(systemPromptTemplate, input.RiskFactorType, input.MainQuestion, checklist.String(), judgementToolName)

	var text strings.Builder
	for _, qa := range input.History {
		fmt.Fprintf(&text, "第%d轮\n问题：%s\n回答摘要：%s\n", qa.Round, qa.Question, qa.Answer)
		for _, judgement := range qa.Judgements {
			fmt.Fprintf(&text, "历史判断[%s]：完整=%t，合理=%t，说明=%s\n", judgement.QuestionKey, judgement.Completeness, judgement.Reasonableness, judgement.Note)
		}
		text.WriteString("\n")
	}
	fmt.Fprintf(&text, "本轮\n问题：%s\n回答摘要：%s\n", input.CurrentQuestion, input.LatestAnswer)
	for _, answer := range input.Answers {
		if answer.Text != "" {
			fmt.Fprintf(&text, "[%s] 文本：%s\n", answer.QuestionKey, answer.Text)
		} else {
			fmt.Fprintf(&text, "[%s] 图片数量：%d\n", answer.QuestionKey, len(answer.ImagePaths))
		}
	}

	parts := []schema.MessageInputPart{{Type: schema.ChatMessagePartTypeText, Text: text.String()}}
	for _, answer := range input.Answers {
		for _, imagePath := range answer.ImagePaths {
			data, err := os.ReadFile(imagePath)
			if err != nil {
				return nil, fmt.Errorf("read image for %s: %w", answer.QuestionKey, err)
			}
			encoded := base64.StdEncoding.EncodeToString(data)
			mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(imagePath)))
			if mimeType == "" {
				mimeType = "image/jpeg"
			}
			parts = append(parts,
				schema.MessageInputPart{Type: schema.ChatMessagePartTypeText, Text: "以下图片属于问题 " + answer.QuestionKey},
				schema.MessageInputPart{Type: schema.ChatMessagePartTypeImageURL, Image: &schema.MessageInputImage{MessagePartCommon: schema.MessagePartCommon{Base64Data: &encoded, MIMEType: mimeType}, Detail: schema.ImageURLDetailHigh}},
			)
		}
	}

	user := &schema.Message{Role: schema.User, Content: text.String()}
	if len(parts) > 1 {
		user.Content = ""
		user.UserInputMultiContent = parts
	}
	return []*schema.Message{schema.SystemMessage(sys), user}, nil
}

func hasImageAnswers(input riskfactor.JudgeInput) bool {
	for _, answer := range input.Answers {
		if len(answer.ImagePaths) > 0 {
			return true
		}
	}
	return false
}
