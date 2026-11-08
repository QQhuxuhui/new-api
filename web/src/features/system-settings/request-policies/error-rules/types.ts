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

export const ERROR_RULE_MATCH_TYPES = [
  'AND',
  'OR',
  'STATUS_ONLY',
  'KEYWORD_ONLY',
] as const
export type ErrorRuleMatchType = (typeof ERROR_RULE_MATCH_TYPES)[number]
export type ErrorRuleType = 'server' | 'client'

export const MAX_SAME_CHANNEL_RETRY_COUNT = 10
export const MAX_SAME_CHANNEL_RETRY_INTERVAL_MS = 30000

export type ErrorRule = {
  id: string
  name: string
  status_codes: number[]
  keywords: string[]
  match_type: ErrorRuleMatchType
  enabled: boolean
  priority: number
  error_type: ErrorRuleType
  return_immediately: boolean
  retry_count: number
  retry_interval_ms: number
  description: string
}

export function parseErrorRules(json: string): ErrorRule[] {
  try {
    const parsed: unknown = JSON.parse(json || '[]')
    return Array.isArray(parsed) ? (parsed as ErrorRule[]) : []
  } catch {
    return []
  }
}

export function serializeErrorRules(rules: ErrorRule[]): string {
  return JSON.stringify(rules)
}

/** Rules ordered as the gateway evaluates them: priority desc, then stored order. */
export function sortErrorRules(rules: ErrorRule[]): ErrorRule[] {
  return rules
    .map((rule, index) => ({ rule, index }))
    .sort((a, b) => b.rule.priority - a.rule.priority || a.index - b.index)
    .map((item) => item.rule)
}

export function newErrorRuleId(): string {
  return `rule-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
}

export function blankErrorRule(): ErrorRule {
  return {
    id: newErrorRuleId(),
    name: '',
    status_codes: [],
    keywords: [],
    match_type: 'KEYWORD_ONLY',
    enabled: true,
    priority: 0,
    error_type: 'server',
    return_immediately: false,
    retry_count: 0,
    retry_interval_ms: 0,
    description: '',
  }
}
