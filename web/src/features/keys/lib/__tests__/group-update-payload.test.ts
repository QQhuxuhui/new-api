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
import { describe, expect, test } from 'vitest'

import { apiKeySchema } from '../../types'
import { buildApiKeyGroupUpdatePayload } from '../api-key-form'

const key = apiKeySchema.parse({
  id: 7,
  name: 'production',
  key: 'demo********1234',
  status: 2,
  remain_quota: 123_456_789,
  used_quota: 0,
  unlimited_quota: false,
  expired_time: 1_800_000_000,
  created_time: 0,
  accessed_time: 0,
  group: 'auto',
  auto_groups: ['vip', 'default'],
  cross_group_retry: true,
  model_limits_enabled: true,
  model_limits: '',
  allow_ips: '10.0.0.1',
  allow_user_quota_query: true,
})

describe('group-only update payload', () => {
  test('copies every other field verbatim when moving to a normal group', () => {
    expect(buildApiKeyGroupUpdatePayload(key, 'default')).toEqual({
      name: 'production',
      remain_quota: 123_456_789,
      expired_time: 1_800_000_000,
      unlimited_quota: false,
      model_limits_enabled: true,
      model_limits: '',
      allow_ips: '10.0.0.1',
      allow_user_quota_query: true,
      group: 'default',
      auto_groups: [],
      cross_group_retry: false,
    })
  })

  test('keeps the stored automatic order and turns retry on when moving to auto', () => {
    const payload = buildApiKeyGroupUpdatePayload(
      { ...key, group: 'default', cross_group_retry: false },
      'auto'
    )
    expect(payload.auto_groups).toEqual(['vip', 'default'])
    expect(payload.cross_group_retry).toBe(true)
  })

  test('inherits the global automatic order when the key has none', () => {
    const payload = buildApiKeyGroupUpdatePayload(
      { ...key, group: 'default', auto_groups: null },
      'auto'
    )
    expect(payload.auto_groups).toEqual([])
  })
})
