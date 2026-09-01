<template>
  <div class="page-stack">
    <section class="page-heading page-heading-wrap">
      <div>
        <h2>账户管理</h2>
        <p>{{ status.healthy }} 健康 · {{ status.cooling }} 冷却 · {{ status.disabled }} 禁用</p>
      </div>
      <div class="button-group account-actions">
        <button class="button button-secondary" type="button" :disabled="operation !== ''" @click="$emit('refresh-credits')">
          <LoaderCircle v-if="operation === 'credits'" class="spin" :size="16" />
          <Coins v-else :size="16" />
          <span>刷新积分</span>
        </button>
        <button class="button button-secondary" type="button" :disabled="operation !== ''" @click="$emit('checkin')">
          <LoaderCircle v-if="operation === 'checkin'" class="spin" :size="16" />
          <CalendarCheck v-else :size="16" />
          <span>立即签到</span>
        </button>
        <button class="button button-primary" type="button" :disabled="operation !== ''" @click="oauthOpen = true">
          <UserRoundPlus :size="16" />
          <span>添加账户</span>
        </button>
      </div>
    </section>

    <section class="panel">
      <header class="panel-header">
        <div>
          <h3>账户池</h3>
          <span>{{ status.total }} 个账户</span>
        </div>
        <button
          class="icon-button"
          type="button"
          title="刷新账户状态"
          aria-label="刷新账户状态"
          :disabled="loading"
          @click="$emit('refresh')"
        >
          <RefreshCw :class="{ spin: loading }" :size="16" />
        </button>
      </header>
      <AccountTable :accounts="status.accounts" @copied="$emit('copied', $event)" />
    </section>

    <OAuthDialog :open="oauthOpen" @close="oauthOpen = false" @completed="oauthCompleted" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { CalendarCheck, Coins, LoaderCircle, RefreshCw, UserRoundPlus } from '@lucide/vue'
import type { StatusResponse } from '@/types'
import AccountTable from '@/components/AccountTable.vue'
import OAuthDialog from '@/components/OAuthDialog.vue'

defineProps<{
  status: StatusResponse
  loading: boolean
  operation: '' | 'credits' | 'checkin'
}>()

const emit = defineEmits<{
  refresh: []
  'refresh-credits': []
  checkin: []
  copied: [success: boolean]
  'oauth-completed': []
}>()

const oauthOpen = ref(false)

function oauthCompleted(): void {
  emit('oauth-completed')
}
</script>
