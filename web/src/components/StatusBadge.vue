<script setup lang="ts">
import type { SessionStatus } from '../types'

const props = defineProps<{ status: SessionStatus }>()

const labelMap: Record<SessionStatus, string> = {
  processing: '处理中',
  cleared: '已排除怀疑',
  not_cleared: '未排除怀疑',
  llm_error: 'LLM调用出错',
}

const classMap: Record<SessionStatus, string> = {
  processing: 'badge badge-processing',
  cleared: 'badge badge-cleared',
  not_cleared: 'badge badge-not-cleared',
  llm_error: 'badge badge-error',
}
</script>

<template>
  <span :class="classMap[props.status]">{{ labelMap[props.status] }}</span>
</template>

<style scoped>
.badge {
  display: inline-block;
  padding: 2px 10px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 600;
  color: #fff;
  white-space: nowrap;
}
.badge-processing {
  background-color: #f0a020;
}
.badge-cleared {
  background-color: #2e9e5b;
}
.badge-not-cleared {
  background-color: #d9534f;
}
.badge-error {
  background-color: #888;
}
</style>
