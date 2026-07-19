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
import { z } from 'zod'

export const invoiceApplicationSchema = z.object({
  invoiceTitle: z
    .string()
    .trim()
    .min(1, 'Invoice title is required')
    .max(200, 'Invoice title must not exceed 200 characters'),
})

export type InvoiceApplicationFormValues = z.infer<
  typeof invoiceApplicationSchema
>

export const invoiceReviewSchema = z.object({
  remark: z
    .string()
    .trim()
    .max(500, 'Review remark must not exceed 500 characters'),
})

export type InvoiceReviewFormValues = z.infer<typeof invoiceReviewSchema>

export const invoiceURLSchema = z.object({
  invoiceURL: z
    .string()
    .trim()
    .min(1, 'Invoice URL is required')
    .max(2048, 'Invoice URL must not exceed 2048 characters')
    .refine((value) => {
      try {
        const url = new URL(value)
        return (
          (url.protocol === 'http:' || url.protocol === 'https:') &&
          url.hostname !== '' &&
          url.username === '' &&
          url.password === ''
        )
      } catch {
        return false
      }
    }, 'Enter a valid HTTP or HTTPS URL'),
})

export type InvoiceURLFormValues = z.infer<typeof invoiceURLSchema>
