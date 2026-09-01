<template>
  <div class="playground-page">
    <section class="page-heading page-heading-wrap">
      <div>
        <h2>聊天调试台</h2>
        <p>{{ selectedModel || '未选择模型' }}</p>
      </div>
      <div class="playground-toolbar">
        <label class="select-control">
          <span>模型</span>
          <select v-model="selectedModel" :disabled="busy || !models.length">
            <option v-if="!models.length" value="">暂无模型</option>
            <option v-for="model in models" :key="model.id" :value="model.id">{{ model.id }}</option>
          </select>
        </label>
        <div class="segmented-control" aria-label="响应模式">
          <button type="button" :class="{ active: stream }" :disabled="busy" @click="stream = true">流式</button>
          <button type="button" :class="{ active: !stream }" :disabled="busy" @click="stream = false">完整</button>
        </div>
      </div>
    </section>

    <section class="playground-tool">
      <div class="playground-settings">
        <label class="field-label" for="system-prompt">System Prompt</label>
        <textarea
          id="system-prompt"
          v-model="systemPrompt"
          rows="3"
          placeholder="You are a helpful assistant."
          :disabled="busy"
        />
      </div>

      <div ref="transcript" class="chat-transcript" aria-live="polite">
        <div v-if="!conversation.length" class="chat-empty">
          <MessageSquareText :size="30" />
          <strong>暂无会话</strong>
        </div>
        <article
          v-for="(message, index) in conversation"
          :key="index"
          class="chat-message"
          :class="[`chat-${message.role}`, { 'chat-error': message.error }]"
        >
          <span class="chat-avatar" aria-hidden="true">
            <User v-if="message.role === 'user'" :size="16" />
            <Bot v-else :size="16" />
          </span>
          <div>
            <header>
              <strong>{{ message.role === 'user' ? '你' : selectedModel || '模型' }}</strong>
              <span v-if="message.pending"><LoaderCircle class="spin" :size="13" />生成中</span>
            </header>
            <p>{{ message.content || (message.pending ? ' ' : '（空响应）') }}</p>
          </div>
        </article>
      </div>

      <div class="composer">
        <textarea
          v-model="prompt"
          rows="3"
          placeholder="输入消息"
          :disabled="busy"
          @keydown.ctrl.enter.prevent="send"
          @keydown.meta.enter.prevent="send"
        />
        <div class="composer-actions">
          <button
            class="icon-button"
            type="button"
            title="清空会话"
            aria-label="清空会话"
            :disabled="busy || !conversation.length"
            @click="clear"
          >
            <Trash2 :size="17" />
          </button>
          <button v-if="busy" class="button button-danger" type="button" @click="stop">
            <Square :size="15" />
            <span>停止</span>
          </button>
          <button v-else class="button button-primary" type="button" :disabled="!canSend" @click="send">
            <Send :size="16" />
            <span>发送</span>
          </button>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import {
  Bot,
  LoaderCircle,
  MessageSquareText,
  Send,
  Square,
  Trash2,
  User,
} from '@lucide/vue'
import { createChatCompletion } from '@/api'
import type { ChatMessage, ModelInfo } from '@/types'

interface ConversationMessage extends ChatMessage {
  pending?: boolean
  error?: boolean
}

const props = defineProps<{ models: ModelInfo[] }>()

const selectedModel = ref('')
const stream = ref(true)
const systemPrompt = ref('')
const prompt = ref('')
const busy = ref(false)
const conversation = ref<ConversationMessage[]>([])
const transcript = ref<HTMLElement>()
let controller: AbortController | undefined

const canSend = computed(() => Boolean(selectedModel.value && prompt.value.trim()))

watch(
  () => props.models,
  (models) => {
    if (models.some((model) => model.id === selectedModel.value)) return
    selectedModel.value = models.find((model) => model.id === 'deepseek-v4-flash')?.id || models[0]?.id || ''
  },
  { immediate: true },
)

async function scrollToBottom(): Promise<void> {
  await nextTick()
  if (transcript.value) transcript.value.scrollTop = transcript.value.scrollHeight
}

async function send(): Promise<void> {
  const content = prompt.value.trim()
  if (!content || !selectedModel.value || busy.value) return
  prompt.value = ''
  conversation.value.push({ role: 'user', content })
  const requestMessages: ChatMessage[] = conversation.value.map(({ role, content: text }) => ({ role, content: text }))
  if (systemPrompt.value.trim()) {
    requestMessages.unshift({ role: 'system', content: systemPrompt.value.trim() })
  }
  const answer: ConversationMessage = { role: 'assistant', content: '', pending: true }
  conversation.value.push(answer)
  busy.value = true
  controller = new AbortController()
  await scrollToBottom()
  try {
    await createChatCompletion(
      selectedModel.value,
      requestMessages,
      stream.value,
      (delta) => {
        answer.content += delta
        void scrollToBottom()
      },
      controller.signal,
    )
  } catch (caught) {
    answer.error = true
    answer.content = caught instanceof DOMException && caught.name === 'AbortError'
      ? '请求已停止'
      : caught instanceof Error
        ? caught.message
        : '请求失败'
  } finally {
    answer.pending = false
    busy.value = false
    controller = undefined
    await scrollToBottom()
  }
}

function stop(): void {
  controller?.abort()
}

function clear(): void {
  conversation.value = []
}
</script>
