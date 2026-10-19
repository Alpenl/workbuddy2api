<template>
  <div v-if="bootstrapping" class="bootstrap-screen" aria-label="正在加载">
    <span class="brand-mark"><Bot :size="22" /></span>
    <LoaderCircle class="spin" :size="20" />
  </div>

  <LoginView
    v-else-if="!unlocked"
    :busy="authBusy"
    :error="authError"
    :theme="theme"
    @login="authenticate"
    @toggle-theme="toggleTheme"
  />

  <AppShell
    v-else
    :current-page="currentPage"
    :status="status"
    :theme="theme"
    :mobile-open="mobileOpen"
    @navigate="navigate"
    @lock="lock"
    @toggle-theme="toggleTheme"
    @open-mobile="mobileOpen = true"
    @close-mobile="mobileOpen = false"
  >
    <OverviewView
      v-if="currentPage === 'overview'"
      :status="status"
      :loading="statusLoading"
      @refresh="loadStatus(true)"
      @copied="showCopyResult"
    />
    <AccountsView
      v-else-if="currentPage === 'accounts'"
      :status="status"
      :loading="statusLoading"
      :operation="operation"
      @refresh="loadStatus(true)"
      @refresh-credits="runCredits"
      @checkin="runCheckin"
      @copied="showCopyResult"
      @oauth-completed="oauthCompleted"
    />
    <ModelsView
      v-else-if="currentPage === 'models'"
      :models="models"
      :loading="modelsLoading"
      @refresh="loadModels(true)"
      @copied="showCopyResult"
    />
    <LogsView v-else-if="currentPage === 'logs'" @copied="showCopyResult" />
    <PlaygroundView v-else :models="models" />
  </AppShell>

  <Transition name="toast">
    <div v-if="toast" class="toast-message" :class="`toast-${toast.type}`" role="status">
      <CircleCheck v-if="toast.type === 'success'" :size="17" />
      <CircleAlert v-else :size="17" />
      <span>{{ toast.message }}</span>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Bot, CircleAlert, CircleCheck, LoaderCircle } from '@lucide/vue'
import {
  APIError,
  checkinAccounts,
  getModels,
  getStatus,
  refreshCredits,
  setAPIKey,
} from '@/api'
import type { ModelInfo, PageID, StatusResponse } from '@/types'
import LoginView from '@/components/LoginView.vue'
import AppShell from '@/components/AppShell.vue'
import AccountsView from '@/views/AccountsView.vue'
import ModelsView from '@/views/ModelsView.vue'
import OverviewView from '@/views/OverviewView.vue'
import PlaygroundView from '@/views/PlaygroundView.vue'
import LogsView from '@/views/LogsView.vue'

const SESSION_KEY = 'workbuddy2api:api-key'
const THEME_KEY = 'workbuddy2api:theme'
const pages: PageID[] = ['overview', 'accounts', 'models', 'playground', 'logs']

const emptyStatus = (): StatusResponse => ({ accounts: [], total: 0, healthy: 0, cooling: 0, disabled: 0 })
const bootstrapping = ref(true)
const unlocked = ref(false)
const authBusy = ref(false)
const authError = ref('')
const status = ref<StatusResponse>(emptyStatus())
const statusLoading = ref(false)
const models = ref<ModelInfo[]>([])
const modelsLoading = ref(false)
const operation = ref<'' | 'credits' | 'checkin'>('')
const mobileOpen = ref(false)
const currentPage = ref<PageID>(pageFromHash())
const theme = ref<'light' | 'dark'>(initialTheme())
const toast = ref<{ message: string; type: 'success' | 'error' } | null>(null)
let toastTimer: ReturnType<typeof setTimeout> | undefined

watch(theme, applyTheme, { immediate: true })

function initialTheme(): 'light' | 'dark' {
  const saved = localStorage.getItem(THEME_KEY)
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(value: 'light' | 'dark'): void {
  document.documentElement.dataset.theme = value
  document.documentElement.style.colorScheme = value
  localStorage.setItem(THEME_KEY, value)
}

function toggleTheme(): void {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
}

function pageFromHash(): PageID {
  const value = window.location.hash.replace(/^#\/?/, '') as PageID
  return pages.includes(value) ? value : 'overview'
}

function navigate(page: PageID): void {
  currentPage.value = page
  window.location.hash = `/${page}`
  if ((page === 'models' || page === 'playground') && !models.value.length) {
    void loadModels(false)
  }
}

function onHashChange(): void {
  currentPage.value = pageFromHash()
}

async function authenticate(key: string, silent = false): Promise<void> {
  authBusy.value = true
  authError.value = ''
  setAPIKey(key)
  try {
    status.value = await getStatus()
    sessionStorage.setItem(SESSION_KEY, key.trim())
    unlocked.value = true
    void loadModels(false)
  } catch (caught) {
    unlocked.value = false
    sessionStorage.removeItem(SESSION_KEY)
    setAPIKey('')
    if (!silent || !(caught instanceof APIError && caught.status === 401)) {
      authError.value = caught instanceof Error ? caught.message : '无法连接服务'
    }
  } finally {
    authBusy.value = false
  }
}

function lock(): void {
  sessionStorage.removeItem(SESSION_KEY)
  setAPIKey('')
  unlocked.value = false
  status.value = emptyStatus()
  models.value = []
  mobileOpen.value = false
}

async function loadStatus(notify: boolean): Promise<void> {
  if (statusLoading.value) return
  statusLoading.value = true
  try {
    status.value = await getStatus()
    if (notify) showToast('账户状态已刷新', 'success')
  } catch (caught) {
    if (!(caught instanceof APIError && caught.status === 401)) {
      showToast(caught instanceof Error ? caught.message : '刷新失败', 'error')
    }
  } finally {
    statusLoading.value = false
  }
}

async function loadModels(notify: boolean): Promise<void> {
  if (modelsLoading.value) return
  modelsLoading.value = true
  try {
    models.value = (await getModels()).data
    if (notify) showToast('模型列表已刷新', 'success')
  } catch (caught) {
    if (!(caught instanceof APIError && caught.status === 401)) {
      showToast(caught instanceof Error ? caught.message : '模型加载失败', 'error')
    }
  } finally {
    modelsLoading.value = false
  }
}

async function runCredits(): Promise<void> {
  if (operation.value) return
  operation.value = 'credits'
  try {
    const report = await refreshCredits()
    await loadStatus(false)
    showToast(`积分刷新完成：${report.succeeded}/${report.total}`, report.failed ? 'error' : 'success')
  } catch (caught) {
    showToast(caught instanceof Error ? caught.message : '积分刷新失败', 'error')
  } finally {
    operation.value = ''
  }
}

async function runCheckin(): Promise<void> {
  if (operation.value) return
  operation.value = 'checkin'
  try {
    const report = await checkinAccounts()
    await loadStatus(false)
    showToast(`签到完成：${report.succeeded}/${report.total}`, report.failed ? 'error' : 'success')
  } catch (caught) {
    showToast(caught instanceof Error ? caught.message : '签到失败', 'error')
  } finally {
    operation.value = ''
  }
}

function oauthCompleted(): void {
  void loadStatus(false)
  showToast('账户已添加', 'success')
}

function showCopyResult(success: boolean): void {
  showToast(success ? '已复制' : '复制失败', success ? 'success' : 'error')
}

function showToast(message: string, type: 'success' | 'error'): void {
  if (toastTimer) clearTimeout(toastTimer)
  toast.value = { message, type }
  toastTimer = setTimeout(() => {
    toast.value = null
  }, 2600)
}

function onUnauthorized(): void {
  if (unlocked.value) {
    lock()
    authError.value = 'API Key 已失效'
  }
}

onMounted(async () => {
  window.addEventListener('hashchange', onHashChange)
  window.addEventListener('workbuddy:unauthorized', onUnauthorized)
  const storedKey = sessionStorage.getItem(SESSION_KEY)
  await authenticate(storedKey ?? '', storedKey === null)
  bootstrapping.value = false
})

onBeforeUnmount(() => {
  window.removeEventListener('hashchange', onHashChange)
  window.removeEventListener('workbuddy:unauthorized', onUnauthorized)
  if (toastTimer) clearTimeout(toastTimer)
})
</script>
