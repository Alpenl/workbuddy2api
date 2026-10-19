export type PageID = 'overview' | 'accounts' | 'models' | 'playground' | 'logs'

export interface LogEntry {
  id: number
  time: string
  level: 'info' | 'warn' | 'error'
  category: string
  source?: string
  account?: string
  model?: string
  status?: number
  message: string
}

export interface LogPage {
  entries: LogEntry[]
  total: number
  retained: number
  has_more: boolean
  next_before?: number
  oldest?: string
  newest?: string
  accounts: string[]
  models: string[]
  max_entries: number
  max_bytes: number
  storage_ok: boolean
}

export interface LogFilters {
  q?: string
  level?: string
  category?: string
  source?: string
  account?: string
  model?: string
  status?: string
  since?: string
  until?: string
  before?: number
  limit?: number
}

export interface AccountStatus {
  uid: string
  nickname?: string
  credits: number
  cooling: boolean
  cool_kind?: string
  cool_remaining_sec?: number
  until?: string
  reason?: string
  disabled: boolean
  success_count?: number
  err_count?: number
  last_success?: string
  last_err?: string
}

export interface StatusResponse {
  accounts: AccountStatus[]
  total: number
  healthy: number
  cooling: number
  disabled: number
}

export interface ModelInfo {
  id: string
  object: string
  created: number
  owned_by: string
  context_length: number
  max_output_tokens?: number
  credit_multiplier?: number
}

export interface ModelsResponse {
  object: 'list'
  data: ModelInfo[]
}

export interface OperationAccount {
  uid: string
  nickname?: string
  credits: number
  ok: boolean
  message?: string
}

export interface OperationReport {
  total: number
  succeeded: number
  failed: number
  accounts: OperationAccount[]
}

export interface OAuthStartResponse {
  flow_id: string
  auth_url: string
  expires_in: number
}

export interface OAuthPollResponse {
  status: 'pending' | 'completed'
  account?: AccountStatus
}

export interface ChatMessage {
  role: 'system' | 'user' | 'assistant'
  content: string
}
