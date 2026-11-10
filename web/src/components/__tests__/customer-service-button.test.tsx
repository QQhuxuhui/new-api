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
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { STATUS_QUERY_KEY } from '@/lib/status-query'

import { CustomerServiceButton } from '../customer-service-button'

const QR_URL = 'https://cdn.example.com/qq-group.jpg'

function renderButton(qrCode: unknown) {
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: { success: true, data: { CustomerServiceQRCode: qrCode } },
  }))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <span>header</span>
      <CustomerServiceButton />
    </QueryClientProvider>
  )
  return client
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  localStorage.clear()
})

describe('customer service header button', () => {
  test.each([
    ['missing', undefined],
    ['empty', ''],
    ['blank', '   '],
    ['not a web address', 'ftp://cdn.example.com/qq-group.jpg'],
  ])('stays hidden when the QR code is %s', async (_case, qrCode) => {
    const client = renderButton(qrCode)
    await waitFor(() =>
      expect(client.getQueryState(STATUS_QUERY_KEY)?.status).toBe('success')
    )
    expect(
      screen.queryByRole('button', { name: 'Contact support' })
    ).not.toBeInTheDocument()
  })

  test('opens the QR code in a dialog on click and closes it with Escape', async () => {
    const user = userEvent.setup()
    renderButton(`  ${QR_URL}  `)
    const button = await screen.findByRole('button', {
      name: 'Contact support',
    })

    await user.click(button)

    const dialog = await screen.findByRole('dialog', {
      name: 'Contact support',
    })
    expect(dialog.querySelector('img')).toHaveAttribute('src', QR_URL)
    expect(dialog.querySelector('img')).toHaveAttribute(
      'alt',
      'Support QR code'
    )
    await user.keyboard('{Escape}')
    expect(
      screen.queryByRole('dialog', { name: 'Contact support' })
    ).not.toBeInTheDocument()
  })

  test('shows a notice instead of a broken image when the QR code fails to load', async () => {
    const user = userEvent.setup()
    renderButton(QR_URL)
    await user.click(
      await screen.findByRole('button', { name: 'Contact support' })
    )
    const dialog = await screen.findByRole('dialog', {
      name: 'Contact support',
    })

    fireEvent.error(dialog.querySelector('img') as HTMLImageElement)

    expect(await screen.findByText('QR code failed to load')).toBeVisible()
    expect(dialog.querySelector('img')).toBeNull()
  })
})
