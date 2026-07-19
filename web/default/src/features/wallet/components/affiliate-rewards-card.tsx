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
import { Link } from '@tanstack/react-router'
import { ArrowRight, Share2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import type { ReferralOverview } from '@/features/referrals/types'
import { formatQuota } from '@/lib/format'

import type { UserWalletData } from '../types'

interface AffiliateRewardsCardProps {
  user: UserWalletData | null
  overview?: ReferralOverview
  loading?: boolean
}

export function AffiliateRewardsCard(props: AffiliateRewardsCardProps) {
  const { t } = useTranslation()
  if (props.loading) {
    return (
      <Card data-card-hover='false' className='bg-muted/20 py-0'>
        <CardContent className='grid gap-4 p-3 sm:p-4 lg:grid-cols-[minmax(220px,1fr)_minmax(220px,0.72fr)_minmax(320px,1.15fr)] lg:items-center'>
          <div>
            <Skeleton className='h-5 w-32' />
            <Skeleton className='mt-2 h-4 w-48' />
          </div>
          <Skeleton className='h-14 rounded-lg' />
          <Skeleton className='h-10 rounded-lg' />
        </CardContent>
      </Card>
    )
  }

  return (
    <Link
      to='/referrals'
      className='focus-visible:ring-ring block rounded-xl focus-visible:ring-2 focus-visible:outline-none'
      aria-label={t('View referral details')}
    >
      <Card className='bg-muted/20 hover:bg-muted/35 py-0 transition-colors'>
        <CardContent className='grid gap-4 p-3 sm:p-4 lg:grid-cols-[minmax(220px,1fr)_minmax(360px,1.2fr)_auto] lg:items-center'>
          <div className='flex min-w-0 items-center gap-2.5'>
            <IconBadge tone='chart-3'>
              <Share2 />
            </IconBadge>
            <div className='min-w-0'>
              <h3 className='truncate text-sm font-semibold'>
                {t('Referral Program')}
              </h3>
              <p className='text-muted-foreground line-clamp-2 text-xs'>
                {t(
                  'Earn a fixed reward for each successful invite plus commission from eligible top-ups.'
                )}
              </p>
            </div>
          </div>

          <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
            {[
              [t('Invite rewards'), formatQuota(props.user?.aff_quota ?? 0)],
              [
                t('Top-up commission'),
                formatQuota(props.overview?.pending_quota ?? 0),
              ],
              [
                t('Invited users'),
                String(
                  props.overview?.invite_count ?? props.user?.aff_count ?? 0
                ),
              ],
              [
                t('Commission rate'),
                `${props.overview?.commission_rate ?? 0}%`,
              ],
            ].map(([label, value]) => (
              <div key={label}>
                <div className='text-muted-foreground truncate text-[10px] font-medium tracking-wider uppercase'>
                  {label}
                </div>
                <div className='mt-0.5 truncate text-sm font-semibold tabular-nums'>
                  {value}
                </div>
              </div>
            ))}
          </div>

          <div className='text-primary flex items-center justify-end gap-2 text-sm font-medium'>
            {t('View details')}
            <ArrowRight className='size-4' />
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}
