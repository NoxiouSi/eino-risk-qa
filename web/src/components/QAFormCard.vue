<script setup lang="ts">
import { riskFactorTypeLabel } from '../types'

const props = defineProps<{
  riskFactorType: string
  mainQuestion: string
  answer: string
  disabled: boolean
  // generating: 该问题当前正处于后端生成中（本轮请求尚未结束），此时mainQuestion可能为空
  // 或仍在逐字增长；仅用于展示"正在生成"占位与禁用输入，不代表一定在流式返回。
  generating?: boolean
  // showCursor: 仅在流式返回且仍在增量接收中才为true，用于展示打字光标动画。
  showCursor?: boolean
  // errorMessage: 上一次提交失败时的错误提示，内联展示在问题气泡下方（不进入聊天历史）。
  errorMessage?: string
}>()

const emit = defineEmits<{
  'update:answer': [string]
}>()
</script>

<template>
  <div class="qa-card">
    <div class="qa-question-row">
      <span class="qa-tag">{{ riskFactorTypeLabel(props.riskFactorType) }}</span>
      <div class="qa-bubble qa-bubble-bot">
        <span v-if="props.generating && !props.mainQuestion" class="qa-placeholder">正在生成问题…</span>
        <template v-else>{{ props.mainQuestion }}</template>
        <span v-if="props.showCursor" class="qa-cursor">▍</span>
      </div>
    </div>
    <p v-if="props.errorMessage" class="qa-error">{{ props.errorMessage }}，请重新提交</p>
    <div class="qa-answer-row">
      <textarea
        class="qa-answer-input"
        :value="props.answer"
        :disabled="props.disabled || props.generating"
        rows="2"
        :placeholder="props.generating ? '问题生成完毕后即可填写...' : '请输入您的回答...'"
        @input="emit('update:answer', ($event.target as HTMLTextAreaElement).value)"
      />
    </div>
  </div>
</template>

<style scoped>
.qa-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 4px 0;
}
.qa-question-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.qa-tag {
  flex-shrink: 0;
  margin-top: 8px;
  font-size: 11px;
  font-weight: 600;
  color: #3b6df0;
  background: linear-gradient(135deg, #eef1f7, #e4ebfd);
  padding: 3px 9px;
  border-radius: 10px;
  white-space: nowrap;
}
.qa-bubble {
  max-width: 78%;
  padding: 10px 14px;
  border-radius: 14px;
  font-size: 13px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
}
.qa-bubble-bot {
  background: #eef1f7;
  color: #222;
  border-bottom-left-radius: 4px;
}
.qa-placeholder {
  color: #999;
  font-style: italic;
}
.qa-error {
  margin: 0;
  padding-left: calc(58px + 8px);
  font-size: 12px;
  color: #c0392b;
}
.qa-cursor {
  animation: blink 1s steps(1) infinite;
}
@keyframes blink {
  50% {
    opacity: 0;
  }
}
.qa-answer-row {
  display: flex;
  justify-content: flex-end;
}
.qa-answer-input {
  width: 78%;
  box-sizing: border-box;
  padding: 10px 14px;
  border: 1.5px solid #dbe3f5;
  border-radius: 14px;
  border-bottom-right-radius: 4px;
  font-size: 13px;
  font-family: inherit;
  resize: vertical;
  transition: border-color 0.15s ease;
  background: #fff;
}
.qa-answer-input:focus {
  outline: none;
  border-color: #5d8aff;
}
.qa-answer-input:disabled {
  background: #f7f8fa;
  color: #999;
}
</style>
