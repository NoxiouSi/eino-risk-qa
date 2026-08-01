<script setup lang="ts">
import { reactive, ref, computed } from 'vue'
import RiskFactorForm from './components/RiskFactorForm.vue'
import QAFormCard from './components/QAFormCard.vue'
import SessionCard from './components/SessionCard.vue'
import DebugPanel from './components/DebugPanel.vue'
import { getBatch, getMainQuestions, submitBatch, submitBatchStream, submitFollowUp, submitFollowUpStream } from './api/client'
import type { ChatBubble, MainQuestionItem, SessionResultDTO, SSEEvent } from './types'

interface SessionCardState {
  sessionId: string
  riskFactorType: string
  mainQuestion: string
  status: SessionResultDTO['status']
  extractedInfo: SessionResultDTO['extracted_info']
  // 只读历史：仅包含"已确认"的问答对与终态收尾消息，绝不包含正在生成中的内容——
  // 这正是避免"聊天区先流式展示一遍、表单又展示一遍"重复问题的关键。
  bubbles: ChatBubble[]
  // 当前应在表单中展示的问题文本：仅在拿到确定结果（processing）后才被赋值；
  // 出错时保持上一次的值不变，供用户在原问题上原地重试。
  followUpQuestion: string
  // 生成中的临时缓冲区：流式 message_delta 增量都累加在这里，
  // 生成结束前表单直接展示这个缓冲区内容（含光标动画），而不产生任何额外气泡。
  generatingText: string
  generating: boolean
  // 最近一次提交失败的错误提示，内联展示在对应表单卡片下方，不进入bubbles历史。
  errorMessage: string
}

function createCard(sessionId: string, riskFactorType: string, mainQuestion: string): SessionCardState {
  return {
    sessionId,
    riskFactorType,
    mainQuestion,
    status: 'processing',
    extractedInfo: null,
    bubbles: [],
    followUpQuestion: '',
    generatingText: '',
    generating: false,
    errorMessage: '',
  }
}

function commitRound(card: SessionCardState, questionText: string, answerText: string) {
  card.bubbles.push({ role: 'question', text: questionText })
  card.bubbles.push({ role: 'answer', text: answerText })
}

// applyResult 是同步/流式两条路径共用的"结果落地"逻辑：
// - processing：新问题写入 followUpQuestion，供表单下一轮展示（不产生气泡）。
// - cleared/not_cleared：收尾话术作为一条system气泡写入只读历史，会话退出表单。
// - error：不改动 followUpQuestion（保留上一次已知问题以便原地重试），仅记录errorMessage。
function applyResult(card: SessionCardState, result: SessionResultDTO) {
  card.generating = false
  card.status = result.status
  card.extractedInfo = result.extracted_info
  if (!card.riskFactorType) card.riskFactorType = result.risk_factor_type

  if (result.error) {
    card.errorMessage = result.error.message
    return
  }
  card.errorMessage = ''
  if (result.status === 'processing') {
    card.followUpQuestion = result.message
  } else {
    card.bubbles.push({ role: 'system', text: result.message })
    card.followUpQuestion = ''
  }
}

function handleStreamEvent(card: SessionCardState, ev: SSEEvent) {
  if (ev.type === 'message_delta') {
    card.generatingText += ev.data.content
  } else if (ev.type === 'result') {
    applyResult(card, ev.data)
  } else if (ev.type === 'error') {
    card.generating = false
    card.errorMessage = ev.data.message
    card.status = 'llm_error'
  }
  // done 事件无需特殊处理：result/error 已经落地最终状态。
}

// ---------------- 用户信息 & 主问题拉取阶段 ----------------
const userId = ref('u_1001')
const userName = ref('张三')
const stream = ref(false)

const questionsFetched = ref(false)
const loadingQuestions = ref(false)
const questionsError = ref('')
const qaItems = ref<MainQuestionItem[]>([])
const answerDrafts = reactive<Record<string, string>>({})

const allAnswered = computed(
  () => qaItems.value.length > 0 && qaItems.value.every((item) => (answerDrafts[item.risk_factor_type] || '').trim().length > 0),
)

async function handleStart() {
  questionsError.value = ''
  const id = userId.value.trim()
  if (!id) {
    questionsError.value = '请填写用户ID'
    return
  }
  loadingQuestions.value = true
  try {
    const resp = await getMainQuestions(id)
    if (!resp.items.length) {
      questionsError.value = '该用户暂未配置任何风险项'
      return
    }
    qaItems.value = resp.items
    for (const key of Object.keys(answerDrafts)) delete answerDrafts[key]
    for (const item of resp.items) answerDrafts[item.risk_factor_type] = ''
    questionsFetched.value = true
  } catch (e) {
    questionsError.value = (e as Error).message
  } finally {
    loadingQuestions.value = false
  }
}

function handleReset() {
  questionsFetched.value = false
  questionsError.value = ''
  qaItems.value = []
  for (const key of Object.keys(answerDrafts)) delete answerDrafts[key]
  for (const key of Object.keys(followUpDrafts)) delete followUpDrafts[key]
  batchSubmitted.value = false
  submitError.value = ''
  resetSessions()
  batchId.value = ''
}

// ---------------- 会话消息流状态（沿用现有并发批量提交/追问架构） ----------------
const submittingBatch = ref(false)
const submitError = ref('')
const batchSubmitted = ref(false)

const batchId = ref('')
const sessions = reactive<Record<string, SessionCardState>>({})
const sessionOrder = ref<string[]>([])

// 所有需要用户输入的追问回答统一放在此处，与主问题表单同构：
// 用户填完当前所有待回答项后一次性点击"提交回答"，并发提交给各自会话接口，
// 而非每个会话卡片各自独立提交。
const followUpDrafts = reactive<Record<string, string>>({})
const followUpSubmitting = ref(false)

const pendingSessions = computed(() =>
  sessionOrder.value.map((id) => sessions[id]).filter((s) => s.status === 'processing' || s.status === 'llm_error'),
)

const allFollowUpAnswered = computed(
  () =>
    pendingSessions.value.length > 0 &&
    pendingSessions.value.every((s) => !s.generating && (followUpDrafts[s.sessionId] || '').trim().length > 0),
)

const allSessionsDone = computed(() => sessionOrder.value.length > 0 && pendingSessions.value.length === 0)

// ---------------- 调试面板（独立展示，不影响主界面） ----------------
const showDebug = ref(false)
const debugSessions = computed(() =>
  sessionOrder.value.map((id) => ({
    sessionId: sessions[id].sessionId,
    riskFactorType: sessions[id].riskFactorType,
    status: sessions[id].status,
    extractedInfo: sessions[id].extractedInfo,
  })),
)

function resetSessions() {
  for (const key of Object.keys(sessions)) delete sessions[key]
  sessionOrder.value = []
}

// ---------------- 统一提交：回答完全部问答卡后一次性发起批量首轮提交 ----------------
async function handleSubmitAll() {
  if (!allAnswered.value || submittingBatch.value) return
  submitError.value = ''

  const payload = {
    user: { user_id: userId.value.trim(), name: userName.value.trim() || undefined },
    risk_factors: qaItems.value.map((item) => ({
      risk_factor_type: item.risk_factor_type,
      main_question: item.main_question,
      answer: answerDrafts[item.risk_factor_type].trim(),
    })),
  }

  submittingBatch.value = true
  resetSessions()
  batchId.value = ''
  batchSubmitted.value = true

  try {
    if (!stream.value) {
      const resp = await submitBatch(payload)
      batchId.value = resp.batch_id
      const results = resp.results ?? []
      for (let idx = 0; idx < results.length; idx++) {
        const r = results[idx]
        const item = qaItems.value[idx]
        const answer = answerDrafts[item.risk_factor_type].trim()
        const card = createCard(r.session_id, r.risk_factor_type, item.main_question)
        commitRound(card, item.main_question, answer)
        sessions[card.sessionId] = card
        sessionOrder.value.push(card.sessionId)
        applyResult(card, r)
      }
    } else {
      // 流式场景：session_id 由服务端在流中才揭示，且多个风险要素的事件交错到达同一条流。
      // 采用简化的"先到先认领"策略，将首次出现的 session_id 按顺序与未认领的问答卡绑定——
      // 这是调试工具的合理简化（生产场景不涉及该问题，详见 docs/DESIGN.md 流式输出设计章节）。
      const claimed = new Array(qaItems.value.length).fill(false)
      const claimNextItem = () => {
        const idx = claimed.findIndex((c) => !c)
        if (idx === -1) return qaItems.value[0]
        claimed[idx] = true
        return qaItems.value[idx]
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
          card = createCard(sessionId, item.risk_factor_type, item.main_question)
          commitRound(card, item.main_question, answerDrafts[item.risk_factor_type].trim())
          card.generating = true
          sessions[sessionId] = card
          sessionOrder.value.push(sessionId)
        }
        handleStreamEvent(card, ev)
      })
    }
  } catch (e) {
    submitError.value = (e as Error).message
  } finally {
    submittingBatch.value = false
  }
}

// ---------------- 追问回答：统一表单一次性提交，并发调用各会话的追问接口 ----------------
async function submitOneFollowUp(sessionId: string, answer: string) {
  const card = sessions[sessionId]
  if (!card) return

  commitRound(card, card.followUpQuestion, answer)
  card.generating = true
  card.generatingText = ''
  card.errorMessage = ''

  try {
    if (!stream.value) {
      const result = await submitFollowUp(sessionId, answer)
      applyResult(card, result)
    } else {
      await submitFollowUpStream(sessionId, answer, (ev) => handleStreamEvent(card, ev))
    }
  } catch (e) {
    card.generating = false
    card.errorMessage = (e as Error).message
    card.status = 'llm_error'
  }
}

async function handleSubmitAllFollowUps() {
  if (!allFollowUpAnswered.value || followUpSubmitting.value) return
  followUpSubmitting.value = true

  // 先固定本轮需要提交的目标与回答内容，再统一发起，避免提交过程中 pendingSessions
  // 重新计算导致的目标漂移。
  const targets = pendingSessions.value
    .filter((s) => !s.generating)
    .map((s) => ({
      sessionId: s.sessionId,
      answer: (followUpDrafts[s.sessionId] || '').trim(),
    }))
  for (const t of targets) delete followUpDrafts[t.sessionId]

  try {
    await Promise.all(targets.map((t) => submitOneFollowUp(t.sessionId, t.answer)))
  } finally {
    followUpSubmitting.value = false
  }
}

// ---------------- 批次查询（调试面板内，恢复调试上下文） ----------------
const queryBatchId = ref('')
const queryError = ref('')
const queryLoading = ref(false)

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
      const card = createCard(s.session_id, s.risk_factor_type, s.main_question)
      const history = s.history ?? []
      if (history.length) {
        for (const h of history) {
          card.bubbles.push({ role: 'question', text: h.question })
          card.bubbles.push({ role: 'answer', text: h.answer })
        }
      } else {
        card.bubbles.push({ role: 'question', text: s.main_question })
      }
      card.extractedInfo = s.extracted_info
      if (s.error) {
        card.errorMessage = s.error.message
        card.status = s.status
      } else if (s.status === 'processing') {
        card.followUpQuestion = s.message
        card.status = s.status
      } else {
        card.bubbles.push({ role: 'system', text: s.message })
        card.status = s.status
      }
      sessions[s.session_id] = card
      sessionOrder.value.push(s.session_id)
    }
    questionsFetched.value = true
    batchSubmitted.value = true
    showDebug.value = false
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
      <div class="page-header-row">
        <div>
          <h1>Eino Risk QA 助手</h1>
          <p class="subtitle">按用户拉取需要回答的风险问题，以表单形式统一填写并提交</p>
        </div>
        <button class="btn-debug" type="button" @click="showDebug = true">调试信息</button>
      </div>
    </header>

    <div class="chat-shell">
      <RiskFactorForm
        v-model:userId="userId"
        v-model:userName="userName"
        v-model:stream="stream"
        :loading="loadingQuestions"
        :started="questionsFetched"
        @start="handleStart"
        @reset="handleReset"
      />

      <div class="chat-scroll">
        <p v-if="questionsError" class="error-text">{{ questionsError }}</p>

        <div v-if="!questionsFetched && !loadingQuestions && !questionsError" class="hint-bubble">
          请输入用户ID并点击"开始"，获取需要回答的风险问题。
        </div>
        <div v-if="loadingQuestions" class="hint-bubble">正在拉取该用户的风险问题...</div>

        <template v-if="questionsFetched && !batchSubmitted">
          <div class="hint-bubble">请在下方填写以下 {{ qaItems.length }} 个问题的回答，全部填完后统一提交：</div>
          <QAFormCard
            v-for="item in qaItems"
            :key="item.risk_factor_type"
            :risk-factor-type="item.risk_factor_type"
            :main-question="item.main_question"
            :answer="answerDrafts[item.risk_factor_type]"
            :disabled="submittingBatch"
            @update:answer="(v) => (answerDrafts[item.risk_factor_type] = v)"
          />
          <p v-if="submitError" class="error-text">{{ submitError }}</p>
          <div class="submit-row">
            <button class="btn-primary" type="button" :disabled="!allAnswered || submittingBatch" @click="handleSubmitAll">
              {{ submittingBatch ? '提交中...' : '提交回答' }}
            </button>
          </div>
        </template>

        <template v-if="batchSubmitted">
          <div v-if="submittingBatch && !sessionOrder.length" class="hint-bubble">正在提交，请稍候…</div>

          <SessionCard
            v-for="sid in sessionOrder"
            :key="sid"
            :risk-factor-type="sessions[sid].riskFactorType"
            :bubbles="sessions[sid].bubbles"
          />

          <template v-if="pendingSessions.length">
            <div class="hint-bubble">还有 {{ pendingSessions.length }} 个问题待回答，请在下方统一填写后提交：</div>
            <QAFormCard
              v-for="s in pendingSessions"
              :key="s.sessionId"
              :risk-factor-type="s.riskFactorType"
              :main-question="s.generating ? s.generatingText : s.followUpQuestion"
              :answer="followUpDrafts[s.sessionId] || ''"
              :disabled="followUpSubmitting || s.generating"
              :generating="s.generating"
              :show-cursor="s.generating && stream"
              :error-message="s.errorMessage"
              @update:answer="(v) => (followUpDrafts[s.sessionId] = v)"
            />
            <div class="submit-row">
              <button
                class="btn-primary"
                type="button"
                :disabled="!allFollowUpAnswered || followUpSubmitting"
                @click="handleSubmitAllFollowUps"
              >
                {{ followUpSubmitting ? '提交中...' : '提交回答' }}
              </button>
            </div>
          </template>

          <div v-else-if="allSessionsDone" class="hint-bubble">全部问题已完成，感谢您的配合。</div>
        </template>
      </div>
    </div>

    <DebugPanel
      :open="showDebug"
      :batch-id="batchId"
      :sessions="debugSessions"
      v-model:query-batch-id="queryBatchId"
      :query-loading="queryLoading"
      :query-error="queryError"
      @close="showDebug = false"
      @query="handleQueryBatch"
    />
  </div>
</template>

<style scoped>
.page {
  max-width: 720px;
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
  margin-bottom: 16px;
}
.page-header-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.page-header h1 {
  font-size: 20px;
  font-weight: 600;
  margin: 0 0 4px;
}
.subtitle {
  color: #888;
  font-size: 13px;
  margin: 0;
}
.btn-debug {
  cursor: pointer;
  border: 1.5px solid #e2e6ef;
  border-radius: 20px;
  padding: 6px 16px;
  font-size: 12px;
  color: #666;
  background: #fff;
  white-space: nowrap;
  flex-shrink: 0;
}
.btn-debug:hover {
  border-color: #5d8aff;
  color: #3b6df0;
}
.chat-shell {
  display: flex;
  flex-direction: column;
  border-radius: 18px;
  background: #fff;
  box-shadow: 0 4px 24px rgba(59, 109, 240, 0.08), 0 1px 4px rgba(0, 0, 0, 0.04);
  overflow: hidden;
  border: 1px solid #eef1f7;
}
.chat-scroll {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 18px 18px 22px;
  min-height: 320px;
  max-height: 640px;
  overflow-y: auto;
  background: #fafbfd;
}
.hint-bubble {
  align-self: flex-start;
  max-width: 90%;
  background: #eef1f7;
  color: #555;
  font-size: 12.5px;
  padding: 8px 14px;
  border-radius: 12px;
  margin-bottom: 6px;
}
.error-text {
  color: #c0392b;
  font-size: 13px;
}
.submit-row {
  display: flex;
  justify-content: center;
  margin-top: 12px;
  padding-top: 12px;
}
button {
  cursor: pointer;
  border-radius: 20px;
  padding: 6px 14px;
  font-size: 13px;
  border: none;
}
.btn-primary {
  background: linear-gradient(135deg, #3b6df0, #5d8aff);
  color: #fff;
  font-weight: 600;
  padding: 10px 32px;
  box-shadow: 0 2px 8px rgba(59, 109, 240, 0.3);
  transition: transform 0.12s ease, box-shadow 0.12s ease;
}
.btn-primary:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(59, 109, 240, 0.4);
}
.btn-primary:disabled {
  background: #b9c6ec;
  box-shadow: none;
  cursor: not-allowed;
}
</style>
