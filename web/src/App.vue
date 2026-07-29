<script setup lang="ts">
import { reactive, ref } from 'vue'
import RiskFactorForm from './components/RiskFactorForm.vue'
import SessionCard from './components/SessionCard.vue'
import { getBatch, submitBatch, submitBatchStream, submitFollowUp, submitFollowUpStream } from './api/client'
import type { ChatBubble, RiskFactorFormItem, SessionResultDTO, SSEEvent } from './types'

interface SessionCardState {
  sessionId: string
  riskFactorType: string
  mainQuestion: string
  status: SessionResultDTO['status']
  extractedInfo: SessionResultDTO['extracted_info']
  bubbles: ChatBubble[]
  submitting: boolean
  streamingIndex: number
  answerDraft: string
}

const userId = ref('u_1001')
const userName = ref('张三')
const stream = ref(false)
const formItems = ref<RiskFactorFormItem[]>([
  { riskFactorType: 'identity', mainQuestion: '请说明您的身份信息及职业背景', answer: '' },
])
const submittingBatch = ref(false)
const formError = ref('')

const batchId = ref('')
const sessions = reactive<Record<string, SessionCardState>>({})
const sessionOrder = ref<string[]>([])

const queryBatchId = ref('')
const queryError = ref('')
const queryLoading = ref(false)

function resetSessions() {
  for (const key of Object.keys(sessions)) delete sessions[key]
  sessionOrder.value = []
}

function newCardFromResult(result: SessionResultDTO, mainQuestion: string, answer: string): SessionCardState {
  const bubbles: ChatBubble[] = [
    { role: 'question', text: mainQuestion },
    { role: 'answer', text: answer },
  ]
  bubbles.push(
    result.error ? { role: 'error', text: result.error.message } : { role: 'system', text: result.message },
  )
  return {
    sessionId: result.session_id,
    riskFactorType: result.risk_factor_type,
    mainQuestion,
    status: result.status,
    extractedInfo: result.extracted_info,
    bubbles,
    submitting: false,
    streamingIndex: -1,
    answerDraft: '',
  }
}

function ensureCard(sessionId: string, riskFactorType: string, mainQuestion: string, answer: string) {
  if (!sessions[sessionId]) {
    sessions[sessionId] = {
      sessionId,
      riskFactorType,
      mainQuestion,
      status: 'processing',
      extractedInfo: null,
      bubbles: [
        { role: 'question', text: mainQuestion },
        { role: 'answer', text: answer },
      ],
      submitting: false,
      streamingIndex: -1,
      answerDraft: '',
    }
    sessionOrder.value.push(sessionId)
  }
  return sessions[sessionId]
}

function appendBubble(card: SessionCardState, bubble: ChatBubble): number {
  card.bubbles.push(bubble)
  return card.bubbles.length - 1
}

function applyResultToCard(card: SessionCardState, result: SessionResultDTO, bubbleIndex: number) {
  card.status = result.status
  card.extractedInfo = result.extracted_info
  if (!card.riskFactorType) card.riskFactorType = result.risk_factor_type
  card.bubbles[bubbleIndex] = result.error
    ? { role: 'error', text: result.error.message }
    : { role: 'system', text: result.message }
}

// ---------------- 发起批量提交 ----------------
async function handleSubmitBatch() {
  formError.value = ''
  if (!userId.value.trim()) {
    formError.value = '请填写用户ID'
    return
  }
  for (const item of formItems.value) {
    if (!item.mainQuestion.trim() || !item.answer.trim()) {
      formError.value = '每个风险要素的主问题与回答均不能为空'
      return
    }
  }

  submittingBatch.value = true
  resetSessions()
  batchId.value = ''

  const payload = {
    user: { user_id: userId.value.trim(), name: userName.value.trim() || undefined },
    risk_factors: formItems.value.map((i) => ({
      risk_factor_type: i.riskFactorType,
      main_question: i.mainQuestion,
      answer: i.answer,
    })),
  }

  try {
    if (!stream.value) {
      const resp = await submitBatch(payload)
      batchId.value = resp.batch_id
      const results = resp.results ?? []
      for (let idx = 0; idx < results.length; idx++) {
        const r = results[idx]
        const item = formItems.value[idx]
        const card = newCardFromResult(r, item.mainQuestion, item.answer)
        sessions[card.sessionId] = card
        sessionOrder.value.push(card.sessionId)
      }
    } else {
      // 流式场景：session_id 由服务端在流中才揭示，且多个风险要素的事件交错到达同一条流。
      // 采用简化的"先到先认领"策略，将首次出现的 session_id 按顺序与未认领的表单项绑定——
      // 这是调试工具的合理简化（生产场景不涉及该问题，详见 docs/DESIGN.md 流式输出设计章节）。
      const claimed = new Array(formItems.value.length).fill(false)
      const claimNextItem = () => {
        const idx = claimed.findIndex((c) => !c)
        if (idx === -1) return formItems.value[0]
        claimed[idx] = true
        return formItems.value[idx]
      }

      await submitBatchStream(payload, (ev: SSEEvent) => {
        if (ev.type === 'batch_created') {
          batchId.value = ev.data.batch_id
          return
        }
        const sessionId = (ev.data as { session_id?: string }).session_id
        if (!sessionId) return

        let card = sessions[sessionId]
        if (!card) {
          const item = claimNextItem()
          card = ensureCard(sessionId, '', item.mainQuestion, item.answer)
          card.streamingIndex = appendBubble(card, { role: 'system', text: '' })
        }

        if (ev.type === 'message_delta') {
          card.bubbles[card.streamingIndex].text += ev.data.content
        } else if (ev.type === 'result') {
          applyResultToCard(card, ev.data, card.streamingIndex)
        } else if (ev.type === 'error') {
          card.bubbles[card.streamingIndex] = { role: 'error', text: ev.data.message }
          card.status = 'llm_error'
        } else if (ev.type === 'done') {
          card.streamingIndex = -1
        }
      })
    }
  } catch (e) {
    formError.value = (e as Error).message
  } finally {
    submittingBatch.value = false
  }
}

// ---------------- 追问回答提交（每个会话卡片独立触发） ----------------
async function handleSubmitFollowUp(sessionId: string) {
  const card = sessions[sessionId]
  if (!card) return
  const answer = card.answerDraft.trim()
  if (!answer || card.submitting) return

  appendBubble(card, { role: 'answer', text: answer })
  card.answerDraft = ''
  card.submitting = true
  const loadingIndex = appendBubble(card, { role: 'system', text: stream.value ? '' : '正在分析中……' })
  if (stream.value) card.streamingIndex = loadingIndex

  try {
    if (!stream.value) {
      const result = await submitFollowUp(sessionId, answer)
      applyResultToCard(card, result, loadingIndex)
    } else {
      await submitFollowUpStream(sessionId, answer, (ev) => {
        if (ev.type === 'message_delta') {
          card.bubbles[loadingIndex].text += ev.data.content
        } else if (ev.type === 'result') {
          applyResultToCard(card, ev.data, loadingIndex)
        } else if (ev.type === 'error') {
          card.bubbles[loadingIndex] = { role: 'error', text: ev.data.message }
          card.status = 'llm_error'
        } else if (ev.type === 'done') {
          card.streamingIndex = -1
        }
      })
    }
  } catch (e) {
    card.bubbles[loadingIndex] = { role: 'error', text: (e as Error).message }
    card.status = 'llm_error'
  } finally {
    card.submitting = false
  }
}

// ---------------- 批次查询（恢复调试上下文） ----------------
async function handleQueryBatch() {
  queryError.value = ''
  const id = queryBatchId.value.trim()
  if (!id) return
  queryLoading.value = true
  try {
    const resp = await getBatch(id)
    batchId.value = resp.batch_id
    resetSessions()
    for (const s of resp.sessions ?? []) {
      const history = s.history ?? []
      // history[0]（round=0）的 question 本身就是 main_question，避免重复展示。
      const bubbles: ChatBubble[] = history.length ? [] : [{ role: 'question', text: s.main_question }]
      for (const h of history) {
        bubbles.push({ role: 'question', text: h.question })
        bubbles.push({ role: 'answer', text: h.answer })
      }
      bubbles.push(s.error ? { role: 'error', text: s.error.message } : { role: 'system', text: s.message })
      sessions[s.session_id] = {
        sessionId: s.session_id,
        riskFactorType: s.risk_factor_type,
        mainQuestion: s.main_question,
        status: s.status,
        extractedInfo: s.extracted_info,
        bubbles,
        submitting: false,
        streamingIndex: -1,
        answerDraft: '',
      }
      sessionOrder.value.push(s.session_id)
    }
  } catch (e) {
    queryError.value = (e as Error).message
  } finally {
    queryLoading.value = false
  }
}
</script>

<template>
  <div class="page">
    <header class="page-header">
      <h1>Eino Risk QA 调试台</h1>
      <p class="subtitle">仅用于开发调试批量提交/追问问答/流式输出等后端能力，非面向业务用户的正式界面</p>
    </header>

    <RiskFactorForm
      v-model:userId="userId"
      v-model:userName="userName"
      v-model:items="formItems"
      v-model:stream="stream"
      :submitting="submittingBatch"
      @submit="handleSubmitBatch"
    />
    <p v-if="formError" class="error-text">{{ formError }}</p>

    <div v-if="batchId" class="batch-info">
      当前批次：<code>{{ batchId }}</code>
    </div>

    <section v-if="sessionOrder.length" class="session-list">
      <h2>会话列表</h2>
      <SessionCard
        v-for="sid in sessionOrder"
        :key="sid"
        :session-id="sessions[sid].sessionId"
        :risk-factor-type="sessions[sid].riskFactorType"
        :status="sessions[sid].status"
        :bubbles="sessions[sid].bubbles"
        :extracted-info="sessions[sid].extractedInfo"
        :submitting="sessions[sid].submitting"
        :streaming-index="sessions[sid].streamingIndex"
        :answer-draft="sessions[sid].answerDraft"
        @update:answer-draft="(v) => (sessions[sid].answerDraft = v)"
        @submit="handleSubmitFollowUp(sid)"
      />
    </section>

    <section class="query-panel">
      <h2>批次查询</h2>
      <div class="query-row">
        <input v-model="queryBatchId" placeholder="粘贴 batch_id 以恢复调试上下文" />
        <button class="btn-secondary" type="button" :disabled="queryLoading" @click="handleQueryBatch">
          {{ queryLoading ? '查询中...' : '查询' }}
        </button>
      </div>
      <p v-if="queryError" class="error-text">{{ queryError }}</p>
    </section>
  </div>
</template>

<style scoped>
.page {
  max-width: 860px;
  margin: 0 auto;
  padding: 24px 16px 60px;
  font-family:
    -apple-system,
    BlinkMacSystemFont,
    'Segoe UI',
    'PingFang SC',
    sans-serif;
  color: #222;
}
.page-header {
  margin-bottom: 20px;
}
.page-header h1 {
  font-size: 20px;
  margin: 0 0 4px;
}
.subtitle {
  color: #888;
  font-size: 13px;
  margin: 0;
}
.error-text {
  color: #c0392b;
  font-size: 13px;
}
.batch-info {
  margin-bottom: 16px;
  font-size: 13px;
  color: #555;
}
.session-list h2,
.query-panel h2 {
  font-size: 15px;
  margin: 0 0 10px;
}
.query-panel {
  margin-top: 30px;
  border-top: 1px dashed #ddd;
  padding-top: 16px;
}
.query-row {
  display: flex;
  gap: 8px;
}
.query-row input {
  flex: 1;
  padding: 6px 8px;
  border: 1px solid #ccc;
  border-radius: 4px;
  font-size: 13px;
}
button {
  cursor: pointer;
  border-radius: 4px;
  padding: 6px 14px;
  font-size: 13px;
  border: none;
}
.btn-secondary {
  background: #e7e7e7;
  color: #333;
}
.btn-secondary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
code {
  background: #f2f2f2;
  padding: 2px 6px;
  border-radius: 4px;
}
</style>
