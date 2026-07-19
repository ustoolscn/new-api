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
  Copy01Icon,
  MoneyReceive02Icon,
  PercentCircleIcon,
  UserMultiple02Icon,
  WalletAdd02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Skeleton } from '@/components/ui/skeleton'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getSelf } from '@/lib/api'
import { formatQuota } from '@/lib/format'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import {
  claimInviteRewards,
  claimReferralCommissions,
  getReferralCode,
  getReferralOverview,
} from './api'
import { InvitedUsersCard } from './components/invited-users-card'
import { ReferralRewardsCard } from './components/referral-rewards-card'

const PAGE_SIZE = 10

export function Referrals() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const setUser = useAuthStore((state) => state.auth.setUser)
  const [page, setPage] = useState(1)
  const { copyToClipboard } = useCopyToClipboard()

  const codeQuery = useQuery({
    queryKey: ['referral-code'],
    queryFn: async () => {
      const response = await getReferralCode()
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load referral code'))
      }
      return response.data
    },
  })

  const overviewQuery = useQuery({
    queryKey: ['referrals', page, PAGE_SIZE],
    queryFn: async () => {
      const response = await getReferralOverview(page, PAGE_SIZE)
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to load referral information')
        )
      }
      return response.data
    },
    placeholderData: keepPreviousData,
  })

  const refreshCurrentUser = async () => {
    const response = await getSelf()
    if (response.success && response.data) {
      setUser(response.data as AuthUser)
    }
  }

  const refreshReferralData = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['referrals'] }),
      refreshCurrentUser(),
    ])
  }

  const inviteRewardMutation = useMutation({
    mutationFn: claimInviteRewards,
    onSuccess: async (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to claim invite rewards'))
        return
      }
      toast.success(response.message || t('Invite rewards claimed'))
      await refreshReferralData()
    },
    onError: () => {
      toast.error(t('Failed to claim invite rewards'))
    },
  })

  const claimMutation = useMutation({
    mutationFn: claimReferralCommissions,
    onSuccess: async (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to claim commission'))
        return
      }
      toast.success(response.message || t('Commission claimed'))
      await refreshReferralData()
    },
    onError: () => {
      toast.error(t('Failed to claim commission'))
    },
  })

  const referralLink =
    codeQuery.data && typeof window !== 'undefined'
      ? `${window.location.origin}/sign-up?aff=${codeQuery.data}`
      : ''
  const overview = overviewQuery.data
  const stats = [
    {
      label: t('Invited users'),
      value: String(overview?.invite_count ?? 0),
      icon: UserMultiple02Icon,
      tone: 'info' as const,
    },
    {
      label: t('Rewarded invites'),
      value: String(overview?.rewarded_invite_count ?? 0),
      icon: WalletAdd02Icon,
      tone: 'warning' as const,
    },
    {
      label: t('Reward per invite'),
      value: formatQuota(overview?.invite_reward_quota ?? 0),
      icon: Coins01Icon,
      tone: 'success' as const,
    },
    {
      label: t('Commission rate'),
      value: `${overview?.commission_rate ?? 0}%`,
      icon: PercentCircleIcon,
      tone: 'primary' as const,
    },
  ]

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Referrals')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t(
          'Invite users, earn registration rewards, and receive commission from their eligible top-ups.'
        )}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
          <Card data-card-hover='false'>
            <CardHeader>
              <div className='flex items-start gap-3'>
                <IconBadge tone='primary' size='lg'>
                  <HugeiconsIcon icon={MoneyReceive02Icon} strokeWidth={1.8} />
                </IconBadge>
                <div>
                  <CardTitle>{t('Your referral link')}</CardTitle>
                  <CardDescription className='mt-1 max-w-2xl'>
                    {t(
                      'Users who register through this link are connected to your account. Successful invitations earn a fixed reward, and eligible top-ups generate commission.'
                    )}
                  </CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {codeQuery.isLoading ? (
                <Skeleton className='h-10 w-full' />
              ) : (
                <InputGroup className='h-10'>
                  <InputGroupInput
                    value={referralLink}
                    readOnly
                    aria-label={t('Referral link')}
                    className='font-mono text-sm'
                  />
                  <InputGroupAddon align='inline-end'>
                    <InputGroupButton
                      size='icon-sm'
                      onClick={() => copyToClipboard(referralLink)}
                      disabled={!referralLink}
                      aria-label={t('Copy referral link')}
                    >
                      <HugeiconsIcon icon={Copy01Icon} strokeWidth={2} />
                    </InputGroupButton>
                  </InputGroupAddon>
                </InputGroup>
              )}
            </CardContent>
          </Card>

          <div className='grid gap-4 sm:grid-cols-2 xl:grid-cols-4'>
            {stats.map((stat) => (
              <Card key={stat.label} data-card-hover='false'>
                <CardHeader className='gap-3'>
                  <div className='flex items-center justify-between gap-3'>
                    <CardDescription>{stat.label}</CardDescription>
                    <IconBadge tone={stat.tone} size='sm'>
                      <HugeiconsIcon icon={stat.icon} strokeWidth={2} />
                    </IconBadge>
                  </div>
                  {overviewQuery.isLoading ? (
                    <Skeleton className='h-8 w-28' />
                  ) : (
                    <CardTitle className='text-2xl tabular-nums'>
                      {stat.value}
                    </CardTitle>
                  )}
                </CardHeader>
              </Card>
            ))}
          </div>

          <ReferralRewardsCard
            overview={overview}
            loading={overviewQuery.isLoading}
            claimingInviteRewards={inviteRewardMutation.isPending}
            claimingCommission={claimMutation.isPending}
            onClaimInviteRewards={() =>
              inviteRewardMutation.mutate(
                overview?.invite_reward_pending_quota ?? 0
              )
            }
            onClaimCommission={() => claimMutation.mutate()}
          />

          {overviewQuery.isError ? (
            <Card data-card-hover='false'>
              <CardContent className='flex flex-col items-center gap-3 py-10 text-center'>
                <p className='font-medium'>
                  {t('Failed to load referral information')}
                </p>
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
            <InvitedUsersCard
              data={overview?.invited_users}
              loading={overviewQuery.isLoading || overviewQuery.isFetching}
              page={page}
              onPageChange={setPage}
            />
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
