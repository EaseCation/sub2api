import { apiClient } from './client'
import type { UsageQueryParams } from '@/types'

export interface RepositoryStat {
  repository: string
  requests: number
  total_tokens: number
  cost: number
  actual_cost: number
}

export interface RepositoryStatsPage {
  repositories: RepositoryStat[]
  page: number
  page_size: number
  has_more: boolean
  generated_at: string
}

export interface RepositoryUsageFilters extends UsageQueryParams {
  user_id?: number
  upstream_model_mismatch?: boolean
}

export async function getRepositoryUsage(
  params: RepositoryUsageFilters,
  admin: boolean,
  signal?: AbortSignal
): Promise<RepositoryStatsPage> {
  const { data } = await apiClient.get<RepositoryStatsPage>(
    admin ? '/admin/usage/repositories' : '/usage/repositories',
    { params, signal }
  )
  return data
}
