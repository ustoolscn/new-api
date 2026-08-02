import { useQuery } from '@tanstack/react-query'
import {
  ChartColumn,
  Percent,
  UserRoundCheck,
  UsersRound,
  WalletCards,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { InviteeConsumeCard } from '@/features/referrals/components/invitee-consume-card'
import { ReferralRewardsCard } from '@/features/referrals/components/referral-rewards-card'
import { formatQuota } from '@/lib/format'

import { getAdminReferralOverview } from '../api'
import type { ReferralInviterSummary } from '../types'

const DETAIL_PAGE_SIZE = 10

type UserReferralDrawerProps = {
  user: ReferralInviterSummary | null
  onOpenChange: (open: boolean) => void
}

export function UserReferralDrawer(props: UserReferralDrawerProps) {
  const { t } = useTranslation()
    const userId = props.user?.id ?? 0

  const overviewQuery = useQuery({
    queryKey: ['admin-referral-overview', userId, DETAIL_PAGE_SIZE],
    queryFn: async () => {
      const response = await getAdminReferralOverview(
        userId,
        1,
        DETAIL_PAGE_SIZE
      )
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to load referral information')
        )
      }
      return response.data
    },
    enabled: userId > 0,
  })

  const overview = overviewQuery.data
  const stats = [
    {
      label: t('Invited users'),
      value: String(overview?.invite_count ?? 0),
      icon: UsersRound,
    },
    {
      label: t('Rewarded invites'),
      value: String(overview?.rewarded_invite_count ?? 0),
      icon: UserRoundCheck,
    },
    {
      label: t('Total invitee consumption'),
      value: formatQuota(overview?.invitee_consume_total ?? 0),
      icon: ChartColumn,
    },
    {
      label: t('Total earned'),
      value: formatQuota(
        (overview?.invite_reward_total_quota ?? 0) +
          (overview?.total_quota ?? 0)
      ),
      icon: WalletCards,
    },
    {
      label: t('Commission rate'),
      value: `${overview?.commission_rate ?? 0}%`,
      icon: Percent,
    },
  ]

  return (
    <Sheet
      open={props.user !== null}
      onOpenChange={props.onOpenChange}
    >
      <SheetContent className={sideDrawerContentClassName('sm:max-w-5xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Referral details')}</SheetTitle>
          <SheetDescription>
            {props.user
              ? t('Referral details for {{username}} (ID: {{id}}).', {
                  username: props.user.display_name || props.user.username,
                  id: props.user.id,
                })
              : null}
          </SheetDescription>
        </SheetHeader>

        <div className={sideDrawerFormClassName('gap-4')}>
          <div className='grid shrink-0 gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-5'>
            {stats.map((stat) => (
              <Card key={stat.label} size='sm' data-card-hover='false'>
                <CardContent className='flex items-center justify-between gap-3'>
                  <div>
                    <CardDescription>{stat.label}</CardDescription>
                    {overviewQuery.isLoading ? (
                      <Skeleton className='mt-2 h-7 w-24' />
                    ) : (
                      <CardTitle className='mt-2 text-xl tabular-nums'>
                        {stat.value}
                      </CardTitle>
                    )}
                  </div>
                  <stat.icon className='text-muted-foreground size-5' />
                </CardContent>
              </Card>
            ))}
          </div>

          {overviewQuery.isError ? (
            <Card data-card-hover='false'>
              <CardHeader>
                <CardTitle>
                  {t('Failed to load referral information')}
                </CardTitle>
                <CardDescription>{overviewQuery.error.message}</CardDescription>
              </CardHeader>
              <CardContent>
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => overviewQuery.refetch()}
                >
                  {t('Retry')}
                </Button>
              </CardContent>
            </Card>
          ) : (
            <>
              <div className='shrink-0'>
                <ReferralRewardsCard
                  overview={overview}
                  loading={overviewQuery.isLoading}
                  readonly
                />
              </div>
              <div className='shrink-0'>
                <InviteeConsumeCard inviterId={userId} />
              </div>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
