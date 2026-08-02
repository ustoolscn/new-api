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
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Search01Icon,
  UserMultiple02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import dayjs from '@/lib/dayjs'
import { formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  getAdminInviteeConsumeReport,
  getInviteeConsumeReport,
} from '../api'

const PAGE_SIZE = 10
const DATE_FORMAT = 'YYYY-MM-DD'

type RangePreset = 'this_month' | 'last_month' | 'last_30_days' | 'last_year'

type InviteeConsumeCardProps = {
  inviterId?: number
  className?: string
}

function getPresetRange(preset: RangePreset) {
  const now = dayjs()
  switch (preset) {
    case 'last_month': {
      const start = now.subtract(1, 'month').startOf('month')
      return {
        start: start.format(DATE_FORMAT),
        end: start.endOf('month').format(DATE_FORMAT),
      }
    }
    case 'last_30_days':
      return {
        start: now.subtract(29, 'day').startOf('day').format(DATE_FORMAT),
        end: now.format(DATE_FORMAT),
      }
    case 'last_year':
      return {
        start: now
          .subtract(1, 'year')
          .add(1, 'day')
          .startOf('day')
          .format(DATE_FORMAT),
        end: now.format(DATE_FORMAT),
      }
    case 'this_month':
    default:
      return {
        start: now.startOf('month').format(DATE_FORMAT),
        end: now.format(DATE_FORMAT),
      }
  }
}

function toQueryRange(startDate: string, endDate: string) {
  const start = dayjs(startDate).startOf('day')
  const end = dayjs(endDate).add(1, 'day').startOf('day')
  return {
    startTimestamp: start.unix(),
    endTimestamp: end.unix(),
    valid: start.isValid() && end.isValid() && end.unix() > start.unix(),
  }
}

function formatTimestamp(timestamp: number, emptyText: string): string {
  if (!timestamp) return emptyText
  return new Date(timestamp * 1000).toLocaleString()
}

export function InviteeConsumeCard(props: InviteeConsumeCardProps) {
  const { t } = useTranslation()
  const initialRange = getPresetRange('this_month')
  const [draftStartDate, setDraftStartDate] = useState(initialRange.start)
  const [draftEndDate, setDraftEndDate] = useState(initialRange.end)
  const [activePreset, setActivePreset] = useState<RangePreset | null>(
    'this_month'
  )
  const [appliedStartDate, setAppliedStartDate] = useState(initialRange.start)
  const [appliedEndDate, setAppliedEndDate] = useState(initialRange.end)
  const [page, setPage] = useState(1)

  const appliedRange = useMemo(
    () => toQueryRange(appliedStartDate, appliedEndDate),
    [appliedEndDate, appliedStartDate]
  )

  const reportQuery = useQuery({
    queryKey: [
      'invitee-activity-report',
      props.inviterId ?? 'self',
      appliedRange.startTimestamp,
      appliedRange.endTimestamp,
      page,
      PAGE_SIZE,
    ],
    queryFn: async () => {
      const request = {
        startTimestamp: appliedRange.startTimestamp,
        endTimestamp: appliedRange.endTimestamp,
        page,
        pageSize: PAGE_SIZE,
      }
      const response = props.inviterId
        ? await getAdminInviteeConsumeReport(props.inviterId, request)
        : await getInviteeConsumeReport(request)
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to load invitee consumption')
        )
      }
      return response.data
    },
    enabled:
      appliedRange.valid && (props.inviterId == null || props.inviterId > 0),
    placeholderData: keepPreviousData,
  })

  const applyDraftRange = () => {
    const next = toQueryRange(draftStartDate, draftEndDate)
    if (!next.valid) {
      toast.error(t('Please select a valid date range'))
      return
    }
    setAppliedStartDate(draftStartDate)
    setAppliedEndDate(draftEndDate)
    setPage(1)
  }

  const applyPreset = (preset: RangePreset) => {
    const range = getPresetRange(preset)
    setDraftStartDate(range.start)
    setDraftEndDate(range.end)
    setActivePreset(preset)
    setAppliedStartDate(range.start)
    setAppliedEndDate(range.end)
    setPage(1)
  }

  const items = reportQuery.data?.users?.items ?? []
  const total = reportQuery.data?.users?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const presets: Array<{ value: RangePreset; label: string }> = [
    { value: 'this_month', label: t('This Month') },
    { value: 'last_month', label: t('Last Month') },
    { value: 'last_30_days', label: t('Last 30 Days') },
    { value: 'last_year', label: t('Last Year') },
  ]

  const summaryCards = [
    {
      label: t('Top-ups'),
      value: String(reportQuery.data?.top_up_count_total ?? 0),
    },
    {
      label: t('Credited balance'),
      value: formatQuota(reportQuery.data?.recharge_quota_total ?? 0),
    },
    {
      label: t('Commission earned'),
      value: formatQuota(reportQuery.data?.commission_quota_total ?? 0),
    },
    {
      label: t('Period total consumption'),
      value: formatQuota(reportQuery.data?.range_consume_total ?? 0),
    },
  ]

  return (
    <Card className={props.className} data-card-hover='false'>
      <CardHeader className='shrink-0 space-y-4'>
        <div className='flex items-start gap-3'>
          <IconBadge tone='primary' size='lg'>
            <HugeiconsIcon icon={UserMultiple02Icon} strokeWidth={1.8} />
          </IconBadge>
          <div className='min-w-0 flex-1'>
            <CardTitle>{t('Invited users')}</CardTitle>
            <CardDescription className='mt-1'>
              {t(
                'Filter invited users by date range. Top-ups, credited balance, commission, and consumption all use the same selected period.'
              )}
            </CardDescription>
          </div>
        </div>

        <div className='flex flex-wrap gap-2'>
          {presets.map((preset) => (
            <Button
              key={preset.value}
              type='button'
              size='sm'
              variant={activePreset === preset.value ? 'default' : 'outline'}
              onClick={() => applyPreset(preset.value)}
            >
              {preset.label}
            </Button>
          ))}
        </div>

        <div className='grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end'>
          <div className='space-y-1.5'>
            <label
              className='text-muted-foreground text-xs'
              htmlFor='invitee-activity-start'
            >
              {t('Start Date')}
            </label>
            <Input
              id='invitee-activity-start'
              type='date'
              value={draftStartDate}
              onChange={(event) => {
                setDraftStartDate(event.target.value)
                setActivePreset(null)
              }}
            />
          </div>
          <div className='space-y-1.5'>
            <label
              className='text-muted-foreground text-xs'
              htmlFor='invitee-activity-end'
            >
              {t('End Date')}
            </label>
            <Input
              id='invitee-activity-end'
              type='date'
              value={draftEndDate}
              onChange={(event) => {
                setDraftEndDate(event.target.value)
                setActivePreset(null)
              }}
            />
          </div>
          <Button type='button' onClick={applyDraftRange} className='sm:mb-0.5'>
            <HugeiconsIcon
              icon={Search01Icon}
              strokeWidth={2}
              data-icon='inline-start'
            />
            {t('Search')}
          </Button>
        </div>

        <p className='text-muted-foreground text-xs tabular-nums'>
          {t('Selected period')}: {appliedStartDate} ~ {appliedEndDate}
        </p>
      </CardHeader>

      <CardContent className={cn('space-y-4 px-0')}>
        <div className='grid gap-3 px-6 sm:grid-cols-2 xl:grid-cols-4'>
          {summaryCards.map((card) => (
            <div key={card.label} className='rounded-xl border px-4 py-3'>
              <p className='text-muted-foreground text-sm'>{card.label}</p>
              {reportQuery.isLoading ? (
                <Skeleton className='mt-2 h-7 w-24' />
              ) : (
                <p className='mt-1 text-xl font-semibold tabular-nums'>
                  {card.value}
                </p>
              )}
            </div>
          ))}
        </div>

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='pl-6'>{t('User')}</TableHead>
              <TableHead>{t('Joined at')}</TableHead>
              <TableHead className='text-right'>{t('Top-ups')}</TableHead>
              <TableHead className='text-right'>
                {t('Credited balance')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Commission earned')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Period consumption')}
              </TableHead>
              <TableHead className='pr-6 text-right'>
                {t('Last commission')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {reportQuery.isLoading
              ? Array.from({ length: 5 }, (_, index) => (
                  <TableRow key={index}>
                    <TableCell className='pl-6'>
                      <Skeleton className='h-8 w-36' />
                    </TableCell>
                    <TableCell>
                      <Skeleton className='h-4 w-28' />
                    </TableCell>
                    <TableCell>
                      <Skeleton className='ml-auto h-4 w-10' />
                    </TableCell>
                    <TableCell>
                      <Skeleton className='ml-auto h-4 w-20' />
                    </TableCell>
                    <TableCell>
                      <Skeleton className='ml-auto h-4 w-20' />
                    </TableCell>
                    <TableCell>
                      <Skeleton className='ml-auto h-4 w-20' />
                    </TableCell>
                    <TableCell className='pr-6'>
                      <Skeleton className='ml-auto h-4 w-28' />
                    </TableCell>
                  </TableRow>
                ))
              : items.map((user) => (
                  <TableRow key={user.id}>
                    <TableCell className='pl-6'>
                      <div className='min-w-36'>
                        <p className='font-medium'>
                          {user.display_name || user.username}
                        </p>
                        <p className='text-muted-foreground text-xs'>
                          @{user.username}
                        </p>
                      </div>
                    </TableCell>
                    <TableCell>
                      {formatTimestamp(user.created_at, '-')}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {user.top_up_count}
                    </TableCell>
                    <TableCell className='text-right font-medium tabular-nums'>
                      {formatQuota(user.recharge_quota_total)}
                    </TableCell>
                    <TableCell className='text-right font-medium tabular-nums'>
                      {formatQuota(user.commission_quota_total)}
                    </TableCell>
                    <TableCell className='text-right font-medium tabular-nums'>
                      {formatQuota(user.range_consume_quota)}
                    </TableCell>
                    <TableCell className='pr-6 text-right'>
                      {formatTimestamp(
                        user.last_commission_at,
                        t('No commission yet')
                      )}
                    </TableCell>
                  </TableRow>
                ))}
          </TableBody>
        </Table>

        {!reportQuery.isLoading && items.length === 0 ? (
          <div className='flex flex-col items-center gap-2 px-6 py-12 text-center'>
            <HugeiconsIcon
              icon={UserMultiple02Icon}
              strokeWidth={1.5}
              className='text-muted-foreground size-10'
            />
            <p className='font-medium'>{t('No invited users yet')}</p>
            <p className='text-muted-foreground max-w-sm text-sm'>
              {t(
                'Share your referral link. New users who register through it will appear here.'
              )}
            </p>
          </div>
        ) : null}
      </CardContent>

      {total > PAGE_SIZE ? (
        <CardFooter className='shrink-0 justify-between border-t pt-4'>
          <p className='text-muted-foreground text-sm'>
            {t('Page {{page}} of {{pageCount}}', {
              page,
              pageCount,
            })}
          </p>
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='outline'
              size='icon-sm'
              disabled={page <= 1 || reportQuery.isFetching}
              onClick={() => setPage((current) => Math.max(1, current - 1))}
              aria-label={t('Previous page')}
            >
              <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
            </Button>
            <Button
              type='button'
              variant='outline'
              size='icon-sm'
              disabled={page >= pageCount || reportQuery.isFetching}
              onClick={() =>
                setPage((current) => Math.min(pageCount, current + 1))
              }
              aria-label={t('Next page')}
            >
              <HugeiconsIcon icon={ArrowRight01Icon} strokeWidth={2} />
            </Button>
          </div>
        </CardFooter>
      ) : null}
    </Card>
  )
}
