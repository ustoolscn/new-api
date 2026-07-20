/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useNavigate, useSearch } from '@tanstack/react-router'
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  RefreshCw,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import dayjs from '@/lib/dayjs'
import { formatCompactNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import { StatusSection } from './components/status-section'
import { StatusTimeline } from './components/status-timeline'
import { useServiceStatus } from './hooks/use-service-status'
import type { ServiceStatusGranularity, ServiceStatusSnapshot } from './types'

type HealthState = 'operational' | 'degraded' | 'outage' | 'unknown'

function getHealthState(snapshot: ServiceStatusSnapshot): HealthState {
  if (snapshot.overall.success_rate == null) return 'unknown'
  if (snapshot.overall.success_rate >= 99) return 'operational'
  if (snapshot.overall.success_rate >= 95) return 'degraded'
  return 'outage'
}

export function ServiceStatus() {
  const { t } = useTranslation()
  const search = useSearch({ from: '/service-status/' })
  const navigate = useNavigate()
  const granularity: ServiceStatusGranularity =
    search.granularity === 'hour' ? 'hour' : 'day'
  const statusQuery = useServiceStatus(granularity, search.end)
  const snapshot = statusQuery.data?.data

  const updateSearch = (next: {
    granularity?: ServiceStatusGranularity
    end?: number
  }) => {
    navigate({
      to: '/service-status',
      search: (previous) => ({ ...previous, ...next }),
    })
  }

  const selectGranularity = (next: ServiceStatusGranularity) => {
    updateSearch({ granularity: next, end: undefined })
  }

  const movePeriod = (direction: 'previous' | 'next') => {
    if (!snapshot) return
    if (direction === 'previous') {
      updateSearch({ end: snapshot.start_timestamp })
      return
    }
    const duration = snapshot.end_timestamp - snapshot.start_timestamp
    updateSearch({ end: snapshot.end_timestamp + duration })
  }

  let statusContent: React.ReactNode = <ServiceStatusLoading />
  if (!statusQuery.isLoading) {
    statusContent = snapshot ? (
      <ServiceStatusContent snapshot={snapshot} />
    ) : (
      <ServiceStatusError onRetry={() => statusQuery.refetch()} />
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <PageTransition className='mx-auto w-full max-w-[1120px] space-y-8 px-4 pt-24 pb-14 sm:px-6 lg:px-8'>
        <header className='space-y-3 text-center'>
          <p className='text-primary text-sm font-medium'>
            {t('Service Status')}
          </p>
          <h1 className='text-3xl font-semibold tracking-tight sm:text-4xl'>
            {t('System status')}
          </h1>
          <p className='text-muted-foreground mx-auto max-w-2xl text-sm sm:text-base'>
            {t('Service health based on request success rates.')}
          </p>
        </header>

        <div className='flex flex-col justify-between gap-3 sm:flex-row sm:items-center'>
          <div className='bg-muted inline-flex w-fit rounded-lg p-1'>
            {(['hour', 'day'] as const).map((value) => (
              <Button
                key={value}
                type='button'
                variant={granularity === value ? 'outline' : 'ghost'}
                size='sm'
                className={cn(
                  granularity === value && 'bg-background shadow-sm'
                )}
                onClick={() => selectGranularity(value)}
              >
                {value === 'hour' ? t('Hourly') : t('Daily')}
              </Button>
            ))}
          </div>

          <div className='flex items-center gap-2'>
            <Button
              type='button'
              variant='outline'
              size='icon-sm'
              aria-label={t('Previous period')}
              onClick={() => movePeriod('previous')}
              disabled={!snapshot}
            >
              <ChevronLeft />
            </Button>
            <div className='text-muted-foreground min-w-48 text-center text-xs tabular-nums sm:text-sm'>
              {snapshot ? formatPeriodRange(snapshot) : '—'}
            </div>
            <Button
              type='button'
              variant='outline'
              size='icon-sm'
              aria-label={t('Next period')}
              onClick={() => movePeriod('next')}
              disabled={!snapshot || snapshot.is_current_period}
            >
              <ChevronRight />
            </Button>
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t('Refresh')}
              onClick={() => statusQuery.refetch()}
              disabled={statusQuery.isFetching}
            >
              <RefreshCw
                className={cn(statusQuery.isFetching && 'animate-spin')}
              />
            </Button>
          </div>
        </div>

        {statusContent}
      </PageTransition>
    </PublicLayout>
  )
}

function ServiceStatusContent(props: { snapshot: ServiceStatusSnapshot }) {
  const { t } = useTranslation()
  const health = getHealthState(props.snapshot)
  const statusConfig = {
    operational: {
      icon: CheckCircle2,
      title: t('All systems operational'),
      description: t('All requests are being processed normally.'),
      className: 'text-emerald-600 dark:text-emerald-400',
      panelClassName:
        'border-emerald-200/70 bg-emerald-50/70 dark:border-emerald-900 dark:bg-emerald-950/30',
    },
    degraded: {
      icon: Activity,
      title: t('Degraded performance recently'),
      description: t('Some requests failed during this period.'),
      className: 'text-amber-600 dark:text-amber-400',
      panelClassName:
        'border-amber-200/70 bg-amber-50/70 dark:border-amber-900 dark:bg-amber-950/30',
    },
    outage: {
      icon: AlertTriangle,
      title: t('Significant outages detected'),
      description: t('Request failures are affecting service availability.'),
      className: 'text-rose-600 dark:text-rose-400',
      panelClassName:
        'border-rose-200/70 bg-rose-50/70 dark:border-rose-900 dark:bg-rose-950/30',
    },
    unknown: {
      icon: Activity,
      title: t('No request data in this period'),
      description: t('No request data is available for this period.'),
      className: 'text-muted-foreground',
      panelClassName: 'bg-muted/40',
    },
  } satisfies Record<
    HealthState,
    {
      icon: typeof Activity
      title: string
      description: string
      className: string
      panelClassName: string
    }
  >
  const currentStatus = statusConfig[health]
  const StatusIcon = currentStatus.icon

  return (
    <div className='space-y-10'>
      <section
        className={cn(
          'rounded-2xl border px-5 py-6 sm:px-7',
          currentStatus.panelClassName
        )}
      >
        <div className='flex flex-col gap-6'>
          <div className='flex items-start gap-3'>
            <StatusIcon
              className={cn('mt-0.5 size-6 shrink-0', currentStatus.className)}
            />
            <div>
              <h2 className='text-xl font-semibold'>{currentStatus.title}</h2>
              <p className='text-muted-foreground mt-1 text-sm'>
                {currentStatus.description}
              </p>
            </div>
          </div>
          <StatusTimeline
            periods={props.snapshot.periods}
            series={props.snapshot.overall.series}
          />
        </div>
      </section>

      <section className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
        <StatusSummaryCard
          label={t('Requests')}
          value={formatCompactNumber(props.snapshot.overall.request_count)}
        />
        <StatusSummaryCard
          label={t('Success rate')}
          value={
            props.snapshot.overall.success_rate == null
              ? t('No data')
              : `${props.snapshot.overall.success_rate.toFixed(2)}%`
          }
        />
        <StatusSummaryCard
          label={t('Active models')}
          value={props.snapshot.models_total.toLocaleString()}
        />
        <StatusSummaryCard
          label={t('Active groups')}
          value={props.snapshot.groups_total.toLocaleString()}
        />
      </section>

      <StatusSection
        title={t('Model status')}
        description={t('Request volume and success rate for each model.')}
        searchPlaceholder={t('Search models...')}
        metrics={props.snapshot.models}
        periods={props.snapshot.periods}
        total={props.snapshot.models_total}
        truncated={props.snapshot.models_truncated}
      />

      <StatusSection
        title={t('Group status')}
        description={t('Request volume and success rate for each group.')}
        searchPlaceholder={t('Search groups...')}
        metrics={props.snapshot.groups}
        periods={props.snapshot.periods}
        total={props.snapshot.groups_total}
        truncated={props.snapshot.groups_truncated}
      />

      <p className='text-muted-foreground text-center text-xs'>
        {t('Updated every minute')} ·{' '}
        {dayjs.unix(props.snapshot.generated_at).format('YYYY-MM-DD HH:mm:ss')}
      </p>
    </div>
  )
}

function StatusSummaryCard(props: { label: string; value: string }) {
  return (
    <Card size='sm'>
      <CardContent className='space-y-1 py-1'>
        <div className='font-mono text-2xl font-semibold tabular-nums'>
          {props.value}
        </div>
        <div className='text-muted-foreground text-xs'>{props.label}</div>
      </CardContent>
    </Card>
  )
}

function ServiceStatusLoading() {
  return (
    <div className='space-y-8'>
      <Skeleton className='h-48 rounded-2xl' />
      <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
        {Array.from({ length: 4 }, (_, index) => (
          <Skeleton key={index} className='h-24 rounded-xl' />
        ))}
      </div>
      <Skeleton className='h-[420px] rounded-xl' />
    </div>
  )
}

function ServiceStatusError(props: { onRetry: () => void }) {
  const { t } = useTranslation()
  return (
    <div className='bg-card rounded-xl border border-dashed px-6 py-14 text-center'>
      <AlertTriangle className='text-muted-foreground mx-auto size-7' />
      <h2 className='mt-4 text-base font-semibold'>
        {t('Unable to load service status')}
      </h2>
      <Button className='mt-5' variant='outline' onClick={props.onRetry}>
        {t('Retry')}
      </Button>
    </div>
  )
}

function formatPeriodRange(snapshot: ServiceStatusSnapshot): string {
  const format = snapshot.granularity === 'hour' ? 'MM-DD HH:mm' : 'YYYY-MM-DD'
  return `${dayjs.unix(snapshot.start_timestamp).format(format)} – ${dayjs
    .unix(snapshot.end_timestamp)
    .format(format)}`
}
