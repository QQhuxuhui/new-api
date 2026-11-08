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
import { useMutation } from '@tanstack/react-query'
import { FlaskConical } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'

import { testErrorRules, type ErrorRuleTestResult } from '../api'
import type { ErrorRule } from './types'

type ErrorRuleTestPanelProps = {
  rulesJson: string
  rules: ErrorRule[]
}

function resultSummary(
  t: (key: string, options?: Record<string, unknown>) => string,
  result: ErrorRuleTestResult,
  rules: ErrorRule[]
): string {
  if (!result.matched_rule_id) {
    return t('No rule matched. The retry status codes decide.')
  }
  const name =
    rules.find((rule) => rule.id === result.matched_rule_id)?.name ??
    result.matched_rule_id
  if (result.error_type === 'server') {
    return t('Matched "{{name}}": switch to another channel.', { name })
  }
  if (result.return_immediately) {
    return t('Matched "{{name}}": client error, return immediately.', { name })
  }
  return t('Matched "{{name}}": client error, keep retrying.', { name })
}

export function ErrorRuleTestPanel(props: ErrorRuleTestPanelProps) {
  const { t } = useTranslation()
  const [statusCode, setStatusCode] = useState('400')
  const [message, setMessage] = useState('')
  const mutation = useMutation({
    mutationFn: testErrorRules,
    onError: (error) => handleServerError(error),
    meta: { errorToast: false },
  })
  const code = Number(statusCode)
  const codeValid = Number.isInteger(code) && code >= 100 && code <= 599

  return (
    <div className='bg-muted/25 grid gap-3 px-4 py-4 sm:px-5'>
      <h4 className='text-sm font-medium'>{t('Test rules')}</h4>
      <div className='grid gap-3 sm:grid-cols-[8rem_1fr_auto] sm:items-end'>
        <div className='grid gap-1.5'>
          <Label htmlFor='error-rule-test-status'>{t('Status code')}</Label>
          <Input
            id='error-rule-test-status'
            type='number'
            min={100}
            max={599}
            value={statusCode}
            aria-invalid={!codeValid}
            onChange={(event) => setStatusCode(event.target.value)}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='error-rule-test-message'>{t('Error message')}</Label>
          <Textarea
            id='error-rule-test-message'
            rows={2}
            value={message}
            onChange={(event) => setMessage(event.target.value)}
          />
        </div>
        <Button
          variant='outline'
          disabled={!codeValid || mutation.isPending}
          onClick={() =>
            mutation.mutate({
              rules: props.rulesJson,
              status_code: code,
              message,
            })
          }
        >
          <FlaskConical aria-hidden='true' data-icon='inline-start' />
          {t('Test')}
        </Button>
      </div>
      {mutation.data ? (
        <p role='status' className='text-sm'>
          {resultSummary(t, mutation.data, props.rules)}
        </p>
      ) : null}
      <p className='text-muted-foreground text-xs'>
        {t('Tests the rules shown above, including unsaved changes.')}
      </p>
    </div>
  )
}
