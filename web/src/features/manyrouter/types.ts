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
export type ManyRouterProduct = {
  model: string
  kind: 'dedicated' | 'fixed_auto'
  strategy_kind?:
    | 'lowest_price'
    | 'low_latency'
    | 'high_sla'
    | 'high_quality'
    | 'balanced'
    | 'custom'
  display_name: string
  group_key: string
  entry_open: boolean
  sale_ratio: string
  price_version_id?: string | null
  price_confirmed_at?: string | null
  available_suppliers: number
  failover_ready: boolean
  request_samples: number
  sla_percent?: number | null
  ttft_p50_ms?: number | null
  ttft_p95_ms?: number | null
  quality_grade: 'excellent' | 'good' | 'fair' | 'poor' | 'insufficient'
  confidence: 'high' | 'medium' | 'low' | 'insufficient'
  facts_through?: string | null
  status: 'available' | 'insufficient' | 'stale' | 'unavailable' | 'closed'
}

export type ManyRouterSnapshot = {
  id: string
  contract_version: 'm3-site-products-v1'
  version: number
  site_id: string
  site_name: string
  route_plan_id: string
  score_run_id?: string | null
  generated_at: string
  facts_through?: string | null
  products: ManyRouterProduct[]
  content_hash: string
}

export type ManyRouterResponse = {
  success: boolean
  message?: string
  data?: ManyRouterSnapshot
}
