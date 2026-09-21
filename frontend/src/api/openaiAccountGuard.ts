import { apiClient } from './client'

export interface OpenAIAccountGuardSettings {
  enabled: boolean
  min_available_accounts: number
  check_interval_seconds: number
  message: string
  exempt_paths: string[]
}

export interface OpenAIAccountGuardStatus {
  locked: boolean
  message: string
}

export async function getOpenAIAccountGuardSettings(): Promise<OpenAIAccountGuardSettings> {
  const { data } = await apiClient.get<OpenAIAccountGuardSettings>('/admin/settings/openai-account-guard')
  return data
}

export async function saveOpenAIAccountGuardSettings(settings: OpenAIAccountGuardSettings): Promise<OpenAIAccountGuardSettings> {
  const { data } = await apiClient.put<OpenAIAccountGuardSettings>('/admin/settings/openai-account-guard', settings)
  return data
}

export async function getOpenAIAccountGuardStatus(): Promise<OpenAIAccountGuardStatus> {
  const { data } = await apiClient.get<OpenAIAccountGuardStatus>('/settings/openai-account-guard/status', { timeout: 10000 })
  return data
}
