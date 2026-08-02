// 封装批量提交/追问回答提交(含stream)/批次查询/会话查询的HTTP调用。
// 流式请求使用 fetch + ReadableStream 自行解析 SSE 文本帧（不用 EventSource，因其不支持 POST）。

import type {
  BatchRequestDTO,
  BatchResponseDTO,
  ErrorResponseDTO,
  MainQuestionsResponseDTO,
  QuestionAnswerDTO,
  AttachmentResponseDTO,
  SessionDetailDTO,
  SessionResultDTO,
  SSEEvent,
} from '../types'

const BASE = '/api/v1'
const MODEL_REQUEST_TIMEOUT_MS = 5 * 60 * 1000 + 15 * 1000

async function fetchWithTimeout(url: string, init?: RequestInit, timeoutMs = MODEL_REQUEST_TIMEOUT_MS): Promise<Response> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), timeoutMs)
  try {
    return await fetch(url, { ...init, signal: controller.signal })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new Error('模型分析超时，请稍后重试；若已生成批次，可在调试面板中查询处理结果')
    }
    throw error
  } finally {
    window.clearTimeout(timer)
  }
}

async function parseJSONOrThrow<T>(res: Response): Promise<T> {
  const text = await res.text()
  let json: unknown = null
  try {
    json = text ? JSON.parse(text) : null
  } catch {
    // ignore, fall through to error below
  }
  if (!res.ok) {
    const err = json as ErrorResponseDTO | null
    throw new Error(err?.message ?? `HTTP ${res.status}`)
  }
  return json as T
}

function parseMainQuestionsResponse(value: unknown): MainQuestionsResponseDTO {
  if (!value || typeof value !== 'object') {
    throw new TypeError('问题接口响应格式错误')
  }

  const response = value as { user_id?: unknown; items?: unknown }
  if (typeof response.user_id !== 'string' || !Array.isArray(response.items)) {
    throw new TypeError('问题接口响应格式错误')
  }
  for (const item of response.items) {
    if (!item || typeof item !== 'object') {
      throw new TypeError('问题接口响应格式错误')
    }
    if (!Array.isArray((item as { questions?: unknown }).questions)) {
      throw new TypeError('问题接口响应格式不兼容：缺少 items[].questions，请重启后端服务后重试')
    }
  }
  return value as MainQuestionsResponseDTO
}

/** 按用户查询其拥有的风险项及各自对应的主问题（GET /api/v1/users/{user_id}/main-questions）。 */
export async function getMainQuestions(userId: string): Promise<MainQuestionsResponseDTO> {
  const res = await fetch(`${BASE}/users/${encodeURIComponent(userId)}/main-questions`)
  return parseMainQuestionsResponse(await parseJSONOrThrow<unknown>(res))
}

export async function submitBatch(body: Omit<BatchRequestDTO, 'stream'>): Promise<BatchResponseDTO> {
  const res = await fetchWithTimeout(`${BASE}/batches`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...body, stream: false }),
  })
  return parseJSONOrThrow<BatchResponseDTO>(res)
}

export async function submitFollowUp(sessionId: string, answers: QuestionAnswerDTO[]): Promise<SessionResultDTO> {
  const res = await fetchWithTimeout(`${BASE}/sessions/${encodeURIComponent(sessionId)}/answers`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ answers, stream: false }),
  })
  return parseJSONOrThrow<SessionResultDTO>(res)
}

export async function uploadAttachment(userId: string, riskFactorType: string, questionKey: string, file: File): Promise<AttachmentResponseDTO> {
  const form = new FormData()
  form.append('user_id', userId)
  form.append('risk_factor_type', riskFactorType)
  form.append('question_key', questionKey)
  form.append('file', file)
  const res = await fetch(`${BASE}/attachments`, { method: 'POST', body: form })
  return parseJSONOrThrow<AttachmentResponseDTO>(res)
}

export async function getBatch(batchId: string): Promise<BatchResponseDTO> {
  const res = await fetch(`${BASE}/batches/${encodeURIComponent(batchId)}`)
  return parseJSONOrThrow<BatchResponseDTO>(res)
}

export async function getSession(sessionId: string): Promise<SessionDetailDTO> {
  const res = await fetch(`${BASE}/sessions/${encodeURIComponent(sessionId)}`)
  return parseJSONOrThrow<SessionDetailDTO>(res)
}

/**
 * 解析一段 SSE 文本协议缓冲区中已完整到达的事件帧（以空行分隔），
 * 返回已解析出的事件列表与缓冲区中尚未消费完的剩余文本（不完整的最后一帧留给下一次继续拼接）。
 */
function parseSSEChunk(buffer: string): { events: SSEEvent[]; rest: string } {
  const events: SSEEvent[] = []
  const frames = buffer.split('\n\n')
  // 最后一个元素可能是尚未收到结尾空行的不完整帧，留到下次继续拼接。
  const rest = frames.pop() ?? ''

  for (const frame of frames) {
    let eventName = ''
    const dataLines: string[] = []
    for (const line of frame.split('\n')) {
      if (line.startsWith('event:')) {
        eventName = line.slice('event:'.length).trim()
      } else if (line.startsWith('data:')) {
        dataLines.push(line.slice('data:'.length).trim())
      }
    }
    if (!eventName || dataLines.length === 0) continue
    try {
      const data = JSON.parse(dataLines.join('\n'))
      events.push({ type: eventName, data } as SSEEvent)
    } catch {
      // 忽略无法解析的帧
    }
  }
  return { events, rest }
}

async function streamBodyOrThrow(res: Response): Promise<ReadableStream<Uint8Array>> {
  if (res.ok && res.body) return res.body
  const errText = await res.text().catch(() => '')
  let message = `HTTP ${res.status}`
  try {
    const parsed = JSON.parse(errText) as ErrorResponseDTO
    if (parsed?.message) message = parsed.message
  } catch {
    /* ignore */
  }
  throw new Error(message)
}

async function consumeSSEStream(body: ReadableStream<Uint8Array>, onEvent: (event: SSEEvent) => void): Promise<void> {
  const reader = body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buffer = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const parsed = parseSSEChunk(buffer)
    buffer = parsed.rest
    for (const event of parsed.events) onEvent(event)
  }
  if (!buffer.trim()) return
  for (const event of parseSSEChunk(buffer + '\n\n').events) onEvent(event)
}

/** 通用的流式 POST 请求 + SSE 帧解析，通过 onEvent 回调逐帧上报。 */
async function streamPost(url: string, body: unknown, onEvent: (event: SSEEvent) => void): Promise<void> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), MODEL_REQUEST_TIMEOUT_MS)
  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal: controller.signal,
    })
    await consumeSSEStream(await streamBodyOrThrow(res), onEvent)
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new Error('模型分析超时，请稍后重试；若已生成批次，可在调试面板中查询处理结果')
    }
    throw error
  } finally {
    window.clearTimeout(timer)
  }
}

export function submitBatchStream(
  body: Omit<BatchRequestDTO, 'stream'>,
  onEvent: (event: SSEEvent) => void,
): Promise<void> {
  return streamPost(`${BASE}/batches`, { ...body, stream: true }, onEvent)
}

export function submitFollowUpStream(
  sessionId: string,
  answers: QuestionAnswerDTO[],
  onEvent: (event: SSEEvent) => void,
): Promise<void> {
  return streamPost(
    `${BASE}/sessions/${encodeURIComponent(sessionId)}/answers`,
    { answers, stream: true },
    onEvent,
  )
}
