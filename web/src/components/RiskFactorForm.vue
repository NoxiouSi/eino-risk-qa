<script setup lang="ts">
const props = defineProps<{
  userId: string
  userName: string
  loading: boolean
  started: boolean
}>()

const emit = defineEmits<{
  'update:userId': [string]
  'update:userName': [string]
  start: []
  reset: []
}>()
</script>

<template>
  <div class="user-bar">
    <template v-if="!props.started">
      <input
        class="user-input"
        :value="props.userId"
        placeholder="请输入用户ID，如 u_1001"
        :disabled="props.loading"
        @input="emit('update:userId', ($event.target as HTMLInputElement).value)"
        @keydown.enter="emit('start')"
      />
      <input
        class="user-input user-input-name"
        :value="props.userName"
        placeholder="姓名（可选）"
        :disabled="props.loading"
        @input="emit('update:userName', ($event.target as HTMLInputElement).value)"
        @keydown.enter="emit('start')"
      />
      <button class="btn-start" type="button" :disabled="props.loading" @click="emit('start')">
        {{ props.loading ? '加载中...' : '开始' }}
      </button>
    </template>
    <template v-else>
      <div class="user-summary">
        当前用户：<strong>{{ props.userName || props.userId }}</strong>
        <span class="user-summary-id">({{ props.userId }})</span>
      </div>
      <button class="btn-reset" type="button" @click="emit('reset')">更换用户</button>
    </template>
  </div>
</template>

<style scoped>
.user-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  background: #fff;
  border-bottom: 1px solid #eef1f7;
}
.user-input {
  padding: 8px 12px;
  border: 1.5px solid #e2e6ef;
  border-radius: 20px;
  font-size: 13px;
  font-family: inherit;
  background: #f7f8fa;
  transition: border-color 0.15s ease;
}
.user-input:focus {
  outline: none;
  border-color: #5d8aff;
  background: #fff;
}
.user-input-name {
  flex: 1;
}
.btn-start {
  cursor: pointer;
  border: none;
  border-radius: 20px;
  padding: 8px 20px;
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  background: linear-gradient(135deg, #3b6df0, #5d8aff);
  box-shadow: 0 2px 8px rgba(59, 109, 240, 0.3);
  transition: transform 0.12s ease, box-shadow 0.12s ease;
  white-space: nowrap;
}
.btn-start:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(59, 109, 240, 0.4);
}
.btn-start:disabled {
  background: #b9c6ec;
  box-shadow: none;
  cursor: not-allowed;
}
.user-summary {
  flex: 1;
  font-size: 13px;
  color: #333;
}
.user-summary-id {
  color: #999;
  font-size: 12px;
  margin-left: 4px;
}
.btn-reset {
  cursor: pointer;
  border: 1.5px solid #e2e6ef;
  border-radius: 20px;
  padding: 6px 16px;
  font-size: 12px;
  color: #555;
  background: #f7f8fa;
}
.btn-reset:hover {
  border-color: #5d8aff;
  color: #3b6df0;
}
</style>
