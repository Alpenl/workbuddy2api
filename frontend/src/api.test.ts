import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createChatCompletion, getStatus, setAPIKey } from './api'

describe('API client', () => {
  beforeEach(() => {
    setAPIKey('test-secret')
    vi.restoreAllMocks()
  })

  it('sends the configured API key', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      accounts: [],
      total: 0,
      healthy: 0,
      cooling: 0,
      disabled: 0,
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await getStatus()

    expect(fetchMock).toHaveBeenCalledOnce()
    const options = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect((options.headers as Headers).get('Authorization')).toBe('Bearer test-secret')
  })

  it('aggregates content from an SSE chat response', async () => {
    const encoder = new TextEncoder()
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode('data: {"choices":[{"delta":{"content":"你"}}]}\n\n'))
        controller.enqueue(encoder.encode('data: {"choices":[{"delta":{"content":"好"}}]}\n\ndata: [DONE]\n\n'))
        controller.close()
      },
    })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(body, {
      status: 200,
      headers: { 'Content-Type': 'text/event-stream' },
    })))
    const deltas: string[] = []

    const result = await createChatCompletion(
      'glm-5.2',
      [{ role: 'user', content: 'hi' }],
      true,
      (delta) => deltas.push(delta),
    )

    expect(result).toBe('你好')
    expect(deltas).toEqual(['你', '好'])
  })
})
