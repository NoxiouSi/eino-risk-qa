<script setup lang="ts">
import type { ChatBubble, SessionStatus } from '../types'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{
  sessionId: string
  riskFactorType: string
  status: SessionStatus
  bubbles: ChatBubble[]
  extractedInfo: Record<string, unknown> | null | undefined
  submitting: boolean
  streamingIndex: number
  answerDraft: string
}>()

const emit = defineEmits<{
  'update:answerDraft': [string]
  submit: []
}>()

function onEnter(e: KeyboardEvent) {
  e.preventDefault()
  emit('submit')
}
</script>

<template>
  <div class="session-card">
    <div class="card-header">
      <strong>{{ riskFactorType || '识别中...' }}</strong>
      <span class="session-id">{{ sessionId }}</span>
      <StatusBadge :status="status" />
    </div>

    <div class="chat-area">
      <div v-for="(bubble, idx) in bubbles" :key="idx" :class="['bubble', `bubble-${bubble.role}`]">
        {{ bubble.text }}
        <span v-if="idx === streamingIndex" class="cursor">▍</span>
      </div>
    </div>

    <div v-if="extractedInfo && Object.keys(extractedInfo).length" class="extracted-info">
      <div class="extracted-title">已提取信息</div>
      <pre>{{ JSON.stringify(extractedInfo, null, 2) }}</pre>
    </div>

    <div v-if="status === 'processing' && !submitting" class="answer-input-area">
      <textarea
        :value="answerDraft"
        rows="2"
        placeholder="请输入追问回答..."
        @input="emit('update:answerDraft', ($event.target as HTMLTextAreaElement).value)"
        @keydown.enter.exact="onEnter"
      />
      <button class="btn-primary" type="button" @click="emit('submit')">发送</button>
    </div>
    <div v-else-if="status === 'llm_error'" class="answer-input-area">
      <textarea
        :value="answerDraft"
        rows="2"
        placeholder="出错后可重新提交本轮回答重试..."
        @input="emit('update:answerDraft', ($event.target as HTMLTextAreaElement).value)"
      />
      <button class="btn-primary" type="button" @click="emit('submit')">重试</button>
    </div>
    <div v-else-if="submitting" class="ended-hint">处理中...</div>
    <div v-else class="ended-hint">对话已结束</div>
  </div>
</template>

<style scoped>
.session-card {
  border: 1px solid #e2e2e2;
  border-radius: 8px;
  padding: 14px;
  margin-bottom: 16px;
  background: #fff;
}
.card-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.session-id {
  color: #999;
  font-size: 12px;
  font-family: monospace;
}
.chat-area {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 10px;
}
.bubble {
  max-width: 80%;
  padding: 8px 12px;
  border-radius: 10px;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
.bubble-question {
  background: #eef1f7;
  color: #333;
  align-self: flex-start;
  border-bottom-left-radius: 2px;
}
.bubble-answer {
  background: #3b6df0;
  color: #fff;
  align-self: flex-end;
  border-bottom-right-radius: 2px;
}
.bubble-system {
  background: #f2f2f2;
  color: #333;
  align-self: flex-start;
  border-bottom-left-radius: 2px;
}
.bubble-error {
  background: #fde2e1;
  color: #c0392b;
  align-self: flex-start;
}
.cursor {
  animation: blink 1s steps(1) infinite;
}
@keyframes blink {
  50% {
    opacity: 0;
  }
}
.extracted-info {
  background: #fbfbf0;
  border: 1px dashed #ddd;
  border-radius: 6px;
  padding: 6px 10px;
  margin-bottom: 10px;
  font-size: 12px;
}
.extracted-title {
  font-weight: 600;
  margin-bottom: 4px;
  color: #777;
}
pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
}
.answer-input-area {
  display: flex;
  gap: 8px;
}
.answer-input-area textarea {
  flex: 1;
  padding: 6px 8px;
  border: 1px solid #ccc;
  border-radius: 4px;
  font-size: 13px;
  font-family: inherit;
  resize: vertical;
}
.ended-hint {
  text-align: center;
  color: #aaa;
  font-size: 12px;
  padding: 6px 0;
}
button {
  cursor: pointer;
  border-radius: 4px;
  padding: 6px 14px;
  font-size: 13px;
  border: none;
}
.btn-primary {
  background: #3b6df0;
  color: #fff;
}
</style>
