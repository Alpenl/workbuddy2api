<template>
  <div class="page-stack">
    <section class="page-heading">
      <div>
        <h2>模型列表</h2>
        <p>{{ models.length }} 个可用模型</p>
      </div>
      <button class="button button-secondary" type="button" :disabled="loading" @click="$emit('refresh')">
        <RefreshCw :class="{ spin: loading }" :size="16" />
        <span>刷新</span>
      </button>
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
              <th class="numeric">上下文长度</th>
              <th class="numeric">最大输出</th>
              <th class="actions-column"><span class="sr-only">操作</span></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="model in models" :key="model.id">
              <td><code class="model-id">{{ model.id }}</code></td>
              <td><span class="provider-label"><Box :size="14" />{{ model.owned_by }}</span></td>
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
        <article v-for="model in models" :key="model.id" class="model-mobile-row">
          <header>
            <code class="model-id">{{ model.id }}</code>
            <button class="tiny-icon-button" type="button" title="复制模型 ID" aria-label="复制模型 ID" @click="copyModel(model.id)">
              <Copy :size="14" />
            </button>
          </header>
          <dl>
            <div><dt>提供方</dt><dd>{{ model.owned_by }}</dd></div>
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
import { Box, Boxes, Copy, RefreshCw } from '@lucide/vue'
import { copyText, formatNumber } from '@/format'
import type { ModelInfo } from '@/types'

defineProps<{ models: ModelInfo[]; loading: boolean }>()
const emit = defineEmits<{ refresh: []; copied: [success: boolean] }>()

async function copyModel(id: string): Promise<void> {
  emit('copied', await copyText(id))
}
</script>
