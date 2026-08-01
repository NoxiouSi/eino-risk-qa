// 与后端 DTO 对应的 TS 类型定义（详见 docs/DESIGN.md 的 API 接口文档与 SSE 事件帧说明）。

export type RiskFactorType = 'identity' | 'fund_source'

export type SessionStatus = 'processing' | 'cleared' | 'not_cleared' | 'llm_error'

export interface ErrorPayload {
  error_code: string
  message: string
}

// 与 GET /api/v1/users/{user_id}/main-questions 响应对应的类型。
export interface MainQuestionItem {
  risk_factor_type: string
  main_question: string
}

export interface MainQuestionsResponseDTO {
  user_id: string
  items: MainQuestionItem[]
}

// 风险要素类型的可读标签，用于聊天流中的标签 chip 展示。
export const RISK_FACTOR_TYPE_LABELS: Record<string, string> = {
  identity: '身份信息',
  fund_source: '资金来源',
}

export function riskFactorTypeLabel(type: string): string {
  return RISK_FACTOR_TYPE_LABELS[type] ?? type
}

export interface RiskFactorItemDTO {
  risk_factor_type: string
  main_question: string
  answer: string
}

export interface BatchRequestDTO {
  user: { user_id: string; name?: string }
  risk_factors: RiskFactorItemDTO[]
  stream: boolean
}

export interface SessionResultDTO {
  session_id: string
  risk_factor_type: string
  status: SessionStatus
  current_round: number
  message: string
  cleared: boolean | null
  termination_reason: string | null
  extracted_info: Record<string, unknown> | null
  error?: ErrorPayload | null
}

export interface QAPairDTO {
  round: number
  question: string
  answer: string
  completeness: boolean
  reasonableness: boolean
}

export interface SessionDetailDTO extends SessionResultDTO {
  main_question: string
  max_rounds: number
  history?: QAPairDTO[]
}

export interface BatchResponseDTO {
  batch_id: string
  created_at: string
  results?: SessionResultDTO[]
  sessions?: SessionDetailDTO[]
}

export interface ErrorResponseDTO {
  error_code: string
  message: string
  request_id?: string
}

// SSE 事件负载类型
export interface SSEBatchCreatedPayload {
  batch_id: string
}
export interface SSEMessageDeltaPayload {
  session_id: string
  content: string
}
export interface SSEResultPayload extends SessionResultDTO {}
export interface SSEDonePayload {
  session_id: string
}
export interface SSEErrorPayload {
  session_id: string
  error_code: string
  message: string
}

export type SSEEvent =
  | { type: 'batch_created'; data: SSEBatchCreatedPayload }
  | { type: 'message_delta'; data: SSEMessageDeltaPayload }
  | { type: 'result'; data: SSEResultPayload }
  | { type: 'done'; data: SSEDonePayload }
  | { type: 'error'; data: SSEErrorPayload }

// 统一聊天流中的消息气泡角色：
// question(主问题/追问问题) / answer(用户回答) / system(模型统一收敛消息) / error(出错提示)
export type BubbleRole = 'question' | 'answer' | 'system' | 'error'

export interface ChatBubble {
  role: BubbleRole
  text: string
}
