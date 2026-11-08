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
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { ErrorRulesSection } from '../error-rules'
import type { ErrorRule } from '../error-rules/types'

function rule(overrides: Partial<ErrorRule>): ErrorRule {
  return {
    id: 'r1',
    name: 'rule',
    status_codes: [],
    keywords: ['upstream error'],
    match_type: 'KEYWORD_ONLY',
    enabled: true,
    priority: 0,
    error_type: 'server',
    return_immediately: false,
    retry_count: 0,
    retry_interval_ms: 0,
    description: '',
    ...overrides,
  }
}

function renderSection(initial: ErrorRule[]) {
  const changes: ErrorRule[][] = []
  function Harness() {
    const [json, setJson] = useState(JSON.stringify(initial))
    return (
      <ErrorRulesSection
        rulesJson={json}
        onRulesChange={(next) => {
          changes.push(JSON.parse(next) as ErrorRule[])
          setJson(next)
        }}
      />
    )
  }
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <Harness />
    </QueryClientProvider>
  )
  return changes
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('ErrorRulesSection', () => {
  it('adds a keyword rule from the dialog when the form is valid', async () => {
    const user = userEvent.setup()
    const changes = renderSection([])

    await user.click(screen.getByRole('button', { name: 'Add error rule' }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Name'), 'Busy upstream')
    await user.type(
      within(dialog).getByLabelText('Keywords (one per line)'),
      'Upstream rate limit exceeded'
    )
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(changes).toHaveLength(1))
    expect(changes[0]).toEqual([
      expect.objectContaining({
        name: 'Busy upstream',
        match_type: 'KEYWORD_ONLY',
        keywords: ['Upstream rate limit exceeded'],
        status_codes: [],
        error_type: 'server',
        enabled: true,
      }),
    ])
  })

  it('rejects an AND rule without status codes and keeps the rules unchanged', async () => {
    const user = userEvent.setup()
    const changes = renderSection([])

    await user.click(screen.getByRole('button', { name: 'Add error rule' }))
    const dialog = await screen.findByRole('dialog')
    await user.type(within(dialog).getByLabelText('Name'), 'Strict')
    await user.selectOptions(within(dialog).getByLabelText('Match condition'), 'AND')
    await user.type(
      within(dialog).getByLabelText('Keywords (one per line)'),
      'invalid key'
    )
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))

    expect(
      await within(dialog).findByText('This match type needs at least one status code')
    ).toBeInTheDocument()
    expect(changes).toHaveLength(0)
  })

  it('lists rules in evaluation order with the highest priority first', () => {
    renderSection([
      rule({ id: 'low', name: 'Low priority', priority: 1 }),
      rule({ id: 'high', name: 'High priority', priority: 90 }),
    ])

    const table = screen.getByRole('region', { name: 'Error rules table' })
    const names = within(table)
      .getAllByRole('row')
      .slice(1)
      .map((row) => (row.textContent ?? '').includes('High priority'))
    expect(names).toEqual([true, false])
  })

  it('disables a rule from the list switch', async () => {
    const user = userEvent.setup()
    const changes = renderSection([rule({ id: 'r1', name: 'Busy' })])

    await user.click(screen.getByRole('switch', { name: 'Enable Busy' }))

    expect(changes.at(-1)).toEqual([expect.objectContaining({ id: 'r1', enabled: false })])
  })

  it('tests the unsaved rules and reports the decisive rule', async () => {
    const user = userEvent.setup()
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        data: {
          rules: [{ id: 'r1', name: 'Too long', enabled: true, matched: true }],
          matched_rule_id: 'r1',
          error_type: 'client',
          return_immediately: true,
          retry_count: 0,
        },
      },
    })
    const rules = [
      rule({
        id: 'r1',
        name: 'Too long',
        error_type: 'client',
        return_immediately: true,
      }),
    ]
    renderSection(rules)

    await user.type(screen.getByLabelText('Error message'), 'Prompt is too long')
    await user.click(screen.getByRole('button', { name: 'Test' }))

    expect(
      await screen.findByText('Matched "Too long": client error, return immediately.')
    ).toBeInTheDocument()
    expect(post).toHaveBeenCalledWith('/api/option/request_policy/error_rules/test', {
      rules: JSON.stringify(rules),
      status_code: 400,
      message: 'Prompt is too long',
    })
  })
})
