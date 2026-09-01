<template>
  <div v-if="accounts.length" class="table-scroll account-desktop-table">
    <table class="data-table account-table">
      <thead>
        <tr>
          <th>账户</th>
          <th>状态</th>
          <th class="numeric">积分</th>
          <th class="numeric">成功请求</th>
          <th>最近成功</th>
          <th>状态详情</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="account in accounts" :key="account.uid">
          <td>
            <div class="account-identity">
              <span class="account-avatar" aria-hidden="true">{{ accountInitial(account) }}</span>
              <span class="account-name-block">
                <strong>{{ account.nickname || '未命名账户' }}</strong>
                <span class="uid-line">
                  <code :title="account.uid">{{ compactUID(account.uid) }}</code>
                  <button
                    class="tiny-icon-button"
                    type="button"
                    title="复制 UID"
                    aria-label="复制 UID"
                    @click="copyUID(account.uid)"
                  >
                    <Copy :size="13" />
                  </button>
                </span>
              </span>
            </div>
          </td>
          <td><StatusBadge :account="account" /></td>
          <td class="numeric strong-number">{{ formatNumber(account.credits) }}</td>
          <td class="numeric">{{ formatNumber(account.success_count || 0) }}</td>
          <td>{{ formatDate(account.last_success) }}</td>
          <td>
            <span v-if="account.disabled" class="muted-text" :title="account.reason">{{ account.reason || '会话失效' }}</span>
            <span v-else-if="account.cooling" class="muted-text">
              {{ formatDuration(account.cool_remaining_sec) }}
            </span>
            <span v-else class="muted-text">-</span>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
  <div v-if="accounts.length" class="account-mobile-list">
    <article v-for="account in accounts" :key="account.uid" class="account-mobile-row">
      <header>
        <div class="account-identity">
          <span class="account-avatar" aria-hidden="true">{{ accountInitial(account) }}</span>
          <span class="account-name-block">
            <strong>{{ account.nickname || '未命名账户' }}</strong>
            <span class="uid-line">
              <code :title="account.uid">{{ compactUID(account.uid) }}</code>
              <button class="tiny-icon-button" type="button" title="复制 UID" aria-label="复制 UID" @click="copyUID(account.uid)">
                <Copy :size="13" />
              </button>
            </span>
          </span>
        </div>
        <StatusBadge :account="account" />
      </header>
      <dl>
        <div><dt>积分</dt><dd>{{ formatNumber(account.credits) }}</dd></div>
        <div><dt>成功请求</dt><dd>{{ formatNumber(account.success_count || 0) }}</dd></div>
        <div><dt>最近成功</dt><dd>{{ formatDate(account.last_success) }}</dd></div>
        <div>
          <dt>状态详情</dt>
          <dd v-if="account.disabled" :title="account.reason">{{ account.reason || '会话失效' }}</dd>
          <dd v-else-if="account.cooling">{{ formatDuration(account.cool_remaining_sec) }}</dd>
          <dd v-else>-</dd>
        </div>
      </dl>
    </article>
  </div>
  <div v-else class="empty-state">
    <Users :size="28" aria-hidden="true" />
    <strong>暂无账户</strong>
  </div>
</template>

<script setup lang="ts">
import { Copy, Users } from '@lucide/vue'
import { compactUID, copyText, formatDate, formatDuration, formatNumber } from '@/format'
import type { AccountStatus } from '@/types'
import StatusBadge from './StatusBadge.vue'

defineProps<{ accounts: AccountStatus[] }>()

const emit = defineEmits<{ copied: [success: boolean] }>()

function accountInitial(account: AccountStatus): string {
  return (account.nickname || account.uid || 'W').slice(0, 1).toUpperCase()
}

async function copyUID(uid: string): Promise<void> {
  emit('copied', await copyText(uid))
}
</script>
