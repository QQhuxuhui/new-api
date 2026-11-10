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
import type { CellContext } from '@tanstack/react-table'
import { useContext } from 'react'

import { useMediaQuery } from '@/hooks'

import { ApiKeyGroupEditingContext } from '../hooks/use-api-key-group-editing'
import type { ApiKey } from '../types'
import { ApiKeyGroupCell } from './api-key-group-cell'

const EMPTY_RATIOS: Record<string, number | string> = {}

/**
 * Group column cell. A stable component type, so table re-renders update it
 * in place instead of remounting it. Without the editing context it is
 * read-only and shows no ratio.
 */
export function ApiKeyGroupColumnCell(props: CellContext<ApiKey, unknown>) {
  const editing = useContext(ApiKeyGroupEditingContext)
  const shouldReduceMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
  const apiKey = props.row.original
  const group = apiKey.group ?? ''
  const updateGroup = editing?.updateGroup

  return (
    <ApiKeyGroupCell
      group={group}
      ratio={(editing?.ratios ?? EMPTY_RATIOS)[group]}
      crossGroupRetry={apiKey.cross_group_retry}
      shouldReduceMotion={shouldReduceMotion}
      options={editing?.options}
      isUpdating={editing?.pendingIds.has(apiKey.id) ?? false}
      onGroupChange={
        updateGroup
          ? (next) => updateGroup({ id: apiKey.id, group: next })
          : undefined
      }
    />
  )
}
