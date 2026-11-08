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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'

import type { ChannelHealth } from '../../api'
import { ChannelHealthCell } from '../channel-health-cell'
import { ChannelHealthContext } from '../channel-health-context'

function health(overrides: Partial<ChannelHealth>): ChannelHealth {
  return {
    channel_id: 7,
    always_healthy: false,
    is_warning: false,
    is_suspended: false,
    consecutive_failures: 0,
    current_failure_rate: 0,
    window_total_requests: 0,
    window_failure_count: 0,
    ...overrides,
  }
}

function renderCell(state: ChannelHealth | null) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  const map = new Map<number, ChannelHealth>()
  if (state) map.set(state.channel_id, state)
  render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <ChannelHealthContext.Provider value={map}>
          <ChannelHealthCell channelId={7} />
        </ChannelHealthContext.Provider>
      </TooltipProvider>
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('ChannelHealthCell', () => {
  it('shows a placeholder when the channel has no health data', () => {
    renderCell(null)
    expect(screen.getByText('—')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Reset channel health' })
    ).not.toBeInTheDocument()
  })

  it('offers no reset for a healthy channel', () => {
    renderCell(health({}))
    expect(screen.getByText('Healthy')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Reset channel health' })
    ).not.toBeInTheDocument()
  })

  it('shows the always-healthy exemption instead of a degraded state', () => {
    renderCell(health({ always_healthy: true, is_warning: true }))
    expect(screen.getByText('Always Healthy')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Reset channel health' })
    ).not.toBeInTheDocument()
  })

  it('resets a suspended channel through the API', async () => {
    const user = userEvent.setup()
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: true } })
    renderCell(
      health({
        is_suspended: true,
        suspended_until: Math.floor(Date.now() / 1000) + 90,
      })
    )

    expect(screen.getByText(/^Suspended \(/)).toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: 'Reset channel health' })
    )

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/api/channel/7/health/reset')
    )
  })
})
