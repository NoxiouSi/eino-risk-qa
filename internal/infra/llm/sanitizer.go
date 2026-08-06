package llm

import (
	"regexp"
	"strings"
)

// ────────────────────────── 输入净化（Prompt Injection 防护） ──────────────────────────

const maxInputLength = 10000 // 单段用户输入最大字符数

// inputDangerousPatterns 匹配已知 prompt injection 攻击模式。
var inputDangerousPatterns = []*regexp.Regexp{
	// 角色切换 / 指令覆盖
	regexp.MustCompile(`(?i)(ignore|disregard|forget)\s+(all\s+)?(previous|above|prior|earlier)\s+(instructions?|prompts?|rules?|constraints?|requirements?)`),
	regexp.MustCompile(`(?i)(you\s+are\s+now|you\s+are\s+no\s+longer|you\s+become|act\s+as|pretend\s+(to\s+be|you\s+are)|roleplay\s+as)`),
	regexp.MustCompile(`(?i)(system\s*prompt|system\s*message|internal\s+instructions?|hidden\s+rules?)`),
	regexp.MustCompile(`(?i)(new\s+instructions?|new\s+rules?|updated\s+guidelines?|revised\s+prompt)`),
	regexp.MustCompile(`(?i)(override|overwrite|replace)\s+(the\s+)?(system|instructions?|rules?|prompt)`),

	// XML 标签逃逸
	regexp.MustCompile(`</input_section\s*>|<input_section\s*[^>]*>`),

	// 工具调用伪造
	regexp.MustCompile(`(?i)(submit_risk_judgement|judgementTool)\s*[\({]`),

	// 越狱通用模式
	regexp.MustCompile(`(?i)(DAN\s*mode|jailbreak|developer\s*mode|god\s*mode)`),
	regexp.MustCompile(`(?i)(respond\s+in\s+(chinese|english)\s+only|output\s+only|reply\s+with\s+only)`),

	// 中文 / 中英混合注入攻击模式（L0 快速拦截，补充 LLM 判别器）
	regexp.MustCompile(`(忽略|忘记|无视|清除|覆盖|替换|\u5220\u9664)\s*(之前|上面|上述|前面|原先|所有|全部|以往的?|先前的?)(的?\s*)?(指令|规则|要求|提示|限制|约束|条件|说明|指引|系统消息|系统提示)`),
	regexp.MustCompile(`(你\s*(现在|已经|不再|可以|应该)\s*(是|变成|成为|作为|扮演|假装|模仿|化身为?)|从现在开始你\s*(是|不是))`),
	regexp.MustCompile(`(不要\s*(再)?\s*(遵守|执行|按照|遵循|理会|服从|听从)|别\s*(再)?\s*(管|遵守|执行|按照))`),
	regexp.MustCompile(`(输出|回复|回答)\s*(只|仅|只能|只需|必须|一定要?|请?)\s*(用|使用|以|说|写|讲)`),
	regexp.MustCompile(`(放弃\s*(审查|审核|判断|检测|验证)|(审查|审核|判断|检测|验证)\s*(不用|不需要|跳过|不要).*了)`),
	regexp.MustCompile(`(重新\s*(定义|设定|指定|设置)\s*(规则|指令|角色|身份)|不\s*用\s*管\s*(规则|限制))`),
	regexp.MustCompile(`(?i)(ignore|disregard|forget|skip|bypass|override)\p{Han}*[\s\p{Han}]*(审查|审核|判断|规则|限制)`),
	regexp.MustCompile(`\p{Han}*[\s\p{Han}]*(system\s*prompt|internal\s*instructions?|hidden\s*rules?)\p{Han}*`),
}

// SanitizeInput 对用户输入文本做安全净化：
//  1. 截断超长文本。
//  2. 抹除已知 prompt injection 攻击模式。
//  3. 抹除 NUL 字节等控制字符（CWE-158）。
//
// 返回值：净化后文本 + 是否有可疑内容被移除。
func SanitizeInput(input string) (cleaned string, suspicious bool) {
	if input == "" {
		return input, false
	}

	// 1. 长度截断
	if len(input) > maxInputLength {
		input = input[:maxInputLength]
	}

	// 2. 移除 NUL 及其他危险控制字符（保留常见换行/制表）
	input = strings.Map(func(r rune) rune {
		if r == 0 || (r < 0x20 && r != '\n' && r != '\r' && r != '\t') {
			return -1 // 丢弃
		}
		return r
	}, input)

	// 3. 匹配并替换已知攻击模式
	hadSuspicious := false
	for _, pattern := range inputDangerousPatterns {
		if pattern.MatchString(input) {
			hadSuspicious = true
			input = pattern.ReplaceAllString(input, "[redacted]")
		}
	}

	return input, hadSuspicious
}

// ────────────────────────── 输出脱敏（数据输出安全） ──────────────────────────

var (
	// 中国居民身份证号（18位）
	chineseIDPattern = regexp.MustCompile(`\b[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`)
	// 银行卡号（16-19位数字，含空格）
	bankCardPattern = regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{0,3}\b`)
	// 中国大陆手机号
	phonePattern = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	// 邮箱地址
	emailPattern = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
	// 固定电话（带区号）
	landlinePattern = regexp.MustCompile(`\b0\d{2,3}[-\s]?\d{7,8}\b`)
	// IPv4 地址
	ipPattern = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	// 统一社会信用代码（18位）
	usccPattern = regexp.MustCompile(`\b[0-9A-HJ-NPQRTUWXY]{2}\d{6}[0-9A-HJ-NPQRTUWXY]{10}\b`)
)

// replacement func 生成脱敏占位符，保留格式线索。
var replacement = map[*regexp.Regexp]string{
	chineseIDPattern: "**[身份证号已脱敏]**",
	bankCardPattern:  "**[银行卡号已脱敏]**",
	phonePattern:     "**[手机号已脱敏]**",
	emailPattern:     "**[邮箱已脱敏]**",
	landlinePattern:  "**[电话号码已脱敏]**",
	ipPattern:        "**[IP地址已脱敏]**",
	usccPattern:      "**[统一社会信用代码已脱敏]**",
}

// DesensitizeText 对文本中的敏感信息做脱敏处理，返回脱敏后文本。
func DesensitizeText(text string) string {
	if text == "" {
		return text
	}
	result := text
	for pattern, placeholder := range replacement {
		result = pattern.ReplaceAllString(result, placeholder)
	}
	return result
}

// DesensitizeExtractedInfo 对 extracted_info 中的所有字符串值做脱敏。
func DesensitizeExtractedInfo(info map[string]interface{}) map[string]interface{} {
	if info == nil {
		return nil
	}
	desensitized := make(map[string]interface{}, len(info))
	for k, v := range info {
		switch val := v.(type) {
		case string:
			desensitized[k] = DesensitizeText(val)
		default:
			desensitized[k] = val
		}
	}
	return desensitized
}

// SanitizeFollowUpQuestion 对追问文本做安全审查和脱敏：
//  1. 脱敏敏感数据。
//  2. 截断超长追问（防止滥发内容）。
//  3. 若追问包含可疑注入模式，替换为安全兜底文本。
func SanitizeFollowUpQuestion(followUp string) string {
	if followUp == "" {
		return followUp
	}

	cleaned, suspicious := SanitizeInput(followUp)
	if suspicious {
		// 追问文本中出现注入模式说明输出异常，回退安全兜底
		return "请根据审核要求补充相关信息。"
	}

	// 追问文本截断
	if len(cleaned) > 2000 {
		cleaned = cleaned[:2000] + "…"
	}

	// 脱敏
	return DesensitizeText(cleaned)
}

// SanitizeStreamDelta 对流式输出增量做安全审查：
//  若 delta 中包含注入敏感模式，返回空字符串以拦截输出；
//  否则返回原 delta。
func SanitizeStreamDelta(delta string) string {
	if delta == "" {
		return delta
	}
	_, suspicious := SanitizeInput(delta)
	if suspicious {
		return ""
	}
	// 对流式增量做脱敏（逐段递进时也尽量保护敏感信息）
	return DesensitizeText(delta)
}
