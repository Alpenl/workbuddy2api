<template>
  <Teleport to="body">
    <div v-if="open" class="dialog-backdrop" @mousedown.self="close">
      <section class="dialog" role="dialog" aria-modal="true" aria-labelledby="oauth-title">
        <header class="dialog-header">
          <div>
            <span class="dialog-icon"><UserRoundPlus :size="19" /></span>
            <h2 id="oauth-title">添加 WorkBuddy 账户</h2>
          </div>
          <button class="icon-button" type="button" title="关闭" aria-label="关闭" @click="close">
            <X :size="18" />
          </button>
        </header>

        <div class="dialog-body oauth-dialog-body">
          <div class="oauth-state" :class="`oauth-${state}`">
            <LoaderCircle v-if="state === 'starting' || state === 'pending'" class="spin" :size="23" />
            <CircleCheck v-else-if="state === 'completed'" :size="23" />
            <CircleAlert v-else-if="state === 'error'" :size="23" />
            <KeyRound v-else :size="23" />
            <span>
              <strong>{{ stateTitle }}</strong>
              <small v-if="state === 'pending'">自动检查授权结果</small>
              <small v-else-if="state === 'error'">{{ error }}</small>
              <small v-else-if="state === 'completed'">账户已写入并加载</small>
              <small v-else>CodeBuddy CN OAuth</small>
            </span>
          </div>

          <a
            v-if="authURL"
            class="button button-secondary oauth-link"
            :href="authURL"
            target="_blank"
            rel="noopener noreferrer"
          >
            <ExternalLink :size="16" />
            <span>重新打开授权页</span>
          </a>
        </div>

        <footer class="dialog-footer">
          <button class="button button-secondary" type="button" @click="close">取消</button>
          <button
            v-if="state === 'idle' || state === 'error'"
            class="button button-primary"
            type="button"
            @click="start"
          >
            <ExternalLink :size="16" />
            <span>{{ state === 'error' ? '重新开始' : '打开授权页' }}</span>
          </button>
          <button v-else-if="state === 'pending'" class="button button-primary" type="button" disabled>
            <LoaderCircle class="spin" :size="16" />
            <span>等待授权</span>
          </button>
          <button v-else-if="state === 'completed'" class="button button-primary" type="button" @click="close">
            <CircleCheck :size="16" />
            <span>完成</span>
          </button>
        </footer>
      </section>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import {
  CircleAlert,
  CircleCheck,
  ExternalLink,
  KeyRound,
  LoaderCircle,
  UserRoundPlus,
  X,
} from '@lucide/vue'
import { pollOAuth, startOAuth } from '@/api'

type OAuthState = 'idle' | 'starting' | 'pending' | 'completed' | 'error'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{
  close: []
  completed: []
}>()

const state = ref<OAuthState>('idle')
const flowID = ref('')
const authURL = ref('')
const error = ref('')
let timer: ReturnType<typeof setTimeout> | undefined

const stateTitle = computed(() => {
  if (state.value === 'starting') return '正在创建授权'
  if (state.value === 'pending') return '等待浏览器授权'
  if (state.value === 'completed') return '授权成功'
  if (state.value === 'error') return '授权未完成'
  return '准备授权'
})

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) reset()
    else stopPolling()
  },
)

function reset(): void {
  stopPolling()
  state.value = 'idle'
  flowID.value = ''
  authURL.value = ''
  error.value = ''
}

function close(): void {
  stopPolling()
  emit('close')
}

async function start(): Promise<void> {
  state.value = 'starting'
  error.value = ''
  try {
    const result = await startOAuth()
    flowID.value = result.flow_id
    authURL.value = result.auth_url
    state.value = 'pending'
    window.open(result.auth_url, '_blank', 'noopener,noreferrer')
    timer = setTimeout(poll, 1500)
  } catch (caught) {
    state.value = 'error'
    error.value = caught instanceof Error ? caught.message : '无法创建授权'
  }
}

async function poll(): Promise<void> {
  if (!props.open || state.value !== 'pending' || !flowID.value) return
  try {
    const result = await pollOAuth(flowID.value)
    if (result.status === 'completed') {
      state.value = 'completed'
      emit('completed')
      return
    }
    timer = setTimeout(poll, 2000)
  } catch (caught) {
    state.value = 'error'
    error.value = caught instanceof Error ? caught.message : '授权检查失败'
  }
}

function stopPolling(): void {
  if (timer) clearTimeout(timer)
  timer = undefined
}

onBeforeUnmount(stopPolling)
</script>
