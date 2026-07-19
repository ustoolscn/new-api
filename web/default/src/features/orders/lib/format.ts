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
import type { TFunction } from 'i18next'

import type { InvoiceStatus } from '../types'

export function formatOrderTimestamp(timestamp: number): string {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

export function getInvoiceStatusConfig(status: InvoiceStatus, t: TFunction) {
  switch (status) {
    case 'approved':
      return {
        label: t('Approved, awaiting invoice'),
        variant: 'info' as const,
      }
    case 'rejected':
      return { label: t('Rejected'), variant: 'danger' as const }
    case 'issued':
      return { label: t('Invoice issued'), variant: 'success' as const }
    default:
      return { label: t('Pending review'), variant: 'warning' as const }
  }
}
