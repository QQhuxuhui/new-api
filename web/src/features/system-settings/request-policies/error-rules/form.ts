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
import type { TFunction } from 'i18next'
import { z } from 'zod'

import {
  ERROR_RULE_MATCH_TYPES,
  MAX_SAME_CHANNEL_RETRY_COUNT,
  MAX_SAME_CHANNEL_RETRY_INTERVAL_MS,
  type ErrorRule,
} from './types'

function splitLines(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0)
}

function parseStatusCodes(text: string): number[] | null {
  const codes: number[] = []
  for (const token of text.split(/[\s,]+/)) {
    if (!token) continue
    const code = Number(token)
    if (!Number.isInteger(code) || code < 100 || code > 599) return null
    codes.push(code)
  }
  return codes
}

export function createErrorRuleSchema(t: TFunction) {
  return z
    .object({
      name: z.string().trim().min(1, t('Name is required')),
      status_codes_text: z
        .string()
        .refine(
          (value) => parseStatusCodes(value) !== null,
          t('Status codes must be integers between 100 and 599')
        ),
      keywords_text: z.string(),
      match_type: z.enum(ERROR_RULE_MATCH_TYPES),
      enabled: z.boolean(),
      priority: z.number().int(),
      error_type: z.enum(['server', 'client']),
      return_immediately: z.boolean(),
      retry_count: z.number().int().min(0).max(MAX_SAME_CHANNEL_RETRY_COUNT),
      retry_interval_ms: z
        .number()
        .int()
        .min(0)
        .max(MAX_SAME_CHANNEL_RETRY_INTERVAL_MS),
      description: z.string(),
    })
    .superRefine((values, context) => {
      const hasCodes = (parseStatusCodes(values.status_codes_text) ?? [])
        .length
      const hasKeywords = splitLines(values.keywords_text).length
      const needsCodes =
        values.match_type === 'AND' || values.match_type === 'STATUS_ONLY'
      const needsKeywords =
        values.match_type === 'AND' || values.match_type === 'KEYWORD_ONLY'
      if (needsCodes && !hasCodes) {
        context.addIssue({
          code: 'custom',
          path: ['status_codes_text'],
          message: t('This match type needs at least one status code'),
        })
      }
      if (needsKeywords && !hasKeywords) {
        context.addIssue({
          code: 'custom',
          path: ['keywords_text'],
          message: t('This match type needs at least one keyword'),
        })
      }
      if (values.match_type === 'OR' && !hasCodes && !hasKeywords) {
        context.addIssue({
          code: 'custom',
          path: ['keywords_text'],
          message: t('Add a status code or a keyword'),
        })
      }
    })
}

export type ErrorRuleFormValues = z.infer<
  ReturnType<typeof createErrorRuleSchema>
>

export function errorRuleFormValues(rule: ErrorRule): ErrorRuleFormValues {
  return {
    name: rule.name,
    status_codes_text: (rule.status_codes ?? []).join(', '),
    keywords_text: (rule.keywords ?? []).join('\n'),
    match_type: rule.match_type,
    enabled: rule.enabled,
    priority: rule.priority ?? 0,
    error_type: rule.error_type,
    return_immediately: rule.return_immediately,
    retry_count: rule.retry_count ?? 0,
    retry_interval_ms: rule.retry_interval_ms ?? 0,
    description: rule.description ?? '',
  }
}

export function errorRuleFromForm(
  id: string,
  values: ErrorRuleFormValues
): ErrorRule {
  return {
    id,
    name: values.name.trim(),
    status_codes: parseStatusCodes(values.status_codes_text) ?? [],
    keywords: splitLines(values.keywords_text),
    match_type: values.match_type,
    enabled: values.enabled,
    priority: values.priority,
    error_type: values.error_type,
    return_immediately:
      values.error_type === 'client' && values.return_immediately,
    retry_count: values.retry_count,
    retry_interval_ms: values.retry_interval_ms,
    description: values.description.trim(),
  }
}
