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
import { RotateCcw } from 'lucide-react'
import { useContext } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import { resetChannelHealth } from '../api'
import {
  ChannelHealthContext,
  channelHealthQueryKey,
} from './channel-health-context'

export function ChannelHealthCell(props: { channelId: number }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const healthById = useContext(ChannelHealthContext)
  const queryClient = useQueryClient()
  const reset = useMutation({
    mutationFn: () => resetChannelHealth(props.channelId),
    onSuccess: async () => {
      toast.success(t('Channel health reset'))
      await queryClient.invalidateQueries({ queryKey: channelHealthQueryKey })
    },
    onError: (error) => handleServerError(error),
    meta: { errorToast: false },
  })
  const health = healthById?.get(props.channelId)
  if (!health) {
    return <span className='text-muted-foreground'>—</span>
  }

  let label = t('Healthy')
  let variant: 'success' | 'warning' | 'danger' | 'neutral' = 'success'
  if (health.always_healthy) {
    label = t('Always Healthy')
    variant = 'neutral'
  } else if (health.is_suspended) {
    const seconds = Math.max(
      0,
      Math.ceil((health.suspended_until ?? 0) - Date.now() / 1000)
    )
    label = t('Suspended ({{seconds}}s)', { seconds })
    variant = 'danger'
  } else if (health.is_warning) {
    label = t('Warning')
    variant = 'warning'
  }
  const failureRate = formatNumber(
    Math.round(health.current_failure_rate * 100),
    locale
  )
  const canReset =
    !health.always_healthy && (health.is_suspended || health.is_warning)

  return (
    <div className='flex items-center gap-1'>
      <Tooltip>
        <TooltipTrigger render={<span tabIndex={0} />}>
          <StatusBadge label={label} variant={variant} size='sm' copyable={false} />
        </TooltipTrigger>
        <TooltipContent side='top'>
          {t('Last 60s: {{failures}}/{{total}} failed ({{rate}}%)', {
            failures: formatNumber(health.window_failure_count, locale),
            total: formatNumber(health.window_total_requests, locale),
            rate: failureRate,
          })}
        </TooltipContent>
      </Tooltip>
      {canReset ? (
        <Button
          variant='ghost'
          size='icon'
          className='size-7'
          aria-label={t('Reset channel health')}
          disabled={reset.isPending}
          onClick={(event) => {
            event.stopPropagation()
            reset.mutate()
          }}
        >
          <RotateCcw aria-hidden='true' />
        </Button>
      ) : null}
    </div>
  )
}
