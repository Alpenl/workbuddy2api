import type {
  ChatMessage,
  ModelsResponse,
  OAuthPollResponse,
  OAuthStartResponse,
  OperationReport,
  StatusResponse,
  LogFilters,
  LogPage,
} from './types'

let apiKey = ''

export class APIError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'APIError'
    this.status = status
    this.code = code
  }
}

export function setAPIKey(value: string): void {
  apiKey = value.trim()
}

function headers(extra?: HeadersInit): Headers {
  const result = new Headers(extra)
  if (apiKey) {
    result.set('Authorization', `Bearer ${apiKey}`)
  }
  return result
}

async function errorFrom(response: Response): Promise<APIError> {
  let message = `请求失败 (${response.status})`
  let code = 'request_failed'
  try {
    const body = (await response.json()) as {
      error?: { message?: string; code?: string }
    }
    message = body.error?.message || message
    code = body.error?.code || code
  } catch {
    // Keep the HTTP fallback when the response is not JSON.
  }
  if (response.status === 401) {
    window.dispatchEvent(new CustomEvent('workbuddy:unauthorized'))
  }
  return new APIError(response.status, code, message)
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { ...init, headers: headers(init?.headers) })
  if (!response.ok) {
    throw await errorFrom(response)
  }
  return (await response.json()) as T
}

export function getStatus(): Promise<StatusResponse> {
  return requestJSON<StatusResponse>('/status')
}

export function getLogs(filters: LogFilters = {}, signal?: AbortSignal): Promise<LogPage> {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== '') query.set(key, String(value))
  }
  return requestJSON<LogPage>(`/api/admin/logs?${query.toString()}`, { signal, cache: 'no-store' })
}

export function getModels(): Promise<ModelsResponse> {
  return requestJSON<ModelsResponse>('/v1/models')
}

export function refreshCredits(): Promise<OperationReport> {
  return requestJSON<OperationReport>('/api/admin/credits/refresh', { method: 'POST' })
}

export function checkinAccounts(): Promise<OperationReport> {
  return requestJSON<OperationReport>('/api/admin/checkin', { method: 'POST' })
}

export function startOAuth(): Promise<OAuthStartResponse> {
  return requestJSON<OAuthStartResponse>('/api/admin/oauth/start', { method: 'POST' })
}

export function pollOAuth(flowID: string): Promise<OAuthPollResponse> {
  return requestJSON<OAuthPollResponse>('/api/admin/oauth/poll', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ flow_id: flowID }),
  })
}

export async function createChatCompletion(
  model: string,
  messages: ChatMessage[],
  stream: boolean,
  onDelta: (text: string) => void,
  signal?: AbortSignal,
): Promise<string> {
  const response = await fetch('/v1/chat/completions', {
    method: 'POST',
    headers: headers({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ model, messages, stream }),
    signal,
  })
  if (!response.ok) {
    throw await errorFrom(response)
  }
  if (!stream) {
    const body = (await response.json()) as {
      choices?: Array<{ message?: { content?: string } }>
    }
    const content = body.choices?.[0]?.message?.content || ''
    onDelta(content)
    return content
  }

  if (!response.body) {
    throw new APIError(502, 'empty_stream', '上游未返回响应流')
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let content = ''
  while (true) {
    const { done, value } = await reader.read()
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done })
    const lines = buffer.split('\n')
    buffer = done ? '' : (lines.pop() || '')
    for (const rawLine of lines) {
      const line = rawLine.trim()
      if (!line.startsWith('data:')) continue
      const data = line.slice(5).trim()
      if (!data || data === '[DONE]') continue
      try {
        const event = JSON.parse(data) as {
          choices?: Array<{ delta?: { content?: string } }>
        }
        const delta = event.choices?.[0]?.delta?.content || ''
        if (delta) {
          content += delta
          onDelta(delta)
        }
      } catch {
        // Ignore non-JSON SSE comments and keep consuming the stream.
      }
    }
    if (done) break
  }
  return content
}
