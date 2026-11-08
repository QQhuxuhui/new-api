import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export type PolicyConfig = {
  options: Record<string, string>
}
export type PolicyDecision = { action: string; reason: string; source: string }
export type PolicyEvent = {
  attempt: number
  channel_id?: number
  group?: string
  rule?: string
  status?: number
  error_code?: string
  error_source?: string
  elapsed_ms: number
  decision: PolicyDecision
  health?: string
}
type Response<T> = { success: boolean; message?: string; data: T }
export async function getPolicyConfig() {
  const response = await api.get<Response<PolicyConfig>>(
    '/api/option/request_policy'
  )
  return requireServerSuccess(response.data).data
}
export async function savePolicyConfig(options: Record<string, string>) {
  const response = await api.patch<Response<PolicyConfig>>(
    '/api/option/request_policy',
    { options }
  )
  return requireServerSuccess(response.data).data
}

export type ErrorRuleTestResult = {
  rules: { id: string; name: string; enabled: boolean; matched: boolean }[]
  matched_rule_id: string
  error_type: '' | 'server' | 'client'
  return_immediately: boolean
  retry_count: number
}
export async function testErrorRules(input: {
  rules: string
  status_code: number
  message: string
}) {
  const response = await api.post<Response<ErrorRuleTestResult>>(
    '/api/option/request_policy/error_rules/test',
    input
  )
  return requireServerSuccess(response.data).data
}
