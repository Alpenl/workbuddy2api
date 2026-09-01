<template>
  <main class="login-screen">
    <button
      class="icon-button login-theme-button"
      type="button"
      :title="theme === 'dark' ? '切换浅色模式' : '切换深色模式'"
      :aria-label="theme === 'dark' ? '切换浅色模式' : '切换深色模式'"
      @click="$emit('toggle-theme')"
    >
      <Sun v-if="theme === 'dark'" :size="18" />
      <Moon v-else :size="18" />
    </button>

    <section class="login-panel" aria-labelledby="login-title">
      <div class="brand-lockup login-brand">
        <span class="brand-mark" aria-hidden="true"><Bot :size="22" /></span>
        <span>
          <strong id="login-title">WorkBuddy2API</strong>
          <small>管理控制台</small>
        </span>
      </div>

      <form class="login-form" @submit.prevent="submit">
        <label class="field-label" for="api-key">API Key</label>
        <div class="input-with-action">
          <KeyRound :size="17" aria-hidden="true" />
          <input
            id="api-key"
            v-model="key"
            :type="showKey ? 'text' : 'password'"
            autocomplete="current-password"
            placeholder="未配置时可留空"
            spellcheck="false"
            :disabled="busy"
            autofocus
          />
          <button
            type="button"
            class="input-action"
            :title="showKey ? '隐藏 API Key' : '显示 API Key'"
            :aria-label="showKey ? '隐藏 API Key' : '显示 API Key'"
            @click="showKey = !showKey"
          >
            <EyeOff v-if="showKey" :size="17" />
            <Eye v-else :size="17" />
          </button>
        </div>
        <p v-if="error" class="inline-alert inline-alert-error" role="alert">
          <CircleAlert :size="16" />
          <span>{{ error }}</span>
        </p>
        <button class="button button-primary login-submit" type="submit" :disabled="busy">
          <LoaderCircle v-if="busy" class="spin" :size="17" />
          <LogIn v-else :size="17" />
          <span>{{ busy ? '正在验证' : '进入控制台' }}</span>
        </button>
      </form>
    </section>
  </main>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import {
  Bot,
  CircleAlert,
  Eye,
  EyeOff,
  KeyRound,
  LoaderCircle,
  LogIn,
  Moon,
  Sun,
} from '@lucide/vue'

defineProps<{
  busy: boolean
  error: string
  theme: 'light' | 'dark'
}>()

const emit = defineEmits<{
  login: [key: string]
  'toggle-theme': []
}>()

const key = ref('')
const showKey = ref(false)

function submit(): void {
  emit('login', key.value)
}
</script>
