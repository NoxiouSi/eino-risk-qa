export type RiskFactorType = 'identity' | 'fund_source' | 'transaction_scene'
export type SessionStatus = 'processing' | 'cleared' | 'not_cleared' | 'llm_error'

export interface ErrorPayload { error_code: string; message: string }

export interface QuestionItem {
  question_key: string
  question_text: string
  answer_type: 'text' | 'image' | 'file'
  required: boolean
  min_submit_count: number
  sort_order: number
}

export interface MainQuestionItem {
  risk_factor_type: string
  main_question: string
  questions: QuestionItem[]
}

export interface MainQuestionsResponseDTO { user_id: string; items: MainQuestionItem[] }

export const RISK_FACTOR_TYPE_LABELS: Record<string, string> = {
  identity: '身份信息',
  fund_source: '资金来源',
  transaction_scene: '交易场景',
}
export function riskFactorTypeLabel(type: string): string { return RISK_FACTOR_TYPE_LABELS[type] ?? type }

export interface QuestionAnswerDTO {
  question_key: string
  text?: string
  file_ids?: string[]
}
export interface RiskFactorItemDTO { risk_factor_type: string; answers: QuestionAnswerDTO[] }
export interface BatchRequestDTO {
  user: { user_id: string; name?: string }
  risk_factors: RiskFactorItemDTO[]
  stream: boolean
}

export interface QuestionJudgementDTO {
  question_key: string
  completeness: boolean
  reasonableness: boolean
  note: string
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
  missing_question_keys: string[]
  question_judgements: QuestionJudgementDTO[]
  error?: ErrorPayload | null
}

export interface QAPairDTO {
  round: number
  question: string
  answer: string
  completeness: boolean
  reasonableness: boolean
  question_judgements: QuestionJudgementDTO[]
}
export interface SessionDetailDTO extends SessionResultDTO {
  main_question: string
  questions: QuestionItem[]
  max_rounds: number
  history?: QAPairDTO[]
}
export interface BatchResponseDTO {
  batch_id: string
  user_id: string
  user_name: string
  created_at: string
  results?: SessionResultDTO[]
  sessions?: SessionDetailDTO[]
}
export interface ErrorResponseDTO { error_code: string; message: string; request_id?: string }

export interface AttachmentResponseDTO {
  file_id: string
  original_name: string
  mime_type: string
  size_bytes: number
}

export interface QuestionDraft {
  text: string
  fileIds: string[]
  fileNames: string[]
  uploading?: boolean
  error?: string
}
export type QuestionDraftMap = Record<string, QuestionDraft>

export interface SSEBatchCreatedPayload { batch_id: string }
export interface SSEMessageDeltaPayload { session_id: string; content: string }
export interface SSEResultPayload extends SessionResultDTO {}
export interface SSEDonePayload { session_id: string }
export interface SSEErrorPayload { session_id: string; error_code: string; message: string }
export type SSEEvent =
  | { type: 'batch_created'; data: SSEBatchCreatedPayload }
  | { type: 'message_delta'; data: SSEMessageDeltaPayload }
  | { type: 'result'; data: SSEResultPayload }
  | { type: 'done'; data: SSEDonePayload }
  | { type: 'error'; data: SSEErrorPayload }

export type BubbleRole = 'question' | 'answer' | 'system' | 'error'
export interface ChatBubble { role: BubbleRole; text: string }
