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
import {
  Coins01Icon,
  UserMultiple02Icon,
  WalletAdd02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { formatQuota } from '@/lib/format'

import type { ReferralOverview } from '../types'

type ReferralRewardsCardProps = {
  overview?: ReferralOverview
  loading: boolean
  claimingInviteRewards: boolean
  claimingCommission: boolean
  onClaimInviteRewards: () => void
  onClaimCommission: () => void
}

type RewardMetricProps = {
  label: string
  value: string
}

function RewardMetric(props: RewardMetricProps) {
  return (
    <div>
      <p className='text-muted-foreground text-xs'>{props.label}</p>
      <p className='mt-1 font-semibold tabular-nums'>{props.value}</p>
    </div>
  )
}

export function ReferralRewardsCard(props: ReferralRewardsCardProps) {
  const { t } = useTranslation()
  const invitePending = props.overview?.invite_reward_pending_quota ?? 0
  const commissionPending = props.overview?.pending_quota ?? 0
  const claimEnabled = props.overview?.claim_enabled === true

  return (
    <Card data-card-hover='false'>
      <CardHeader>
        <CardTitle>{t('Referral rewards')}</CardTitle>
        <CardDescription>
          {t(
            'Earn a fixed reward for successful invitations and ongoing commission from eligible top-ups.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='grid gap-4 lg:grid-cols-2'>
        {[0, 1].map((index) =>
          props.loading ? (
            <Skeleton key={index} className='h-56 rounded-xl' />
          ) : null
        )}
        {!props.loading ? (
          <>
            <div className='bg-muted/20 flex flex-col gap-5 rounded-xl border p-4 sm:p-5'>
              <div className='flex items-start gap-3'>
                <IconBadge tone='primary' size='lg'>
                  <HugeiconsIcon icon={UserMultiple02Icon} strokeWidth={1.8} />
                </IconBadge>
                <div>
                  <h3 className='font-semibold'>{t('Registration rewards')}</h3>
                  <p className='text-muted-foreground mt-1 text-sm'>
                    {t('{{amount}} for each rewarded invite', {
                      amount: formatQuota(
                        props.overview?.invite_reward_quota ?? 0
                      ),
                    })}
                  </p>
                </div>
              </div>
              <div className='grid grid-cols-3 gap-3'>
                <RewardMetric
                  label={t('Pending rewards')}
                  value={formatQuota(invitePending)}
                />
                <RewardMetric
                  label={t('Total earned')}
                  value={formatQuota(
                    props.overview?.invite_reward_total_quota ?? 0
                  )}
                />
                <RewardMetric
                  label={t('Rewarded invites')}
                  value={String(props.overview?.rewarded_invite_count ?? 0)}
                />
              </div>
              <Button
                type='button'
                className='mt-auto w-full'
                disabled={
                  invitePending <= 0 ||
                  !claimEnabled ||
                  props.claimingInviteRewards
                }
                onClick={props.onClaimInviteRewards}
              >
                {props.claimingInviteRewards ? (
                  <Spinner data-icon='inline-start' />
                ) : (
                  <HugeiconsIcon
                    icon={WalletAdd02Icon}
                    strokeWidth={2}
                    data-icon='inline-start'
                  />
                )}
                {invitePending > 0
                  ? t('Claim {{amount}}', {
                      amount: formatQuota(invitePending),
                    })
                  : t('Nothing to claim')}
              </Button>
            </div>

            <div className='bg-muted/20 flex flex-col gap-5 rounded-xl border p-4 sm:p-5'>
              <div className='flex items-start gap-3'>
                <IconBadge tone='success' size='lg'>
                  <HugeiconsIcon icon={Coins01Icon} strokeWidth={1.8} />
                </IconBadge>
                <div>
                  <h3 className='font-semibold'>{t('Top-up commissions')}</h3>
                  <p className='text-muted-foreground mt-1 text-sm'>
                    {t('{{rate}}% of eligible credited top-ups', {
                      rate: props.overview?.commission_rate ?? 0,
                    })}
                  </p>
                </div>
              </div>
              <div className='grid grid-cols-3 gap-3'>
                <RewardMetric
                  label={t('Pending commission')}
                  value={formatQuota(commissionPending)}
                />
                <RewardMetric
                  label={t('Claimed')}
                  value={formatQuota(props.overview?.claimed_quota ?? 0)}
                />
                <RewardMetric
                  label={t('Total commission')}
                  value={formatQuota(props.overview?.total_quota ?? 0)}
                />
              </div>
              <Button
                type='button'
                className='mt-auto w-full'
                disabled={
                  commissionPending <= 0 ||
                  !claimEnabled ||
                  props.claimingCommission
                }
                onClick={props.onClaimCommission}
              >
                {props.claimingCommission ? (
                  <Spinner data-icon='inline-start' />
                ) : (
                  <HugeiconsIcon
                    icon={WalletAdd02Icon}
                    strokeWidth={2}
                    data-icon='inline-start'
                  />
                )}
                {commissionPending > 0
                  ? t('Claim {{amount}}', {
                      amount: formatQuota(commissionPending),
                    })
                  : t('Nothing to claim')}
              </Button>
            </div>
          </>
        ) : null}
        {!claimEnabled && (invitePending > 0 || commissionPending > 0) ? (
          <p className='text-muted-foreground text-sm lg:col-span-2'>
            {t(
              'Reward claiming is unavailable until payment compliance is confirmed.'
            )}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
