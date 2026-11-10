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
import { useQuery } from '@tanstack/react-query'
import { createContext, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { getUserGroups } from '@/lib/api'
import {
  requireServerSuccess,
  translateServerText,
} from '@/lib/server-error-message'

import type { ApiKeyGroupOption } from '../components/api-key-group-combobox'
import { useApiKeyGroupUpdate } from './use-api-key-group-update'

type UserGroupInfo = { desc?: string; ratio?: number | string }

/** What the group column needs to show ratios and change a key's group. */
export type ApiKeyGroupEditing = {
  ratios: Record<string, number | string>
  options: ApiKeyGroupOption[]
  pendingIds: ReadonlySet<number>
  updateGroup: (update: { id: number; group: string }) => void
}

const EMPTY_USER_GROUPS: Record<string, UserGroupInfo> = {}

/**
 * Carries the group column's changing state (ratios, choices, keys being
 * saved) outside the column definitions. Column cells are rendered by
 * identity, so putting this state in the column definitions would remount
 * every cell, and close an open group list, whenever it changed.
 */
export const ApiKeyGroupEditingContext =
  createContext<ApiKeyGroupEditing | null>(null)

/** Groups the user may pick, built like the edit drawer, plus saving. */
export function useApiKeyGroupEditing(): ApiKeyGroupEditing {
  const { t } = useTranslation()
  const { data } = useQuery({
    queryKey: ['user-groups'],
    queryFn: async () => requireServerSuccess(await getUserGroups()),
    staleTime: 0,
    select: (res): Record<string, UserGroupInfo> =>
      res.success && res.data ? res.data : EMPTY_USER_GROUPS,
  })
  const groups = data ?? EMPTY_USER_GROUPS
  const { pendingIds, updateGroup } = useApiKeyGroupUpdate()

  const choices = useMemo(() => {
    const ratios: Record<string, number | string> = {}
    const options: ApiKeyGroupOption[] = []
    for (const [group, info] of Object.entries(groups)) {
      const ratio =
        typeof info.ratio === 'number' || typeof info.ratio === 'string'
          ? info.ratio
          : undefined
      if (ratio !== undefined) ratios[group] = ratio
      options.push({
        value: group,
        label: group,
        desc: info.desc ? translateServerText(t, info.desc) : group,
        ratio,
      })
    }
    return { ratios, options }
  }, [groups, t])

  return useMemo(
    () => ({ ...choices, pendingIds, updateGroup }),
    [choices, pendingIds, updateGroup]
  )
}
