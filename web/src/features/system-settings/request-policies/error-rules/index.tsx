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
import { Edit, ListFilter, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  DataTableRowActionMenu,
  StaticDataTable,
  TruncatedCell,
} from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenuGroup,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'

import { matchTypeLabel } from './labels'
import { ErrorRuleDialog } from './rule-dialog'
import { ErrorRuleTestPanel } from './test-panel'
import {
  blankErrorRule,
  parseErrorRules,
  serializeErrorRules,
  sortErrorRules,
  type ErrorRule,
} from './types'

type ErrorRulesSectionProps = {
  rulesJson: string
  onRulesChange: (rulesJson: string) => void
}

export function ErrorRulesSection(props: ErrorRulesSectionProps) {
  const { t } = useTranslation()
  const rules = useMemo(() => parseErrorRules(props.rulesJson), [props.rulesJson])
  const ordered = useMemo(() => sortErrorRules(rules), [rules])
  const [editing, setEditing] = useState<{ rule: ErrorRule; isNew: boolean } | null>(null)
  const [deleting, setDeleting] = useState<ErrorRule | null>(null)

  const replaceRule = (rule: ErrorRule) => {
    const index = rules.findIndex((item) => item.id === rule.id)
    const next = [...rules]
    if (index >= 0) next[index] = rule
    else next.push(rule)
    props.onRulesChange(serializeErrorRules(next))
  }

  return (
    <section
      aria-label={t('Error rules')}
      className='bg-card min-w-0 overflow-hidden rounded-xl border'
    >
      <div className='flex flex-wrap items-start justify-between gap-3 px-4 py-5 sm:px-5'>
        <div className='flex min-w-0 flex-col gap-1.5'>
          <div className='flex items-center gap-2'>
            <h3 className='text-base font-semibold'>{t('Error rules')}</h3>
            <Badge variant='secondary' className='tabular-nums'>
              {rules.length}
            </Badge>
          </div>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Classify upstream errors by status code and text before the retry status codes apply. The highest priority match wins.'
            )}
          </p>
        </div>
        <Button onClick={() => setEditing({ rule: blankErrorRule(), isNew: true })}>
          <Plus aria-hidden='true' data-icon='inline-start' />
          {t('Add error rule')}
        </Button>
      </div>
      <Separator />
      {ordered.length === 0 ? (
        <EmptyState
          icon={ListFilter}
          title={t('No error rules')}
          description={t('Errors follow the retry status codes only.')}
          className='min-h-40'
        />
      ) : (
        <StaticDataTable
          className='overflow-x-auto rounded-none border-0'
          containerProps={{ role: 'region', 'aria-label': t('Error rules table'), tabIndex: 0 }}
          tableProps={{ withContainer: false }}
          tableClassName='min-w-[960px] table-fixed [&_th]:px-4 [&_th]:text-muted-foreground [&_td]:px-4 [&_td]:py-3'
          headerRowClassName='bg-muted/35 hover:bg-muted/35'
          data={ordered}
          getRowKey={(rule) => rule.id}
          columns={[
            {
              id: 'enabled',
              header: t('Enabled'),
              className: 'w-20',
              cell: (rule) => (
                <Switch
                  aria-label={t('Enable {{name}}', { name: rule.name })}
                  checked={rule.enabled}
                  onCheckedChange={(checked) => replaceRule({ ...rule, enabled: checked })}
                />
              ),
            },
            {
              id: 'name',
              header: t('Name'),
              className: 'w-[22%]',
              cell: (rule) => (
                <div className='flex min-w-0 flex-col gap-1'>
                  <TruncatedCell tabIndex={0}>{rule.name}</TruncatedCell>
                  <span className='text-muted-foreground text-xs' data-table-text='secondary'>
                    {t('Priority')} {rule.priority}
                  </span>
                </div>
              ),
            },
            {
              id: 'condition',
              header: t('Match condition'),
              cell: (rule) => (
                <div className='flex min-w-0 flex-col gap-1'>
                  <span className='text-sm'>{matchTypeLabel(t, rule.match_type)}</span>
                  {rule.status_codes?.length ? (
                    <code className='text-xs'>{rule.status_codes.join(', ')}</code>
                  ) : null}
                  {rule.keywords?.length ? (
                    <TruncatedCell tabIndex={0} tooltipContent={rule.keywords.join('\n')}>
                      <span className='text-muted-foreground text-xs'>
                        {rule.keywords.join(' · ')}
                      </span>
                    </TruncatedCell>
                  ) : null}
                </div>
              ),
            },
            {
              id: 'action',
              header: t('Action'),
              className: 'w-[24%]',
              cell: (rule) => <RuleActionSummary rule={rule} />,
            },
            {
              id: 'actions',
              header: t('Actions'),
              className: 'w-24 text-right',
              cell: (rule) => (
                <div
                  role='group'
                  aria-label={t('Actions for {{name}}', { name: rule.name })}
                  className='flex items-center justify-end gap-0.5'
                >
                  <Button
                    variant='ghost'
                    size='icon'
                    aria-label={t('Edit error rule')}
                    onClick={() => setEditing({ rule, isNew: false })}
                  >
                    <Edit aria-hidden='true' />
                  </Button>
                  <DataTableRowActionMenu ariaLabel={t('More actions')}>
                    <DropdownMenuGroup>
                      <DropdownMenuItem variant='destructive' onClick={() => setDeleting(rule)}>
                        <Trash2 aria-hidden='true' />
                        {t('Delete Rule')}
                      </DropdownMenuItem>
                    </DropdownMenuGroup>
                  </DataTableRowActionMenu>
                </div>
              ),
            },
          ]}
        />
      )}
      <Separator />
      <ErrorRuleTestPanel rulesJson={props.rulesJson} rules={rules} />
      {editing ? (
        <ErrorRuleDialog
          open
          onOpenChange={(open) => {
            if (!open) setEditing(null)
          }}
          rule={editing.rule}
          isNew={editing.isNew}
          onSave={replaceRule}
        />
      ) : null}
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null)
        }}
        title={t('Delete Rule')}
        desc={t('Delete the rule "{{name}}"? Save the settings to apply.', {
          name: deleting?.name ?? '',
        })}
        destructive
        handleConfirm={() => {
          if (deleting) {
            props.onRulesChange(
              serializeErrorRules(rules.filter((rule) => rule.id !== deleting.id))
            )
          }
          setDeleting(null)
        }}
      />
    </section>
  )
}

function RuleActionSummary(props: { rule: ErrorRule }) {
  const { t } = useTranslation()
  let label = t('Switch channel')
  let variant: 'warning' | 'info' | 'danger' = 'warning'
  if (props.rule.error_type === 'client') {
    label = props.rule.return_immediately
      ? t('Client error, return immediately')
      : t('Client error, keep retrying')
    variant = props.rule.return_immediately ? 'danger' : 'info'
  }
  return (
    <div className='flex min-w-0 flex-col gap-1'>
      <StatusBadge label={label} variant={variant} copyable={false} />
      {props.rule.retry_count > 0 ? (
        <span className='text-muted-foreground text-xs'>
          {t('Retry the same channel {{count}} times, every {{interval}} ms', {
            count: props.rule.retry_count,
            interval: props.rule.retry_interval_ms,
          })}
        </span>
      ) : null}
    </div>
  )
}
