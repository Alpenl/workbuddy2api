<template>
  <div class="page-stack logs-page">
    <section class="page-heading page-heading-wrap">
      <div>
        <h2>运行日志</h2>
        <p>查看签到、聊天和服务运行记录，展开日志可查看完整内容。</p>
      </div>
      <div class="logs-actions">
        <label class="logs-live-toggle"><input v-model="live" type="checkbox" /> 自动刷新（5 秒）</label>
        <button class="button button-secondary" type="button" :disabled="loading" @click="load(false)">
          <RefreshCw :size="16" :class="{ spin: loading }" /> 刷新
        </button>
      </div>
    </section>

    <section class="panel logs-filter-panel" aria-label="日志筛选">
      <form class="logs-filters" @submit.prevent="applyNow">
        <label class="logs-field logs-search"><span>关键词</span><input v-model="filters.q" type="search" maxlength="128" placeholder="搜索日志内容、错误或任务结果" /></label>
        <label class="logs-field"><span>时间范围</span><select v-model="range">
          <option value="all">全部保留记录</option><option value="1h">最近 1 小时</option><option value="24h">最近 24 小时</option><option value="7d">最近 7 天</option><option value="custom">自定义时间</option>
        </select></label>
        <label class="logs-field"><span>级别</span><select v-model="filters.level"><option value="">全部级别</option><option value="info">信息</option><option value="warn">警告 / 重试</option><option value="error">错误</option></select></label>
        <label class="logs-field"><span>类别</span><select v-model="filters.category"><option value="">全部类别</option><option v-for="(label, value) in categories" :key="value" :value="value">{{ label }}</option></select></label>
        <label class="logs-field"><span>任务来源</span><select v-model="filters.source"><option value="">全部来源</option><option value="scheduled">自动任务</option><option value="manual">手动操作</option></select></label>
        <label class="logs-field"><span>HTTP 状态</span><select v-model="filters.status"><option value="">全部状态</option><option value="2xx">2xx 成功</option><option value="4xx">4xx 请求错误</option><option value="5xx">5xx 上游错误</option><option value="401">401</option><option value="429">429</option><option value="500">500</option></select></label>
        <label class="logs-field"><span>账号 ID</span><input v-model="filters.account" list="log-accounts" maxlength="128" placeholder="全部账号，可输入 ID 片段" /></label>
        <label class="logs-field"><span>模型</span><input v-model="filters.model" list="log-models" maxlength="128" placeholder="全部模型，可输入名称" /></label>
        <template v-if="range === 'custom'">
          <label class="logs-field"><span>开始时间</span><input v-model="since" type="datetime-local" /></label>
          <label class="logs-field"><span>结束时间</span><input v-model="until" type="datetime-local" /></label>
        </template>
        <div class="logs-filter-footer">
          <span>组合筛选 · 时间按本地时区显示 · 最新记录在前</span>
          <button class="button button-ghost" type="button" @click="resetFilters">重置筛选</button>
        </div>
      </form>
      <datalist id="log-accounts"><option v-for="account in page?.accounts || []" :key="account" :value="account" /></datalist>
      <datalist id="log-models"><option v-for="model in page?.models || []" :key="model" :value="model" /></datalist>
    </section>

    <div v-if="error" class="logs-notice logs-error" role="alert"><CircleAlert :size="17" /><span>{{ error }}</span><button class="button button-ghost" type="button" @click="applyNow">重试</button></div>
    <div v-if="page && !page.storage_ok" class="logs-notice logs-error" role="alert"><CircleAlert :size="17" /><span>日志写入磁盘异常，当前仍可查看内存中的记录。请检查存储空间和目录权限。</span></div>

    <section class="panel" :aria-busy="loading" aria-label="日志列表">
      <header class="panel-header logs-results-header">
        <div><h3>日志记录 <span class="logs-match-count">{{ page?.total ?? 0 }}</span></h3><span aria-live="polite">已显示 {{ entries.length }} 条 · 已保留 {{ page?.retained ?? 0 }} 条<span v-if="lastUpdated"> · {{ formatTime(lastUpdated, false) }} 更新</span></span></div>
        <label class="select-control"><span>每页</span><select v-model.number="pageSize" aria-label="每页条数"><option :value="50">50</option><option :value="100">100</option><option :value="200">200</option></select></label>
      </header>
      <div v-if="!entries.length" class="empty-state">
        <ScrollText :size="28" />
        <strong>{{ loading ? '正在读取日志' : error ? '暂时无法读取日志' : page?.retained ? '没有符合筛选条件的日志' : '暂无日志' }}</strong>
        <span v-if="!loading && !error">{{ page?.retained ? '调整时间或筛选条件后重试。' : '服务产生新的记录后会显示在这里。' }}</span>
      </div>
      <div v-else class="logs-list">
        <details v-for="entry in entries" :key="entry.id" class="log-entry" :data-level="entry.level">
          <summary>
            <span class="log-entry-meta">
              <time :datetime="entry.time">{{ formatTime(entry.time) }}</time>
              <span class="log-level" :class="`log-level-${entry.level}`">{{ levels[entry.level] }}</span>
              <span class="log-category">{{ categories[entry.category] || entry.category }}</span>
              <span v-if="entry.source" class="log-tag">{{ entry.source === 'scheduled' ? '自动' : '手动' }}</span>
              <code v-if="entry.status" class="log-tag">HTTP {{ entry.status }}</code>
              <code v-if="entry.account" class="log-tag" :title="entry.account">{{ entry.account.slice(0, 8) }}</code>
              <code v-if="entry.model" class="log-tag log-model">{{ entry.model }}</code>
            </span>
            <span class="log-preview">{{ preview(entry.message) }}</span>
          </summary>
          <div class="log-detail">
            <div class="log-detail-header"><span>记录 #{{ entry.id }}</span><button class="button button-ghost" type="button" @click="copyEntry(entry)"><Copy :size="14" />复制日志</button></div>
            <pre>{{ entry.message }}</pre>
          </div>
        </details>
      </div>
      <footer class="logs-pagination">
        <span v-if="!live && entries.length">已暂停自动刷新，可稳定浏览历史记录。</span><span v-else>每 5 秒获取最新记录</span>
        <button v-if="page?.has_more" class="button button-secondary" type="button" :disabled="loading" @click="loadOlder"><LoaderCircle v-if="loading" class="spin" :size="16" />加载更早日志</button>
        <span v-else-if="entries.length">已显示全部匹配记录</span>
      </footer>
    </section>
    <p v-if="page" class="logs-retention">日志跨重启保留，最多 {{ page.max_entries.toLocaleString() }} 条 / {{ Math.round(page.max_bytes / 1048576) }} MiB，超出后清理最旧记录。<span v-if="page.oldest"> 当前最早记录：{{ formatTime(page.oldest) }}。</span></p>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { CircleAlert, Copy, LoaderCircle, RefreshCw, ScrollText } from '@lucide/vue'
import { getLogs } from '@/api'
import { copyText } from '@/format'
import type { LogEntry, LogFilters, LogPage } from '@/types'

const emit = defineEmits<{ copied: [success: boolean] }>()
const categories: Record<string, string> = { checkin: '签到', credits: '余额查询', chat: '聊天', auth: '认证 / 保活', scheduler: '调度', system: '系统' }
const levels = { info: '信息', warn: '警告', error: '错误' }
const filters = reactive({ q: '', level: '', category: '', source: '', account: '', model: '', status: '' })
const range = ref('all')
const since = ref('')
const until = ref('')
const pageSize = ref(100)
const live = ref(true)
const loading = ref(false)
const error = ref('')
const entries = ref<LogEntry[]>([])
const page = ref<LogPage | null>(null)
const lastUpdated = ref('')
let controller: AbortController | undefined
let generation = 0
let disposed = false
let debounce: ReturnType<typeof setTimeout> | undefined
let poll: ReturnType<typeof setInterval> | undefined

function formatTime(value: string, date = true): string {
  return new Intl.DateTimeFormat('zh-CN', { ...(date ? { month: '2-digit', day: '2-digit' } as const : {}), hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(value))
}

function query(): LogFilters {
  const result: LogFilters = { ...filters, limit: pageSize.value }
  if (range.value === 'custom') {
    if (since.value) result.since = new Date(since.value).toISOString()
    if (until.value) result.until = new Date(until.value).toISOString()
    if (result.since && result.until && result.since > result.until) throw new Error('开始时间不能晚于结束时间')
  } else {
    const duration = { '1h': 3600000, '24h': 86400000, '7d': 604800000 }[range.value]
    if (duration) result.since = new Date(Date.now() - duration).toISOString()
  }
  return result
}

async function load(older = false): Promise<void> {
  if (disposed) return
  controller?.abort()
  controller = new AbortController()
  const version = ++generation
  loading.value = true
  error.value = ''
  try {
    const params = query()
    if (older && page.value?.next_before) params.before = page.value.next_before
    const result = await getLogs(params, controller.signal)
    if (version !== generation || disposed) return
    entries.value = older ? [...entries.value, ...result.entries.filter((entry) => !entries.value.some((old) => old.id === entry.id))] : result.entries
    page.value = result
    lastUpdated.value = new Date().toISOString()
  } catch (caught) {
    if (version !== generation || disposed || (caught instanceof Error && caught.name === 'AbortError')) return
    error.value = caught instanceof Error ? caught.message : '无法读取日志'
  } finally {
    if (version === generation && !disposed) loading.value = false
  }
}

function applyNow(): void {
  if (debounce) clearTimeout(debounce)
  debounce = undefined
  void load(false)
}

function loadOlder(): void {
  live.value = false
  void load(true)
}

function resetFilters(): void {
  Object.assign(filters, { q: '', level: '', category: '', source: '', account: '', model: '', status: '' })
  range.value = 'all'; since.value = ''; until.value = ''
}

function preview(message: string): string {
  const line = message.replace(/\s+/g, ' ')
  return line.length > 240 ? `${line.slice(0, 240)}…` : line
}

async function copyEntry(entry: LogEntry): Promise<void> {
  emit('copied', await copyText(`${entry.time} [${entry.level}] ${entry.message}`))
}

watch([filters, range, since, until, pageSize], () => {
  controller?.abort(); generation++
  entries.value = []; page.value = null
  if (debounce) clearTimeout(debounce)
  debounce = setTimeout(applyNow, 300)
}, { deep: true })
watch(live, (value) => { if (value) applyNow() })

onMounted(() => {
  void load()
  poll = setInterval(() => { if (live.value && !loading.value && !debounce && document.visibilityState !== 'hidden') void load() }, 5000)
})
onBeforeUnmount(() => {
  disposed = true; generation++; controller?.abort()
  if (poll) clearInterval(poll)
  if (debounce) clearTimeout(debounce)
})
</script>
