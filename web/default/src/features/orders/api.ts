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
import { api } from '@/lib/api'

import type {
  ApiResponse,
  CreateInvoiceRequestInput,
  InvoiceRequest,
  InvoiceStatus,
  IssueInvoiceRequestInput,
  PageData,
  ReviewInvoiceRequestInput,
  UserOrder,
} from './types'

type ListParams = {
  page: number
  pageSize: number
  status?: string
  keyword?: string
}

export async function getUserOrders(
  params: ListParams
): Promise<ApiResponse<PageData<UserOrder>>> {
  const response = await api.get('/api/user/orders', {
    params: {
      p: params.page,
      page_size: params.pageSize,
      keyword: params.keyword || undefined,
    },
  })
  return response.data
}

export async function createInvoiceRequest(
  input: CreateInvoiceRequestInput
): Promise<ApiResponse<InvoiceRequest>> {
  const response = await api.post('/api/user/invoices', input)
  return response.data
}

export async function getUserInvoiceRequests(
  params: ListParams
): Promise<ApiResponse<PageData<InvoiceRequest>>> {
  const response = await api.get('/api/user/invoices', {
    params: {
      p: params.page,
      page_size: params.pageSize,
      status: params.status || undefined,
    },
  })
  return response.data
}

export async function getAdminInvoiceRequests(
  params: ListParams
): Promise<ApiResponse<PageData<InvoiceRequest>>> {
  const response = await api.get('/api/user/invoices/admin', {
    params: {
      p: params.page,
      page_size: params.pageSize,
      status: params.status || undefined,
    },
  })
  return response.data
}

export async function reviewInvoiceRequest(
  requestId: number,
  input: ReviewInvoiceRequestInput
): Promise<ApiResponse<null>> {
  const response = await api.post(
    `/api/user/invoices/admin/${requestId}/review`,
    input
  )
  return response.data
}

export async function issueInvoiceRequest(
  requestId: number,
  input: IssueInvoiceRequestInput
): Promise<ApiResponse<null>> {
  const response = await api.post(
    `/api/user/invoices/admin/${requestId}/issue`,
    input
  )
  return response.data
}

export const invoiceStatuses: InvoiceStatus[] = [
  'pending',
  'approved',
  'rejected',
  'issued',
]
