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

export type ReferralInvitedUser = {
  id: number
  username: string
  display_name: string
  created_at: number
  top_up_count: number
  recharge_quota_total: number
  commission_quota_total: number
  last_commission_at: number
}

export type ReferralInvitedUsersPage = {
  page: number
  page_size: number
  total: number
  items: ReferralInvitedUser[]
}

export type ReferralOverview = {
  commission_rate: number
  claim_enabled: boolean
  pending_quota: number
  claimed_quota: number
  total_quota: number
  invite_count: number
  rewarded_invite_count: number
  invite_reward_quota: number
  invite_reward_pending_quota: number
  invite_reward_total_quota: number
  invited_users: ReferralInvitedUsersPage
}

export type ReferralClaimResult = {
  claimed_quota: number
}

export type ReferralApiResponse<T> = {
  success: boolean
  message?: string
  data?: T
}
