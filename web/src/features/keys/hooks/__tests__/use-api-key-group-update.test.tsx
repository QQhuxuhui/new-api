/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createInstance } from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { toast } from 'sonner'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { useApiKeys, ApiKeysProvider } from '../../components/api-keys-provider'
import { apiKeySchema, type ApiKey } from '../../types'
import { useApiKeyGroupUpdate } from '../use-api-key-group-update'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

function makeKey(overrides: Partial<ApiKey>): ApiKey {
  return apiKeySchema.parse({
    id: 1,
    name: 'production',
    key: 'demo********1234',
    status: 1,
    remain_quota: 123_456_789,
    used_quota: 0,
    unlimited_quota: false,
    expired_time: -1,
    created_time: 0,
    accessed_time: 0,
    group: 'default',
    model_limits_enabled: true,
    model_limits: '',
    ...overrides,
  })
}

type Deferred<T> = { promise: Promise<T>; resolve: (value: T) => void }

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

function renderUpdate() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const wrapper = (props: { children: ReactNode }) => (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <ApiKeysProvider>{props.children}</ApiKeysProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
  return renderHook(
    () => ({ update: useApiKeyGroupUpdate(), keys: useApiKeys() }),
    { wrapper }
  )
}

function mockLatestKeys(keys: Record<number, ApiKey>) {
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    const id = Number(url.split('/').pop())
    return { data: { success: true, data: keys[id] } }
  })
}

describe('inline API key group update', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  test('saves the latest key with only its group changed and refreshes the list', async () => {
    mockLatestKeys({ 1: makeKey({ id: 1 }) })
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const success = vi.spyOn(toast, 'success')
    const { result } = renderUpdate()
    const refreshBefore = result.current.keys.refreshTrigger

    act(() => result.current.update.updateGroup({ id: 1, group: 'vip' }))

    await waitFor(() => expect(success).toHaveBeenCalledTimes(1))
    expect(put).toHaveBeenCalledWith('/api/token/', {
      id: 1,
      name: 'production',
      remain_quota: 123_456_789,
      expired_time: -1,
      unlimited_quota: false,
      model_limits_enabled: true,
      model_limits: '',
      allow_ips: '',
      allow_user_quota_query: false,
      group: 'vip',
      auto_groups: [],
      cross_group_retry: false,
    })
    expect(result.current.keys.refreshTrigger).not.toBe(refreshBefore)
    expect(result.current.update.pendingIds.size).toBe(0)
  })

  test('keeps each key locked until its own save ends when saves overlap', async () => {
    mockLatestKeys({ 1: makeKey({ id: 1 }), 2: makeKey({ id: 2 }) })
    const first = deferred<{ data: { success: boolean } }>()
    const second = deferred<{ data: { success: boolean } }>()
    vi.spyOn(api, 'put').mockImplementation(async (_url, body) =>
      (body as { id: number }).id === 1 ? first.promise : second.promise
    )
    const { result } = renderUpdate()

    act(() => result.current.update.updateGroup({ id: 1, group: 'vip' }))
    act(() => result.current.update.updateGroup({ id: 2, group: 'vip' }))
    await waitFor(() =>
      expect([...result.current.update.pendingIds].sort()).toEqual([1, 2])
    )

    await act(async () => second.resolve({ data: { success: true } }))
    await waitFor(() =>
      expect([...result.current.update.pendingIds]).toEqual([1])
    )

    await act(async () => first.resolve({ data: { success: true } }))
    await waitFor(() => expect(result.current.update.pendingIds.size).toBe(0))
  })

  test('does not submit a limited key whose remaining quota is negative', async () => {
    mockLatestKeys({ 1: makeKey({ id: 1, remain_quota: -500 }) })
    const put = vi.spyOn(api, 'put')
    const error = vi.spyOn(toast, 'error')
    const { result } = renderUpdate()

    act(() => result.current.update.updateGroup({ id: 1, group: 'vip' }))

    await waitFor(() => expect(error).toHaveBeenCalledTimes(1))
    expect(put).not.toHaveBeenCalled()
    expect(result.current.update.pendingIds.size).toBe(0)
  })

  test('reports a rejected save and unlocks the key without refreshing', async () => {
    mockLatestKeys({ 1: makeKey({ id: 1 }) })
    vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: false, message: 'No permission to access group vip' },
    })
    const error = vi.spyOn(toast, 'error')
    const { result } = renderUpdate()
    const refreshBefore = result.current.keys.refreshTrigger

    act(() => result.current.update.updateGroup({ id: 1, group: 'vip' }))

    await waitFor(() => expect(error).toHaveBeenCalled())
    expect(result.current.keys.refreshTrigger).toBe(refreshBefore)
    await waitFor(() => expect(result.current.update.pendingIds.size).toBe(0))
  })
})
