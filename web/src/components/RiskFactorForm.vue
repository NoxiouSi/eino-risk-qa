<script setup lang="ts">
import type { RiskFactorFormItem, RiskFactorType } from '../types'

const props = defineProps<{
  userId: string
  userName: string
  items: RiskFactorFormItem[]
  stream: boolean
  submitting: boolean
}>()

const emit = defineEmits<{
  'update:userId': [string]
  'update:userName': [string]
  'update:items': [RiskFactorFormItem[]]
  'update:stream': [boolean]
  submit: []
}>()

const typeOptions: { value: RiskFactorType; label: string }[] = [
  { value: 'identity', label: '身份信息 (identity)' },
  { value: 'fund_source', label: '资金来源 (fund_source)' },
]

function addItem() {
  const newItem: RiskFactorFormItem = { riskFactorType: 'identity', mainQuestion: '', answer: '' }
  emit('update:items', [...props.items, newItem])
}

function removeItem(index: number) {
  const next = props.items.filter((_, i) => i !== index)
  emit('update:items', next)
}
</script>

<template>
  <div class="form-panel">
    <h2>发起批量问答</h2>
    <div class="row">
      <label>
        用户ID
        <input
          :value="userId"
          placeholder="u_1001"
          @input="emit('update:userId', ($event.target as HTMLInputElement).value)"
        />
      </label>
      <label>
        姓名
        <input
          :value="userName"
          placeholder="张三（可选）"
          @input="emit('update:userName', ($event.target as HTMLInputElement).value)"
        />
      </label>
      <label class="checkbox-label">
        <input
          type="checkbox"
          :checked="stream"
          @change="emit('update:stream', ($event.target as HTMLInputElement).checked)"
        />
        流式输出（SSE）
      </label>
    </div>

    <div v-for="(item, index) in items" :key="index" class="risk-factor-item">
      <div class="risk-factor-header">
        <select
          :value="item.riskFactorType"
          @change="item.riskFactorType = (($event.target as HTMLSelectElement).value as RiskFactorType)"
        >
          <option v-for="opt in typeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
        </select>
        <button
          class="btn-danger-sm"
          type="button"
          :disabled="items.length <= 1"
          @click="removeItem(index)"
        >
          删除
        </button>
      </div>
      <textarea v-model="item.mainQuestion" placeholder="主问题，例如：请说明您的身份信息及职业背景" rows="2" />
      <textarea v-model="item.answer" placeholder="用户回答" rows="2" />
    </div>

    <div class="actions">
      <button class="btn-secondary" type="button" @click="addItem">+ 新增风险要素</button>
      <button class="btn-primary" type="button" :disabled="submitting" @click="emit('submit')">
        {{ submitting ? '提交中...' : '发起批量提交' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.form-panel {
  border: 1px solid #e2e2e2;
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 20px;
  background: #fafafa;
}
h2 {
  margin: 0 0 12px;
  font-size: 16px;
}
.row {
  display: flex;
  gap: 16px;
  margin-bottom: 12px;
  flex-wrap: wrap;
  align-items: center;
}
label {
  display: flex;
  flex-direction: column;
  font-size: 12px;
  color: #555;
  gap: 4px;
}
.checkbox-label {
  flex-direction: row;
  align-items: center;
  gap: 6px;
}
input[type='text'],
input:not([type]),
select,
textarea {
  padding: 6px 8px;
  border: 1px solid #ccc;
  border-radius: 4px;
  font-size: 13px;
  font-family: inherit;
}
textarea {
  width: 100%;
  box-sizing: border-box;
  margin-bottom: 6px;
  resize: vertical;
}
.risk-factor-item {
  border: 1px solid #ddd;
  border-radius: 6px;
  padding: 10px;
  margin-bottom: 10px;
  background: #fff;
}
.risk-factor-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
}
.actions {
  display: flex;
  gap: 10px;
  margin-top: 10px;
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
.btn-primary:disabled {
  background: #a9b8de;
  cursor: not-allowed;
}
.btn-secondary {
  background: #e7e7e7;
  color: #333;
}
.btn-danger-sm {
  background: #f5e2e2;
  color: #c0392b;
  padding: 2px 8px;
  font-size: 12px;
}
.btn-danger-sm:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
