/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export type ServiceStatusGranularity = 'hour' | 'day'

export type ServiceStatusPeriod = {
  bucket_start: number
  bucket_label: string
}

export type ServiceStatusPoint = {
  request_count: number
  success_count: number
  success_rate: number | null
  avg_ttft_ms: number | null
}

export type ServiceStatusMetric = {
  name: string
  request_count: number
  success_count: number
  success_rate: number | null
  avg_ttft_ms: number | null
  series: ServiceStatusPoint[]
}

export type ServiceStatusSnapshot = {
  generated_at: number
  start_timestamp: number
  end_timestamp: number
  granularity: ServiceStatusGranularity
  is_current_period: boolean
  periods: ServiceStatusPeriod[]
  overall: ServiceStatusMetric
  models: ServiceStatusMetric[]
  groups: ServiceStatusMetric[]
  models_total: number
  groups_total: number
  models_truncated: boolean
  groups_truncated: boolean
}

export type ServiceStatusResponse = {
  success: boolean
  message?: string
  data?: ServiceStatusSnapshot
}
