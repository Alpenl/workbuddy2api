<template>
  <span class="status-badge" :class="statusClass">
    <CircleOff v-if="account.disabled" :size="13" />
    <TimerReset v-else-if="account.cooling" :size="13" />
    <CircleCheck v-else :size="13" />
    <span>{{ statusLabel }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CircleCheck, CircleOff, TimerReset } from '@lucide/vue'
import type { AccountStatus } from '@/types'

const props = defineProps<{ account: AccountStatus }>()

const statusLabel = computed(() => {
  if (props.account.disabled) return '已禁用'
  if (props.account.cooling) return '冷却中'
  return '健康'
})

const statusClass = computed(() => {
  if (props.account.disabled) return 'status-disabled'
  if (props.account.cooling) return 'status-cooling'
  return 'status-healthy'
})
</script>
