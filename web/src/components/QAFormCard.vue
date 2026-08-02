<script setup lang="ts">
import { uploadAttachment } from '../api/client'
import { riskFactorTypeLabel } from '../types'
import type { QuestionDraftMap, QuestionItem } from '../types'

const props = defineProps<{
  userId: string
  riskFactorType: string
  mainQuestion: string
  questions: QuestionItem[]
  drafts: QuestionDraftMap
  disabled: boolean
  generating?: boolean
  showCursor?: boolean
  errorMessage?: string
}>()
const emit = defineEmits<{ 'update:drafts': [QuestionDraftMap] }>()

function updateText(key: string, text: string) {
  emit('update:drafts', { ...props.drafts, [key]: { ...(props.drafts[key] ?? { fileIds: [], fileNames: [] }), text } })
}

async function handleFiles(question: QuestionItem, event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  if (!files.length) return
  const current = props.drafts[question.question_key] ?? { text: '', fileIds: [], fileNames: [] }
  emit('update:drafts', { ...props.drafts, [question.question_key]: { ...current, uploading: true, error: '' } })
  const uploaded = await Promise.allSettled(files.map((file) => uploadAttachment(props.userId, props.riskFactorType, question.question_key, file)))
  const success = uploaded.filter((item): item is PromiseFulfilledResult<Awaited<ReturnType<typeof uploadAttachment>>> => item.status === 'fulfilled')
  const failed = uploaded.find((item) => item.status === 'rejected')
  emit('update:drafts', {
    ...props.drafts,
    [question.question_key]: {
      ...current,
      fileIds: [...current.fileIds, ...success.map((item) => item.value.file_id)],
      fileNames: [...current.fileNames, ...success.map((item) => item.value.original_name)],
      uploading: false,
      error: failed ? (failed.reason as Error).message : '',
    },
  })
  input.value = ''
}

function removeFile(key: string, index: number) {
  const current = props.drafts[key]
  if (!current) return
  emit('update:drafts', {
    ...props.drafts,
    [key]: { ...current, fileIds: current.fileIds.filter((_, i) => i !== index), fileNames: current.fileNames.filter((_, i) => i !== index) },
  })
}
</script>

<template>
  <section class="qa-card">
    <div class="qa-heading">
      <span class="qa-tag">{{ riskFactorTypeLabel(props.riskFactorType) }}</span>
      <div class="qa-title">
        <span v-if="props.generating && !props.mainQuestion" class="qa-placeholder">正在生成问题…</span>
        <template v-else>{{ props.mainQuestion }}</template><span v-if="props.showCursor" class="qa-cursor">▍</span>
      </div>
    </div>
    <p v-if="props.errorMessage" class="qa-error">{{ props.errorMessage }}，请重新提交</p>

    <div class="question-list">
      <article v-for="question in props.questions" :key="question.question_key" class="question-item">
        <div class="question-meta">
          <label :for="`${props.riskFactorType}-${question.question_key}`">{{ question.question_text }}</label>
          <span v-if="question.required" class="required">必填</span>
          <span v-if="question.min_submit_count > 1" class="count">至少 {{ question.min_submit_count }} 份</span>
        </div>
        <textarea
          v-if="question.answer_type === 'text'"
          :id="`${props.riskFactorType}-${question.question_key}`"
          class="text-input"
          :value="props.drafts[question.question_key]?.text ?? ''"
          :disabled="props.disabled || props.generating"
          rows="2"
          :placeholder="`请填写${question.question_text}`"
          @input="updateText(question.question_key, ($event.target as HTMLTextAreaElement).value)"
        />
        <div v-else class="upload-area">
          <label class="upload-button" :class="{ disabled: props.disabled || props.generating || props.drafts[question.question_key]?.uploading }">
            <input type="file" accept="image/jpeg,image/png,image/webp" multiple :disabled="props.disabled || props.generating" @change="handleFiles(question, $event)" />
            {{ props.drafts[question.question_key]?.uploading ? '上传中…' : '选择图片' }}
          </label>
          <div v-if="props.drafts[question.question_key]?.fileNames.length" class="file-list">
            <span v-for="(name, index) in props.drafts[question.question_key].fileNames" :key="`${name}-${index}`" class="file-chip">
              {{ name }}<button type="button" :disabled="props.disabled" @click="removeFile(question.question_key, index)">×</button>
            </span>
          </div>
          <p v-if="props.drafts[question.question_key]?.error" class="field-error">{{ props.drafts[question.question_key].error }}</p>
        </div>
      </article>
    </div>
  </section>
</template>

<style scoped>
.qa-card{display:flex;flex-direction:column;gap:14px;padding:18px;margin:6px 0;border:1px solid #e3e9fb;border-radius:18px;background:linear-gradient(145deg,#fff,#f8faff);box-shadow:0 8px 24px rgba(49,87,213,.07);transition:transform .18s ease,box-shadow .18s ease}.qa-card:hover{transform:translateY(-1px);box-shadow:0 12px 30px rgba(49,87,213,.11)}.qa-heading{display:flex;align-items:flex-start;gap:10px}.qa-tag{flex-shrink:0;padding:4px 10px;border-radius:999px;background:#eef2ff;color:#3157d5;font-size:12px;font-weight:600}.qa-title{color:#172033;font-size:15px;font-weight:600;line-height:1.55}.question-list{display:flex;flex-direction:column;gap:12px}.question-item{display:flex;flex-direction:column;gap:8px;padding:13px;border:1px solid #edf0f7;border-radius:13px;background:#fff}.question-meta{display:flex;align-items:center;gap:8px;font-size:13px;font-weight:600;color:#172033}.required{color:#d64545;font-size:11px}.count{color:#5c667a;font-size:11px}.text-input{box-sizing:border-box;width:100%;padding:10px 12px;border:1.5px solid #dfe5f3;border-radius:10px;background:#fbfcff;font:14px inherit;resize:vertical;transition:border-color .15s,box-shadow .15s}.text-input:focus{outline:none;border-color:#5378ea;box-shadow:0 0 0 3px rgba(83,120,234,.12)}.upload-area{display:flex;flex-direction:column;gap:8px}.upload-button{align-self:flex-start;padding:8px 15px;border:1.5px dashed #8da5ee;border-radius:10px;color:#3157d5;background:#f5f7ff;font-size:13px;cursor:pointer;transition:background .15s}.upload-button:hover{background:#eef2ff}.upload-button.disabled{opacity:.55;cursor:not-allowed}.upload-button input{display:none}.file-list{display:flex;flex-wrap:wrap;gap:7px}.file-chip{display:flex;align-items:center;gap:6px;padding:5px 9px;border-radius:9px;background:#eef7f3;color:#167554;font-size:12px}.file-chip button{border:0;background:transparent;color:inherit;cursor:pointer}.qa-error,.field-error{margin:0;color:#d64545;font-size:12px}.qa-placeholder{color:#8992a5}.qa-cursor{animation:blink 1s steps(1) infinite}@keyframes blink{50%{opacity:0}}textarea:disabled{background:#f2f4f8;color:#8b93a3}
</style>
