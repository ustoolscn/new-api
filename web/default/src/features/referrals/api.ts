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
  ReferralApiResponse,
  ReferralClaimResult,
  ReferralOverview,
} from './types'

export async function getReferralCode(): Promise<ReferralApiResponse<string>> {
  const response = await api.get('/api/user/aff')
  return response.data
}

export async function getReferralOverview(
  page: number,
  pageSize: number
): Promise<ReferralApiResponse<ReferralOverview>> {
  const response = await api.get('/api/user/referrals', {
    params: { p: page, page_size: pageSize },
  })
  return response.data
}

export async function claimReferralCommissions(): Promise<
  ReferralApiResponse<ReferralClaimResult>
> {
  const response = await api.post('/api/user/referrals/claim')
  return response.data
}

export async function claimInviteRewards(
  quota: number
): Promise<ReferralApiResponse<null>> {
  const response = await api.post('/api/user/aff_transfer', { quota })
  return response.data
}
