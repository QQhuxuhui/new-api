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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getApiKey, updateApiKey } from '../api'
import { useApiKeys } from '../components/api-keys-provider'
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import { buildApiKeyGroupUpdatePayload } from '../lib'

type GroupUpdate = { id: number; group: string }

/**
 * Changes one API key's group from the list. Each key is tracked separately,
 * so a key stays locked until its own request ends even while other keys are
 * being saved.
 */
export function useApiKeyGroupUpdate() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { triggerRefresh } = useApiKeys()
  const [pendingIds, setPendingIds] = useState<ReadonlySet<number>>(
    () => new Set()
  )

  const mutation = useMutation({
    mutationFn: async (update: GroupUpdate) => {
      const latest = requireServerSuccess(await getApiKey(update.id)).data
      if (!latest) {
        throw new Error(t(ERROR_MESSAGES.UPDATE_FAILED))
      }
      // The update endpoint rejects a negative quota on a limited key, and
      // silently zeroing it would change the key's balance.
      if (!latest.unlimited_quota && latest.remain_quota < 0) {
        return { negativeQuota: true as const }
      }
      const payload = buildApiKeyGroupUpdatePayload(latest, update.group)
      const result = await updateApiKey({ ...payload, id: update.id })
      return { negativeQuota: false as const, result }
    },
    onMutate: (update) => {
      setPendingIds((current) => new Set(current).add(update.id))
    },
    onSuccess: (outcome, update) => {
      if (outcome.negativeQuota) {
        toast.error(
          t(
            "This key's remaining quota is negative. Adjust its quota in Edit before changing the group."
          )
        )
        return
      }
      if (!outcome.result.success) {
        handleServerError(outcome.result, t(ERROR_MESSAGES.UPDATE_FAILED))
        return
      }
      toast.success(t(SUCCESS_MESSAGES.API_KEY_UPDATED))
      void queryClient.invalidateQueries({ queryKey: ['api-key', update.id] })
      triggerRefresh()
    },
    onError: (error) => {
      handleServerError(error, t(ERROR_MESSAGES.UPDATE_FAILED))
    },
    onSettled: (_outcome, _error, update) => {
      setPendingIds((current) => {
        const next = new Set(current)
        next.delete(update.id)
        return next
      })
    },
  })

  // mutate is stable across renders, so column definitions can depend on it.
  return { pendingIds, updateGroup: mutation.mutate }
}
