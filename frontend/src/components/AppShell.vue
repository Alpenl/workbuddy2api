<template>
  <div class="app-shell">
    <button
      v-if="mobileOpen"
      class="mobile-overlay"
      type="button"
      aria-label="关闭导航"
      @click="$emit('close-mobile')"
    />
    <aside class="sidebar" :class="{ 'sidebar-open': mobileOpen }">
      <div class="sidebar-brand brand-lockup">
        <span class="brand-mark" aria-hidden="true"><Bot :size="21" /></span>
        <span>
          <strong>WorkBuddy2API</strong>
          <small>API Console</small>
        </span>
      </div>

      <nav class="sidebar-nav" aria-label="主导航">
        <button
          v-for="item in navigation"
          :key="item.id"
          class="nav-item"
          :class="{ active: currentPage === item.id }"
          type="button"
          @click="selectPage(item.id)"
        >
          <component :is="item.icon" :size="18" aria-hidden="true" />
          <span>{{ item.label }}</span>
        </button>
      </nav>

      <div class="sidebar-footer">
        <div class="sidebar-health">
          <span class="health-dot" :class="status.healthy > 0 ? 'online' : 'offline'" />
          <span>{{ status.healthy > 0 ? '服务可用' : '暂无可用账户' }}</span>
          <strong>{{ status.healthy }}/{{ status.total }}</strong>
        </div>
        <button class="nav-item" type="button" @click="$emit('toggle-theme')">
          <Sun v-if="theme === 'dark'" :size="18" aria-hidden="true" />
          <Moon v-else :size="18" aria-hidden="true" />
          <span>{{ theme === 'dark' ? '浅色模式' : '深色模式' }}</span>
        </button>
      </div>
    </aside>

    <div class="workspace">
      <header class="topbar">
        <div class="topbar-title">
          <button
            class="icon-button mobile-menu-button"
            type="button"
            title="打开导航"
            aria-label="打开导航"
            @click="$emit('open-mobile')"
          >
            <Menu :size="19" />
          </button>
          <h1>{{ pageTitle }}</h1>
        </div>
        <div class="topbar-actions">
          <span class="desktop-health" :class="status.healthy > 0 ? 'is-online' : 'is-offline'">
            <Activity :size="15" />
            {{ status.healthy > 0 ? '运行中' : '不可用' }}
          </span>
          <button
            class="icon-button"
            type="button"
            title="锁定控制台"
            aria-label="锁定控制台"
            @click="$emit('lock')"
          >
            <LockKeyhole :size="17" />
          </button>
        </div>
      </header>
      <main class="main-content">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import {
  Activity,
  Bot,
  Boxes,
  Gauge,
  LockKeyhole,
  Menu,
  MessageSquareText,
  Moon,
  Sun,
  Users,
} from '@lucide/vue'
import type { PageID, StatusResponse } from '@/types'

const props = defineProps<{
  currentPage: PageID
  status: StatusResponse
  theme: 'light' | 'dark'
  mobileOpen: boolean
}>()

const emit = defineEmits<{
  navigate: [page: PageID]
  lock: []
  'toggle-theme': []
  'open-mobile': []
  'close-mobile': []
}>()

const navigation: Array<{ id: PageID; label: string; icon: Component }> = [
  { id: 'overview', label: '概览', icon: Gauge },
  { id: 'accounts', label: '账户', icon: Users },
  { id: 'models', label: '模型', icon: Boxes },
  { id: 'playground', label: '调试台', icon: MessageSquareText },
]

const pageTitle = computed(() => navigation.find((item) => item.id === props.currentPage)?.label || '概览')

function selectPage(page: PageID): void {
  emit('navigate', page)
  emit('close-mobile')
}
</script>
