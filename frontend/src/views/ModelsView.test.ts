import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ModelsView from './ModelsView.vue'
import type { ModelInfo } from '@/types'

const models: ModelInfo[] = [
  {
    id: 'zeta-free',
    object: 'model',
    created: 1753600000,
    owned_by: 'workbuddy',
    context_length: 32768,
    max_output_tokens: 4096,
    credit_multiplier: 0,
  },
  {
    id: 'alpha-pro',
    object: 'model',
    created: 1753600000,
    owned_by: 'workbuddy',
    context_length: 131072,
    max_output_tokens: 8192,
    credit_multiplier: 0.79,
  },
  {
    id: 'model-10',
    object: 'model',
    created: 1753600000,
    owned_by: 'workbuddy',
    context_length: 65536,
    max_output_tokens: 16384,
    credit_multiplier: 0.17,
  },
  {
    id: 'model-2',
    object: 'model',
    created: 1753600000,
    owned_by: 'workbuddy',
    context_length: 262144,
  },
]

function mountModels() {
  return mount(ModelsView, {
    props: { models, loading: false },
  })
}

function modelIds(wrapper: ReturnType<typeof mountModels>, selector: string): string[] {
  return wrapper.findAll(`${selector} .model-id`).map((node) => node.text())
}

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

  it('sorts both model lists by name with natural ordering by default', () => {
    const wrapper = mountModels()
    const expected = ['alpha-pro', 'model-2', 'model-10', 'zeta-free']

    expect(modelIds(wrapper, '.models-table tbody')).toEqual(expected)
    expect(modelIds(wrapper, '.model-mobile-list')).toEqual(expected)
  })

  it('sorts by credit multiplier in either direction and keeps missing values last', async () => {
    const wrapper = mountModels()
    const select = wrapper.get<HTMLSelectElement>('select[aria-label="排序字段"]')

    await select.setValue('credit_multiplier')
    expect(modelIds(wrapper, '.models-table tbody')).toEqual(['zeta-free', 'model-10', 'alpha-pro', 'model-2'])
    expect(modelIds(wrapper, '.model-mobile-list')).toEqual(['zeta-free', 'model-10', 'alpha-pro', 'model-2'])

    await wrapper.get('button[aria-label="切换为降序"]').trigger('click')
    expect(modelIds(wrapper, '.models-table tbody')).toEqual(['alpha-pro', 'model-10', 'zeta-free', 'model-2'])
    expect(modelIds(wrapper, '.model-mobile-list')).toEqual(['alpha-pro', 'model-10', 'zeta-free', 'model-2'])
  })

  it.each([
    ['context_length', ['zeta-free', 'model-10', 'alpha-pro', 'model-2']],
    ['max_output_tokens', ['zeta-free', 'alpha-pro', 'model-10', 'model-2']],
  ])('sorts by %s', async (sortKey, expected) => {
    const wrapper = mountModels()

    await wrapper.get<HTMLSelectElement>('select[aria-label="排序字段"]').setValue(sortKey)

    expect(modelIds(wrapper, '.models-table tbody')).toEqual(expected)
    expect(modelIds(wrapper, '.model-mobile-list')).toEqual(expected)
  })
})
