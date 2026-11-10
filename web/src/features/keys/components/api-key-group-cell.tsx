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
import { useTranslation } from 'react-i18next'

import { BadgeCell, TruncatedCell } from '@/components/data-table'
import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useMediaQuery } from '@/hooks'
import { cn } from '@/lib/utils'

import {
  ApiKeyGroupCombobox,
  type ApiKeyGroupOption,
} from './api-key-group-combobox'
import { GroupRatioBadge, type GroupRatio } from './auto-group-visuals'

type ApiKeyGroupCellProps = {
  crossGroupRetry: boolean
  group: string
  ratio?: GroupRatio
  shouldReduceMotion: boolean
  /** Selectable groups; with onGroupChange, makes the cell editable on desktop. */
  options?: ApiKeyGroupOption[]
  onGroupChange?: (group: string) => void
  isUpdating?: boolean
}

export function ApiKeyGroupCell(props: ApiKeyGroupCellProps) {
  const { t } = useTranslation()
  // Same breakpoint at which DataTablePage switches to mobile cards, which
  // render this cell too; cards stay read-only and edit through the drawer.
  const isMobile = useMediaQuery('(max-width: 640px)')

  const group = props.group?.trim() || ''
  const isAuto = group === 'auto'
  const ratio =
    !isAuto && group && typeof props.ratio === 'number'
      ? props.ratio
      : undefined
  const fullName = group || t('Follow user group')

  const groupBadge = (
    <GroupBadge
      group={group}
      ratio={ratio}
      ratioLabel={group ? undefined : t('Inherited')}
      type='text'
      className='px-0'
      containerClassName={cn('gap-3', isMobile && 'w-full justify-between')}
    />
  )
  const autoBadges = (
    <>
      <StatusBadge
        label={t('Cross-group')}
        variant='info'
        copyable={false}
        className='px-0'
      />
      <GroupRatioBadge
        ratio={props.ratio}
        isAuto
        shouldReduceMotion={props.shouldReduceMotion}
      />
    </>
  )

  const isEditable =
    !isMobile &&
    props.onGroupChange !== undefined &&
    (props.options?.length ?? 0) > 0

  if (isEditable) {
    const onGroupChange = props.onGroupChange
    return (
      <Tooltip>
        <TooltipTrigger
          render={
            <div
              data-api-key-group-cell={isAuto ? 'auto' : 'editable'}
              className='block max-w-66 min-w-0'
            />
          }
        >
          <ApiKeyGroupCombobox
            options={props.options ?? []}
            value={group}
            disabled={props.isUpdating}
            triggerAriaLabel={t('Change group: {{group}}', {
              group: fullName,
            })}
            onValueChange={(next) => {
              if (next !== group) onGroupChange?.(next)
            }}
          >
            {isAuto ? (
              <span className='flex min-w-0 items-center gap-3 text-xs'>
                {autoBadges}
              </span>
            ) : (
              <span className='flex max-w-full min-w-0'>{groupBadge}</span>
            )}
          </ApiKeyGroupCombobox>
        </TooltipTrigger>
        <TooltipContent className='max-w-xs break-all'>
          <span className='text-xs'>
            {isAuto
              ? t(
                  'Automatically selects the best available group with circuit breaker mechanism'
                )
              : fullName}
          </span>
        </TooltipContent>
      </Tooltip>
    )
  }

  if (!isAuto) {
    return (
      <TruncatedCell
        className={isMobile ? 'w-full' : 'max-w-66'}
        tabIndex={0}
        tooltipContent={fullName}
        tooltipClassName='break-all'
      >
        {groupBadge}
      </TruncatedCell>
    )
  }

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <BadgeCell
            data-api-key-group-cell='auto'
            tabIndex={0}
            className={cn(
              'ml-0 gap-3 overflow-visible text-xs',
              isMobile ? 'w-full justify-between' : 'max-w-66'
            )}
          />
        }
      >
        {autoBadges}
      </TooltipTrigger>
      <TooltipContent>
        <span className='text-xs'>
          {t(
            'Automatically selects the best available group with circuit breaker mechanism'
          )}
        </span>
      </TooltipContent>
    </Tooltip>
  )
}
