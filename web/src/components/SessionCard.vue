<script setup lang="ts">
import { riskFactorTypeLabel } from '../types'
import type { ChatBubble } from '../types'

// SessionCard 现在只承担"只读对话历史展示"职责：渲染某个风险要素会话已经"确认完成"
// 的问答历史（不包含正在生成中的内容——生成过程直接展示在下方的统一表单卡片里，
// 避免"聊天区先流式播一遍、表单又展示一遍"的重复问题）。
const props = defineProps<{
  riskFactorType: string
  bubbles: ChatBubble[]
}>()
</script>

<template>
  <div class="session-group">
    <div class="group-header">
      <span class="risk-tag">{{ riskFactorTypeLabel(props.riskFactorType) || '识别中...' }}</span>
    </div>

    <div v-for="(bubble, idx) in props.bubbles" :key="idx" :class="['bubble-row', `bubble-row-${bubble.role}`]">
      <div :class="['bubble', `bubble-${bubble.role}`]">
        {{ bubble.text }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.session-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 6px 0 14px;
}
.group-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 2px;
}
.risk-tag {
  font-size: 11px;
  font-weight: 600;
  color: #3b6df0;
  background: linear-gradient(135deg, #eef1f7, #e4ebfd);
  padding: 3px 9px;
  border-radius: 10px;
}
.bubble-row {
  display: flex;
}
.bubble-row-question,
.bubble-row-system,
.bubble-row-error {
  justify-content: flex-start;
}
.bubble-row-answer {
  justify-content: flex-end;
}
.bubble {
  max-width: 78%;
  padding: 10px 14px;
  border-radius: 14px;
  font-size: 13px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
}
.bubble-question {
  background: #eef1f7;
  color: #222;
  border-bottom-left-radius: 4px;
}
.bubble-answer {
  background: linear-gradient(135deg, #3b6df0, #5d8aff);
  color: #fff;
  border-bottom-right-radius: 4px;
}
.bubble-system {
  background: #eef1f7;
  color: #222;
  border-bottom-left-radius: 4px;
}
.bubble-error {
  background: #fde2e1;
  color: #c0392b;
  border-bottom-left-radius: 4px;
}
</style>
