<script setup lang="ts">
import StatusBadge from './StatusBadge.vue'
import type { SessionStatus } from '../types'

// DebugPanel 独立承载所有"调试相关"信息（session_id、原始status、已提取字段JSON、
// 批次查询工具），以浮层抽屉形式展示，不嵌入主聊天/表单界面，避免技术细节干扰
// 主流程的问答体验。
export interface DebugSessionInfo {
  sessionId: string
  riskFactorType: string
  status: SessionStatus
  extractedInfo: Record<string, unknown> | null | undefined
}

const props = defineProps<{
  open: boolean
  batchId: string
  sessions: DebugSessionInfo[]
  queryBatchId: string
  queryLoading: boolean
  queryError: string
}>()

const emit = defineEmits<{
  close: []
  'update:queryBatchId': [string]
  query: []
}>()
</script>

<template>
  <Transition name="fade">
    <div v-if="props.open" class="debug-overlay" @click.self="emit('close')">
      <aside class="debug-panel">
        <div class="debug-header">
          <h2>调试信息</h2>
          <button class="btn-close" type="button" @click="emit('close')">×</button>
        </div>

        <div class="debug-body">
          <div class="debug-block">
            <div class="debug-label">批次 ID</div>
            <code v-if="props.batchId">{{ props.batchId }}</code>
            <span v-else class="debug-muted">尚未提交批次</span>
          </div>

          <div class="debug-block">
            <div class="debug-label">会话原始状态 / 已提取字段</div>
            <div v-if="!props.sessions.length" class="debug-muted">暂无会话数据</div>
            <div v-for="s in props.sessions" :key="s.sessionId" class="debug-session">
              <div class="debug-session-header">
                <code class="debug-session-id">{{ s.sessionId }}</code>
                <StatusBadge :status="s.status" />
              </div>
              <div class="debug-field">风险要素类型：{{ s.riskFactorType }}</div>
              <pre
                v-if="s.extractedInfo && Object.keys(s.extractedInfo).length"
                class="debug-json"
              >{{ JSON.stringify(s.extractedInfo, null, 2) }}</pre>
              <div v-else class="debug-field debug-muted">尚未提取到信息</div>
            </div>
          </div>

          <div class="debug-block">
            <div class="debug-label">批次查询</div>
            <div class="query-row">
              <input
                :value="props.queryBatchId"
                placeholder="粘贴 batch_id 以恢复调试上下文"
                @input="emit('update:queryBatchId', ($event.target as HTMLInputElement).value)"
              />
              <button class="btn-secondary" type="button" :disabled="props.queryLoading" @click="emit('query')">
                {{ props.queryLoading ? '查询中...' : '查询' }}
              </button>
            </div>
            <p v-if="props.queryError" class="error-text">{{ props.queryError }}</p>
          </div>
        </div>
      </aside>
    </div>
  </Transition>
</template>

<style scoped>
.debug-overlay {
  position: fixed;
  inset: 0;
  background: rgba(20, 26, 40, 0.32);
  display: flex;
  justify-content: flex-end;
  z-index: 100;
}
.debug-panel {
  width: 360px;
  max-width: 92vw;
  height: 100%;
  background: #fff;
  box-shadow: -8px 0 24px rgba(0, 0, 0, 0.12);
  display: flex;
  flex-direction: column;
}
.debug-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 18px;
  border-bottom: 1px solid #eef1f7;
}
.debug-header h2 {
  font-size: 14px;
  font-weight: 600;
  margin: 0;
  color: #333;
}
.btn-close {
  cursor: pointer;
  border: none;
  background: transparent;
  font-size: 20px;
  line-height: 1;
  color: #999;
  padding: 0 4px;
}
.btn-close:hover {
  color: #555;
}
.debug-body {
  flex: 1;
  overflow-y: auto;
  padding: 16px 18px 28px;
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.debug-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.debug-label {
  font-size: 12px;
  font-weight: 600;
  color: #999;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}
.debug-muted {
  color: #bbb;
  font-size: 12.5px;
}
.debug-session {
  border: 1px dashed #e6e9f0;
  border-radius: 10px;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.debug-session-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.debug-session-id {
  font-size: 11px;
  color: #888;
}
.debug-field {
  font-size: 12.5px;
  color: #555;
}
.debug-json {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  background: #fbfbf0;
  border: 1px dashed #e6e0b8;
  border-radius: 8px;
  padding: 8px 10px;
  color: #6b5d1f;
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
.btn-secondary {
  cursor: pointer;
  border: none;
  border-radius: 4px;
  padding: 6px 14px;
  font-size: 13px;
  background: #e7e7e7;
  color: #333;
}
.btn-secondary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.error-text {
  color: #c0392b;
  font-size: 13px;
  margin: 0;
}
code {
  background: #f2f2f2;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
  word-break: break-all;
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
