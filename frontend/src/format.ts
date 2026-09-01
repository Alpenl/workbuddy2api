export function formatNumber(value: number): string {
  return new Intl.NumberFormat('zh-CN').format(value || 0)
}

export function formatCreditMultiplier(value?: number): string {
  if (value === undefined || !Number.isFinite(value)) return '-'
  return `x${value.toFixed(2)}`
}

export function compactUID(uid: string): string {
  if (uid.length <= 18) return uid
  return `${uid.slice(0, 8)}...${uid.slice(-6)}`
}

export function formatDate(value?: string): string {
  if (!value || value.startsWith('0001-01-01')) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

export function formatDuration(seconds?: number): string {
  if (!seconds || seconds <= 0) return '-'
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.ceil((seconds % 3600) / 60)
  if (hours > 0) return `${hours}小时${minutes > 0 ? `${minutes}分` : ''}`
  return `${Math.max(1, minutes)}分钟`
}

export async function copyText(value: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(value)
    return true
  } catch {
    return false
  }
}
