<script setup lang="ts">
import { reactive, ref, computed, onUnmounted } from 'vue'
import RiskFactorForm from './components/RiskFactorForm.vue'
import QAFormCard from './components/QAFormCard.vue'
import SessionCard from './components/SessionCard.vue'
import DebugPanel from './components/DebugPanel.vue'
import { getBatch, getMainQuestions, submitBatch, submitBatchStream, submitFollowUp, submitFollowUpStream } from './api/client'
import { riskFactorTypeLabel } from './types'
import type { ChatBubble, MainQuestionItem, QuestionAnswerDTO, QuestionDraft, QuestionDraftMap, QuestionItem, QuestionJudgementDTO, SessionDetailDTO, SessionResultDTO, SSEEvent } from './types'

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
  questions: QuestionItem[]
  missingQuestionKeys: string[]
  questionJudgements: QuestionJudgementDTO[]
}

function createCard(sessionId: string, riskFactorType: string, mainQuestion: string, questions: QuestionItem[] = []): SessionCardState {
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
    questions,
    missingQuestionKeys: questions.map((question) => question.question_key),
    questionJudgements: [],
  }
}

function commitRound(card: SessionCardState, questionText: string, answerText: string) {
  card.bubbles = [...card.bubbles, { role: 'question', text: questionText }, { role: 'answer', text: answerText }]
}

// applyResult 是同步/流式两条路径共用的结果落地逻辑：
// - processing：记录需要补充资料的追问，供统一表单展示。
// - cleared/not_cleared：仅记录该风险要素已结束；批次收尾由 allSessionsDone 统一展示。
// - error：保留上一次已知问题以便原地重试，仅记录 errorMessage。
function applyResult(card: SessionCardState, result: SessionResultDTO) {
  card.generating = false
  card.status = result.status
  card.extractedInfo = result.extracted_info
  card.missingQuestionKeys = result.missing_question_keys ?? []
  card.questionJudgements = result.question_judgements ?? []
  if (!card.riskFactorType) card.riskFactorType = result.risk_factor_type

  if (result.error) {
    card.errorMessage = result.error.message
    return
  }
  card.errorMessage = ''
  if (result.status === 'processing') {
    card.followUpQuestion = result.message
  } else {
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
const stream = ref(true)

const questionsFetched = ref(false)
const loadingQuestions = ref(false)
const questionsError = ref('')
const qaItems = ref<MainQuestionItem[]>([])
const answerDrafts = reactive<Record<string, QuestionDraftMap>>({})

function createDraftMap(questions: QuestionItem[]): QuestionDraftMap {
  return Object.fromEntries(questions.map((question) => [question.question_key, { text: '', fileIds: [], fileNames: [] }]))
}
function draftValid(question: QuestionItem, draft?: QuestionDraft): boolean {
  if (draft?.uploading) return false
  if (question.answer_type === 'text') {
    return !question.required || Boolean(draft?.text.trim())
  }
  const count = draft?.fileIds.length ?? 0
  if (count === 0) return !question.required
  return count >= question.min_submit_count && count <= question.max_submit_count
}
function toAnswers(questions: QuestionItem[], drafts: QuestionDraftMap): QuestionAnswerDTO[] {
  const answers: QuestionAnswerDTO[] = []
  for (const question of questions) {
    const draft = drafts[question.question_key]
    if (!draft) continue
    if (question.answer_type === 'text' && draft.text.trim()) answers.push({ question_key: question.question_key, text: draft.text.trim() })
    if (question.answer_type !== 'text' && draft.fileIds.length) answers.push({ question_key: question.question_key, file_ids: draft.fileIds })
  }
  return answers
}
function summarizeDrafts(questions: QuestionItem[], drafts: QuestionDraftMap): string {
  return questions.map((question) => question.answer_type === 'text' ? `${question.question_text}：${drafts[question.question_key]?.text ?? ''}` : `${question.question_text}：已提交${drafts[question.question_key]?.fileIds.length ?? 0}个文件`).join('\n')
}
const allAnswered = computed(() => qaItems.value.length > 0 && qaItems.value.every((item) => item.questions.every((question) => draftValid(question, answerDrafts[item.risk_factor_type]?.[question.question_key]))))

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
    for (const item of resp.items) answerDrafts[item.risk_factor_type] = createDraftMap(item.questions)
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
const submissionElapsedSeconds = ref(0)
let submissionTimer: number | undefined

function startSubmissionTimer() {
  submissionElapsedSeconds.value = 0
  if (submissionTimer !== undefined) window.clearInterval(submissionTimer)
  submissionTimer = window.setInterval(() => submissionElapsedSeconds.value++, 1000)
}

function stopSubmissionTimer() {
  if (submissionTimer !== undefined) window.clearInterval(submissionTimer)
  submissionTimer = undefined
}

onUnmounted(stopSubmissionTimer)

const batchId = ref('')
const sessions = reactive<Record<string, SessionCardState>>({})
const sessionOrder = ref<string[]>([])
// 批次创建时后端返回的 session_id → qaItem 映射。
// 卡片不立即创建，等 result/error 事件到达时才按需构建，实现逐条揭示。
const sessionItemMap = new Map<string, MainQuestionItem>()

// 所有需要用户输入的追问回答统一放在此处，与主问题表单同构：
// 用户填完当前所有待回答项后一次性点击"提交回答"，并发提交给各自会话接口，
// 而非每个会话卡片各自独立提交。
const followUpDrafts = reactive<Record<string, QuestionDraftMap>>({})
const followUpSubmitting = ref(false)

const pendingSessions = computed(() =>
  sessionOrder.value.map((id) => sessions[id]).filter((s) => s.status === 'processing' || s.status === 'llm_error'),
)
function pendingQuestions(session: SessionCardState): QuestionItem[] {
  const missing = new Set(session.missingQuestionKeys)
  return session.questions.filter((question) => missing.size === 0 || missing.has(question.question_key))
}
const pendingSessionsAnalyzing = computed(() => pendingSessions.value.some((session) => session.generating))
const anyResultArrived = computed(() => sessionOrder.value.length > 0)
const completedSessionLabels = computed(() => {
  const hasConfirmedSupplementRequest = pendingSessions.value.some((session) => session.status === 'processing' && !session.generating)
  if (!hasConfirmedSupplementRequest) return []
  return sessionOrder.value
    .map((id) => sessions[id])
    .filter((session) => session.status === 'cleared' || session.status === 'not_cleared')
    .map((session) => riskFactorTypeLabel(session.riskFactorType))
})
const allFollowUpAnswered = computed(() => pendingSessions.value.length > 0 && pendingSessions.value.every((session) => {
  const questions = pendingQuestions(session)
  return !session.generating && questions.length > 0 && questions.every((question) => draftValid(question, followUpDrafts[session.sessionId]?.[question.question_key]))
}))

const allSessionsDone = computed(() =>
  !submittingBatch.value
  && qaItems.value.length > 0
  && sessionOrder.value.length === qaItems.value.length
  && pendingSessions.value.length === 0,
)

// ---------------- 调试面板（独立展示，不影响主界面） ----------------
const showDebug = ref(false)
const debugSessions = computed(() =>
  sessionOrder.value.map((id) => ({
    sessionId: sessions[id].sessionId,
    riskFactorType: sessions[id].riskFactorType,
    status: sessions[id].status,
    extractedInfo: sessions[id].extractedInfo,
    missingQuestionKeys: sessions[id].missingQuestionKeys,
    questionJudgements: sessions[id].questionJudgements,
  })),
)

function resetSessions() {
  for (const key of Object.keys(sessions)) delete sessions[key]
  sessionOrder.value = []
  sessionItemMap.clear()
}

// ---------------- 统一提交：回答完全部问答卡后一次性发起批量首轮提交 ----------------
async function handleSubmitAll() {
  if (!allAnswered.value || submittingBatch.value) return
  submitError.value = ''

  const payload = {
    user: { user_id: userId.value.trim(), name: userName.value.trim() || undefined },
    risk_factors: qaItems.value.map((item) => ({ risk_factor_type: item.risk_factor_type, answers: toAnswers(item.questions, answerDrafts[item.risk_factor_type]) })),
  }

  submittingBatch.value = true
  startSubmissionTimer()
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
        const answer = summarizeDrafts(item.questions, answerDrafts[item.risk_factor_type])
        const card = createCard(r.session_id, r.risk_factor_type, item.main_question, item.questions)
        commitRound(card, item.main_question, answer)
        sessions[card.sessionId] = card
        sessionOrder.value.push(card.sessionId)
        applyResult(card, r)
      }
    } else {
      // 流式场景：batch_created 仅存储 session_id → qaItem 映射，不立即创建卡片。
      // 当某个风险要素的 result/error 事件到达时才创建该卡片，实现"逐条揭示"效果。
      await submitBatchStream(payload, (ev: SSEEvent) => {
        if (ev.type === 'batch_created') {
          batchId.value = ev.data.batch_id
          for (const s of ev.data.sessions) {
            const item = qaItems.value.find((i) => i.risk_factor_type === s.risk_factor_type)
            if (item) sessionItemMap.set(s.session_id, item)
          }
          return
        }
        const sessionId = (ev.data as { session_id?: string }).session_id
        if (!sessionId) return

        let card = sessions[sessionId]
        if (!card) {
          // 仅 result/error 事件触发卡片创建（message_delta 等被跳过）
          if (ev.type !== 'result' && ev.type !== 'error') return
          const item = sessionItemMap.get(sessionId)
          if (!item) return
          card = createCard(sessionId, item.risk_factor_type, item.main_question, item.questions)
          commitRound(card, item.main_question, summarizeDrafts(item.questions, answerDrafts[item.risk_factor_type]))
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
    stopSubmissionTimer()
  }
}

// ---------------- 追问回答：统一表单一次性提交，并发调用各会话的追问接口 ----------------
async function submitOneFollowUp(sessionId: string, drafts: QuestionDraftMap) {
  const card = sessions[sessionId]
  if (!card) return
  const questions = pendingQuestions(card)
  const answers = toAnswers(questions, drafts)
  commitRound(card, card.followUpQuestion, summarizeDrafts(questions, drafts))
  card.generating = true
  card.generatingText = ''
  card.errorMessage = ''

  try {
    if (!stream.value) {
      const result = await submitFollowUp(sessionId, answers)
      applyResult(card, result)
    } else {
      await submitFollowUpStream(sessionId, answers, (ev) => handleStreamEvent(card, ev))
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
      drafts: followUpDrafts[s.sessionId] ?? createDraftMap(pendingQuestions(s)),
    }))
  for (const t of targets) delete followUpDrafts[t.sessionId]

  try {
    await Promise.all(targets.map((t) => submitOneFollowUp(t.sessionId, t.drafts)))
  } finally {
    followUpSubmitting.value = false
  }
}

// ---------------- 批次查询（调试面板内，恢复调试上下文） ----------------
const queryBatchId = ref('')
const queryError = ref('')
const queryLoading = ref(false)

function restoreSessionCard(session: SessionDetailDTO): SessionCardState {
  const card = createCard(session.session_id, session.risk_factor_type, session.main_question, session.questions)
  const history = session.history ?? []
  if (history.length) {
    for (const item of history) {
      card.bubbles.push({ role: 'question', text: item.question }, { role: 'answer', text: item.answer })
    }
  } else {
    card.bubbles.push({ role: 'question', text: session.main_question })
  }
  card.extractedInfo = session.extracted_info
  card.status = session.status
  card.missingQuestionKeys = session.missing_question_keys ?? []
  card.questionJudgements = session.question_judgements ?? []
  if (session.error) card.errorMessage = session.error.message
  else if (session.status === 'processing') card.followUpQuestion = session.message
  return card
}

async function handleQueryBatch() {
  queryError.value = ''
  const id = queryBatchId.value.trim()
  if (!id) return
  queryLoading.value = true
  try {
    const resp = await getBatch(id)
    const restoredSessions = resp.sessions ?? []
    if (!restoredSessions.length) throw new Error('该批次没有可恢复的会话')
    if (restoredSessions.some((session) => !Array.isArray(session.questions) || !session.questions.length)) {
      throw new Error('批次响应缺少问题配置，请确认后端已更新后重试')
    }

    userId.value = resp.user_id
    userName.value = resp.user_name
    batchId.value = resp.batch_id
    qaItems.value = restoredSessions.map((session) => ({
      risk_factor_type: session.risk_factor_type,
      main_question: session.main_question,
      questions: session.questions,
    }))
    for (const key of Object.keys(answerDrafts)) delete answerDrafts[key]
    for (const key of Object.keys(followUpDrafts)) delete followUpDrafts[key]
    resetSessions()

    for (const session of restoredSessions) {
      const card = restoreSessionCard(session)
      sessions[session.session_id] = card
      sessionOrder.value.push(session.session_id)
      if (session.status === 'processing' || session.status === 'llm_error') {
        followUpDrafts[session.session_id] = createDraftMap(pendingQuestions(card))
      }
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
            :user-id="userId"
            :main-question="item.main_question"
            :questions="item.questions"
            :drafts="answerDrafts[item.risk_factor_type]"
            :disabled="submittingBatch"
            @update:drafts="(v) => (answerDrafts[item.risk_factor_type] = v)"
          />
          <p v-if="submitError" class="error-text">{{ submitError }}</p>
          <div class="submit-row">
            <button class="btn-primary" type="button" :disabled="!allAnswered || submittingBatch" @click="handleSubmitAll">
              {{ submittingBatch ? '提交中...' : '提交回答' }}
            </button>
          </div>
        </template>

        <template v-if="batchSubmitted">
          <div v-if="!anyResultArrived" class="hint-bubble">
            请稍等，正在分析您提交的资料……
          </div>

          <SessionCard
            v-for="sid in sessionOrder"
            :key="sid"
            :risk-factor-type="sessions[sid].riskFactorType"
            :bubbles="sessions[sid].bubbles"
          />

          <template v-if="pendingSessions.length">
            <div v-if="completedSessionLabels.length" class="hint-bubble">
              {{ completedSessionLabels.join('、') }}已完成，无需继续补充。
            </div>
            <div class="hint-bubble">
              审核后发现仍有 {{ pendingSessions.length }} 个风险要素需要补充资料，请填写下方缺失内容后统一提交：
            </div>
            <QAFormCard
              v-for="s in pendingSessions"
              :key="s.sessionId"
              :user-id="userId"
              :risk-factor-type="s.riskFactorType"
              :main-question="s.generating ? s.generatingText : s.followUpQuestion"
              :questions="pendingQuestions(s)"
              :drafts="followUpDrafts[s.sessionId] ?? createDraftMap(pendingQuestions(s))"
              :disabled="followUpSubmitting || s.generating"
              :generating="s.generating"
              :show-cursor="s.generating && stream"
              :error-message="s.errorMessage"
              @update:drafts="(v) => (followUpDrafts[s.sessionId] = v)"
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

          <div v-else-if="allSessionsDone" class="hint-bubble">审核结果将在3个工作日内推送给您。</div>
        </template>
      </div>
    </div>

    <DebugPanel
      :open="showDebug"
      v-model:stream="stream"
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
