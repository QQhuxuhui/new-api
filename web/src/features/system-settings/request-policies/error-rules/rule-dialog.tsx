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
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useMemo } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'

import { SettingsSwitchField } from '../../components/settings-form-layout'
import { safeNumberFieldProps } from '../../utils/numeric-field'
import {
  createErrorRuleSchema,
  errorRuleFormValues,
  errorRuleFromForm,
  type ErrorRuleFormValues,
} from './form'
import { matchTypeLabel } from './labels'
import {
  ERROR_RULE_MATCH_TYPES,
  MAX_SAME_CHANNEL_RETRY_COUNT,
  MAX_SAME_CHANNEL_RETRY_INTERVAL_MS,
  type ErrorRule,
} from './types'

const FORM_ID = 'channel-error-rule-form'

type ErrorRuleDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  rule: ErrorRule
  isNew: boolean
  onSave: (rule: ErrorRule) => void
}

export function ErrorRuleDialog(props: ErrorRuleDialogProps) {
  const { t } = useTranslation()
  const schema = useMemo(() => createErrorRuleSchema(t), [t])
  const form = useForm<ErrorRuleFormValues>({
    resolver: zodResolver(schema),
    defaultValues: errorRuleFormValues(props.rule),
  })
  useEffect(() => {
    if (props.open) form.reset(errorRuleFormValues(props.rule))
  }, [props.open, props.rule, form])
  const errorType = useWatch({ control: form.control, name: 'error_type' })

  const save = form.handleSubmit((values) => {
    props.onSave(errorRuleFromForm(props.rule.id, values))
    props.onOpenChange(false)
  })

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.isNew ? t('Add error rule') : t('Edit error rule')}
      contentClassName='sm:max-w-3xl'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={FORM_ID}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={(event) => void save(event)}
          className='min-w-0 space-y-4'
        >
          <div className='grid gap-4 sm:grid-cols-[1fr_8rem]'>
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Name')}</FormLabel>
                  <FormControl>
                    <Input {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='priority'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Priority')}</FormLabel>
                  <FormControl>
                    <Input type='number' step={1} {...safeNumberFieldProps(field)} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
          <FormField
            control={form.control}
            name='match_type'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Match condition')}</FormLabel>
                <FormControl>
                  <NativeSelect
                    value={field.value}
                    onChange={(event) => field.onChange(event.target.value)}
                  >
                    {ERROR_RULE_MATCH_TYPES.map((matchType) => (
                      <NativeSelectOption key={matchType} value={matchType}>
                        {matchTypeLabel(t, matchType)}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <div className='grid gap-4 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='status_codes_text'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Status codes')}</FormLabel>
                  <FormControl>
                    <Input {...field} placeholder='400, 429' />
                  </FormControl>
                  <FormDescription>{t('Separate with commas.')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='keywords_text'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Keywords (one per line)')}</FormLabel>
                  <FormControl>
                    <Textarea rows={4} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Case-insensitive. Matched against the upstream error message and body.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
          <FormField
            control={form.control}
            name='error_type'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Error type')}</FormLabel>
                <FormControl>
                  <NativeSelect
                    value={field.value}
                    onChange={(event) => field.onChange(event.target.value)}
                  >
                    <NativeSelectOption value='server'>
                      {t('Channel error: switch to another channel')}
                    </NativeSelectOption>
                    <NativeSelectOption value='client'>
                      {t('Client error: do not disable the channel')}
                    </NativeSelectOption>
                  </NativeSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          {errorType === 'client' ? (
            <FormField
              control={form.control}
              name='return_immediately'
              render={({ field }) => (
                <SettingsSwitchField
                  controlId='error-rule-return-immediately'
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  label={t('Return the error immediately')}
                  description={t(
                    'Stop retrying and return the upstream error to the client.'
                  )}
                />
              )}
            />
          ) : null}
          <div className='grid gap-4 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='retry_count'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Same-channel retries')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={MAX_SAME_CHANNEL_RETRY_COUNT}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Retries on the current channel before switching. 0 disables.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='retry_interval_ms'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Retry interval (ms)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={MAX_SAME_CHANNEL_RETRY_INTERVAL_MS}
                      step={100}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
          <FormField
            control={form.control}
            name='description'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Description')}</FormLabel>
                <FormControl>
                  <Textarea rows={2} {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchField
                controlId='error-rule-enabled'
                checked={field.value}
                onCheckedChange={field.onChange}
                label={t('Enabled')}
              />
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}
