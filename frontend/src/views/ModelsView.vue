<template>
  <div class="page-stack">
    <section class="page-heading page-heading-wrap">
      <div>
        <h2>模型列表</h2>
        <p>{{ models.length }} 个可用模型</p>
      </div>
      <div class="model-heading-actions">
        <label class="select-control model-sort-control">
          <span>排序</span>
          <select v-model="sortKey" aria-label="排序字段" :disabled="!models.length">
            <option value="name">名称</option>
            <option value="credit_multiplier">积分倍率</option>
            <option value="context_length">上下文长度</option>
            <option value="max_output_tokens">最大输出</option>
          </select>
        </label>
        <button
          class="icon-button model-sort-direction"
          type="button"
          :title="sortDirectionLabel"
          :aria-label="sortDirectionLabel"
          :disabled="models.length < 2"
          @click="toggleSortDirection"
        >
          <ArrowUp v-if="sortDirection === 'asc'" :size="16" />
          <ArrowDown v-else :size="16" />
        </button>
        <button class="button button-secondary" type="button" :disabled="loading" @click="$emit('refresh')">
          <RefreshCw :class="{ spin: loading }" :size="16" />
          <span>刷新</span>
        </button>
      </div>
    </section>

    <section class="panel">
      <header class="panel-header">
        <div>
          <h3>OpenAI 兼容模型</h3>
          <span>实时列表与静态回退</span>
        </div>
      </header>
      <div v-if="models.length" class="table-scroll">
        <table class="data-table models-table">
          <thead>
            <tr>
              <th>模型 ID</th>
              <th>提供方</th>
              <th class="numeric">积分倍率</th>
              <th class="numeric">上下文长度</th>
              <th class="numeric">最大输出</th>
              <th class="actions-column"><span class="sr-only">操作</span></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="model in sortedModels" :key="model.id">
              <td><code class="model-id">{{ model.id }}</code></td>
              <td><span class="provider-label"><Box :size="14" />{{ model.owned_by }}</span></td>
              <td class="numeric strong-number">{{ formatCreditMultiplier(model.credit_multiplier) }}</td>
              <td class="numeric">{{ formatNumber(model.context_length) }}</td>
              <td class="numeric">{{ model.max_output_tokens ? formatNumber(model.max_output_tokens) : '-' }}</td>
              <td class="actions-column">
                <button class="tiny-icon-button" type="button" title="复制模型 ID" aria-label="复制模型 ID" @click="copyModel(model.id)">
                  <Copy :size="14" />
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="models.length" class="model-mobile-list">
        <article v-for="model in sortedModels" :key="model.id" class="model-mobile-row">
          <header>
            <code class="model-id">{{ model.id }}</code>
            <button class="tiny-icon-button" type="button" title="复制模型 ID" aria-label="复制模型 ID" @click="copyModel(model.id)">
              <Copy :size="14" />
            </button>
          </header>
          <dl>
            <div><dt>提供方</dt><dd>{{ model.owned_by }}</dd></div>
            <div><dt>积分倍率</dt><dd>{{ formatCreditMultiplier(model.credit_multiplier) }}</dd></div>
            <div><dt>上下文</dt><dd>{{ formatNumber(model.context_length) }}</dd></div>
            <div><dt>最大输出</dt><dd>{{ model.max_output_tokens ? formatNumber(model.max_output_tokens) : '-' }}</dd></div>
          </dl>
        </article>
      </div>
      <div v-else class="empty-state">
        <Boxes :size="28" />
        <strong>{{ loading ? '正在加载模型' : '暂无模型' }}</strong>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowDown, ArrowUp, Box, Boxes, Copy, RefreshCw } from '@lucide/vue'
import { copyText, formatCreditMultiplier, formatNumber } from '@/format'
import type { ModelInfo } from '@/types'

type SortKey = 'name' | 'credit_multiplier' | 'context_length' | 'max_output_tokens'
type SortDirection = 'asc' | 'desc'

const props = defineProps<{ models: ModelInfo[]; loading: boolean }>()
const emit = defineEmits<{ refresh: []; copied: [success: boolean] }>()
const modelNameCollator = new Intl.Collator('en', { numeric: true, sensitivity: 'base' })

const sortKey = ref<SortKey>('name')
const sortDirection = ref<SortDirection>('asc')
const sortDirectionLabel = computed(() => (sortDirection.value === 'asc' ? '切换为降序' : '切换为升序'))
const sortedModels = computed(() => [...props.models].sort(compareModels))

function sortValue(model: ModelInfo): string | number | undefined {
  switch (sortKey.value) {
    case 'credit_multiplier':
      return model.credit_multiplier
    case 'context_length':
      return model.context_length
    case 'max_output_tokens':
      return model.max_output_tokens
    default:
      return model.id
  }
}

function compareModels(left: ModelInfo, right: ModelInfo): number {
  const leftValue = sortValue(left)
  const rightValue = sortValue(right)

  if (leftValue === undefined || rightValue === undefined) {
    if (leftValue === undefined && rightValue === undefined) {
      return modelNameCollator.compare(left.id, right.id)
    }
    return leftValue === undefined ? 1 : -1
  }

  const comparison =
    typeof leftValue === 'string' && typeof rightValue === 'string'
      ? modelNameCollator.compare(leftValue, rightValue)
      : Number(leftValue) - Number(rightValue)

  if (comparison === 0) {
    return modelNameCollator.compare(left.id, right.id)
  }
  return sortDirection.value === 'asc' ? comparison : -comparison
}

function toggleSortDirection(): void {
  sortDirection.value = sortDirection.value === 'asc' ? 'desc' : 'asc'
}

async function copyModel(id: string): Promise<void> {
  emit('copied', await copyText(id))
}
</script>
