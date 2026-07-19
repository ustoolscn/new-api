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

export type InvoiceStatus = 'pending' | 'approved' | 'rejected' | 'issued'

export type UserOrder = {
  id: number
  invoice_source_id: number
  order_type: 'recharge' | 'subscription'
  plan_id: number
  product_name: string
  amount: number
  money: number
  trade_no: string
  payment_method: string
  payment_provider: string
  create_time: number
  complete_time: number
  status: 'success'
  invoice_request_id: number
  invoice_status: InvoiceStatus | ''
  invoice_eligible: boolean
}

export type InvoiceOrder = {
  top_up_id: number
  trade_no: string
  payment_method: string
  amount: number
  complete_time: number
}

export type InvoiceRequest = {
  id: number
  user_id: number
  username: string
  display_name: string
  invoice_title: string
  amount: number
  status: InvoiceStatus
  review_remark: string
  reviewed_by: number
  reviewed_at: number
  issued_at: number
  created_at: number
  updated_at: number
  order_count: number
  orders: InvoiceOrder[]
  invoice_url: string
  download_available: boolean
}

export type PageData<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export type ApiResponse<T> = {
  success: boolean
  message?: string
  data?: T
}

export type CreateInvoiceRequestInput = {
  invoice_title: string
  top_up_ids: number[]
}

export type ReviewInvoiceRequestInput = {
  action: 'approve' | 'reject'
  remark: string
}

export type IssueInvoiceRequestInput = {
  invoice_url: string
}
