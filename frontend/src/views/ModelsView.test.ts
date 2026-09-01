import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ModelsView from './ModelsView.vue'
import type { ModelInfo } from '@/types'

describe('ModelsView', () => {
  it('shows the credit multiplier in desktop and mobile model lists', () => {
    const model: ModelInfo = {
      id: 'glm-5.2',
      object: 'model',
      created: 1753600000,
      owned_by: 'workbuddy',
      context_length: 131072,
      max_output_tokens: 8192,
      credit_multiplier: 0.79,
    }

    const wrapper = mount(ModelsView, {
      props: { models: [model], loading: false },
    })

    expect(wrapper.find('table').text()).toContain('积分倍率')
    expect(wrapper.find('table').text()).toContain('x0.79')
    expect(wrapper.find('.model-mobile-list').text()).toContain('积分倍率')
    expect(wrapper.find('.model-mobile-list').text()).toContain('x0.79')
  })
})
