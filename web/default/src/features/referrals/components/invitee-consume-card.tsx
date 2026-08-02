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
import { ChartHistogramIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ReferralOverview } from '../types'

type InviteeConsumeCardProps = {
  overview?: ReferralOverview
  loading: boolean
  className?: string
}

export function InviteeConsumeCard(props: InviteeConsumeCardProps) {
  const { t } = useTranslation()
  const months = props.overview?.invitee_consume_months ?? []
  // Newest months first for scanning recent usage.
  const orderedMonths = [...months].reverse()

  return (
    <Card className={props.className} data-card-hover='false'>
      <CardHeader className='shrink-0'>
        <div className='flex items-start gap-3'>
          <IconBadge tone='primary' size='lg'>
            <HugeiconsIcon icon={ChartHistogramIcon} strokeWidth={1.8} />
          </IconBadge>
          <div className='min-w-0 flex-1'>
            <CardTitle>{t('Invitee consumption')}</CardTitle>
            <CardDescription className='mt-1'>
              {t(
                'Lifetime usage by invited users, broken down by calendar month. Monthly values come from durable usage aggregates, not request logs.'
              )}
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='rounded-xl border px-4 py-3'>
          <p className='text-muted-foreground text-sm'>
            {t('Total invitee consumption')}
          </p>
          {props.loading ? (
            <Skeleton className='mt-2 h-8 w-32' />
          ) : (
            <p className='mt-1 text-2xl font-semibold tabular-nums'>
              {formatQuota(props.overview?.invitee_consume_total ?? 0)}
            </p>
          )}
        </div>

        <div className={cn('overflow-hidden rounded-xl border')}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className='pl-4'>{t('Calendar month')}</TableHead>
                <TableHead className='pr-4 text-right'>
                  {t('Consumed quota')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.loading
                ? Array.from({ length: 4 }, (_, index) => (
                    <TableRow key={index}>
                      <TableCell className='pl-4'>
                        <Skeleton className='h-4 w-20' />
                      </TableCell>
                      <TableCell className='pr-4'>
                        <Skeleton className='ml-auto h-4 w-24' />
                      </TableCell>
                    </TableRow>
                  ))
                : orderedMonths.map((month) => (
                    <TableRow key={month.month_start || month.month_label}>
                      <TableCell className='pl-4 font-medium tabular-nums'>
                        {month.month_label}
                      </TableCell>
                      <TableCell className='pr-4 text-right font-medium tabular-nums'>
                        {formatQuota(month.consume_quota)}
                      </TableCell>
                    </TableRow>
                  ))}
            </TableBody>
          </Table>
          {!props.loading && orderedMonths.length === 0 ? (
            <div className='text-muted-foreground px-4 py-8 text-center text-sm'>
              {t('No invitee consumption yet')}
            </div>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}
