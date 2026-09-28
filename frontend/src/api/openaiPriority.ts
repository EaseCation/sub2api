import { apiClient } from './client'

export interface OpenAIPrioritySettings {
  enabled: boolean
  threshold: number
}

export async function getOpenAIPriorityStatus(signal?: AbortSignal): Promise<{ active: boolean }> {
  const { data } = await apiClient.get<{ active: boolean }>('/settings/openai-priority', { signal })
  return data
}

export async function getOpenAIPrioritySettings(): Promise<OpenAIPrioritySettings> {
  const { data } = await apiClient.get<OpenAIPrioritySettings>('/admin/settings/openai-priority')
  return data
}

export async function saveOpenAIPrioritySettings(settings: OpenAIPrioritySettings): Promise<OpenAIPrioritySettings> {
  const { data } = await apiClient.put<OpenAIPrioritySettings>('/admin/settings/openai-priority', settings)
  window.dispatchEvent(new Event('openai-priority-updated'))
  return data
}
