// 封装批量提交/追问回答提交(含stream)/批次查询/会话查询的HTTP调用。
// 流式请求使用 fetch + ReadableStream 自行解析 SSE 文本帧（不用 EventSource，因其不支持 POST）。

import type {
  BatchRequestDTO,
  BatchResponseDTO,
  ErrorResponseDTO,
  MainQuestionsResponseDTO,
  SessionDetailDTO,
  SessionResultDTO,
  SSEEvent,
} from '../types'

const BASE = '/api/v1'

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

/** 按用户查询其拥有的风险项及各自对应的主问题（GET /api/v1/users/{user_id}/main-questions）。 */
export async function getMainQuestions(userId: string): Promise<MainQuestionsResponseDTO> {
  const res = await fetch(`${BASE}/users/${encodeURIComponent(userId)}/main-questions`)
  return parseJSONOrThrow<MainQuestionsResponseDTO>(res)
}

export async function submitBatch(body: Omit<BatchRequestDTO, 'stream'>): Promise<BatchResponseDTO> {
  const res = await fetch(`${BASE}/batches`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...body, stream: false }),
  })
  return parseJSONOrThrow<BatchResponseDTO>(res)
}

export async function submitFollowUp(sessionId: string, answer: string): Promise<SessionResultDTO> {
  const res = await fetch(`${BASE}/sessions/${encodeURIComponent(sessionId)}/answers`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ answer, stream: false }),
  })
  return parseJSONOrThrow<SessionResultDTO>(res)
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

/** 通用的流式 POST 请求 + SSE 帧解析，通过 onEvent 回调逐帧上报。 */
async function streamPost(url: string, body: unknown, onEvent: (event: SSEEvent) => void): Promise<void> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok || !res.body) {
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

  const reader = res.body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buffer = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const { events, rest } = parseSSEChunk(buffer)
    buffer = rest
    for (const ev of events) onEvent(ev)
  }
  // flush 尾部残留（服务端应始终以空行收尾，这里做兜底）
  if (buffer.trim()) {
    const { events } = parseSSEChunk(buffer + '\n\n')
    for (const ev of events) onEvent(ev)
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
  answer: string,
  onEvent: (event: SSEEvent) => void,
): Promise<void> {
  return streamPost(
    `${BASE}/sessions/${encodeURIComponent(sessionId)}/answers`,
    { answer, stream: true },
    onEvent,
  )
}
