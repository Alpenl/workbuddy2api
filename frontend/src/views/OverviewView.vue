<template>
  <div class="page-stack">
    <section class="page-heading">
      <div>
        <h2>运行概览</h2>
        <p>WorkBuddy 账户池</p>
      </div>
      <button class="button button-secondary" type="button" :disabled="loading" @click="$emit('refresh')">
        <RefreshCw :class="{ spin: loading }" :size="16" />
        <span>刷新</span>
      </button>
    </section>

    <section class="metric-grid" aria-label="账户池指标">
      <article class="metric-card metric-total">
        <span class="metric-icon"><Users :size="19" /></span>
        <span><small>账户总数</small><strong>{{ formatNumber(status.total) }}</strong></span>
      </article>
      <article class="metric-card metric-healthy">
        <span class="metric-icon"><CircleCheck :size="19" /></span>
        <span><small>健康账户</small><strong>{{ formatNumber(status.healthy) }}</strong></span>
      </article>
      <article class="metric-card metric-cooling">
        <span class="metric-icon"><TimerReset :size="19" /></span>
        <span><small>冷却账户</small><strong>{{ formatNumber(status.cooling) }}</strong></span>
      </article>
      <article class="metric-card metric-credits">
        <span class="metric-icon"><Coins :size="19" /></span>
        <span><small>可用积分</small><strong>{{ formatNumber(totalCredits) }}</strong></span>
      </article>
    </section>

    <section class="connection-strip">
      <div>
        <span class="health-dot" :class="status.healthy > 0 ? 'online' : 'offline'" />
        <span>{{ status.healthy > 0 ? 'OpenAI 接口可用' : 'OpenAI 接口不可用' }}</span>
      </div>
      <div class="endpoint-value">
        <code>{{ baseURL }}</code>
        <button class="tiny-icon-button" type="button" title="复制 Base URL" aria-label="复制 Base URL" @click="copyBaseURL">
          <Copy :size="14" />
        </button>
      </div>
    </section>

    <section class="panel">
      <header class="panel-header">
        <div>
          <h3>账户概览</h3>
          <span>{{ status.total }} 个账户</span>
        </div>
      </header>
      <AccountTable :accounts="status.accounts.slice(0, 6)" @copied="$emit('copied', $event)" />
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CircleCheck, Coins, Copy, RefreshCw, TimerReset, Users } from '@lucide/vue'
import { copyText, formatNumber } from '@/format'
import type { StatusResponse } from '@/types'
import AccountTable from '@/components/AccountTable.vue'

const props = defineProps<{
  status: StatusResponse
  loading: boolean
}>()

const emit = defineEmits<{
  refresh: []
  copied: [success: boolean]
}>()

const totalCredits = computed(() => props.status.accounts.reduce((sum, account) => sum + account.credits, 0))
const baseURL = `${window.location.origin}/v1`

async function copyBaseURL(): Promise<void> {
  emit('copied', await copyText(baseURL))
}
</script>
