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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatLogQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getLogStats, getUserLogStats } from '../api'
import { DEFAULT_LOG_STATS } from '../constants'
import { buildApiParams } from '../lib/utils'
import { useLogsViewScope, useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

function StatBadge(props: {
  label: string
  value: string | number
  accent: string
}) {
  return (
    <span className='border-border/60 bg-muted/25 inline-flex h-7 items-center gap-2 rounded-md border px-2.5 text-xs shadow-xs'>
      <span className={cn('h-3.5 w-0.5 rounded-full', props.accent)} />
      <span className='text-muted-foreground'>{props.label}</span>
      <span className='text-foreground/85 font-mono font-semibold tabular-nums'>
        {props.value}
      </span>
    </span>
  )
}

function formatTokenStat(value: number | undefined) {
  return (value ?? 0).toLocaleString()
}

function ModelStatBadge(props: {
  modelName: string
  quotaValue: string
  inputTokens: number
  cacheTokens: number
  completionTokens: number
  usageLabel: string
  inputLabel: string
  cacheLabel: string
  outputLabel: string
}) {
  const modelName = props.modelName || '-'

  return (
    <span className='border-border/60 bg-muted/20 inline-flex min-h-7 max-w-full flex-wrap items-center gap-x-2 gap-y-1 rounded-md border px-2.5 py-1 text-xs shadow-xs'>
      <span className='text-foreground/85 max-w-[16rem] truncate font-medium'>
        {modelName}
      </span>
      <span className='text-muted-foreground inline-flex items-center gap-1'>
        <span>{props.usageLabel}</span>
        <span className='text-foreground/85 font-mono font-semibold tabular-nums'>
          {props.quotaValue}
        </span>
      </span>
      <span className='text-muted-foreground inline-flex items-center gap-1'>
        <span>{props.inputLabel}</span>
        <span className='text-foreground/85 font-mono font-semibold tabular-nums'>
          {formatTokenStat(props.inputTokens)}
        </span>
      </span>
      <span className='text-muted-foreground inline-flex items-center gap-1'>
        <span>{props.cacheLabel}</span>
        <span className='text-foreground/85 font-mono font-semibold tabular-nums'>
          {formatTokenStat(props.cacheTokens)}
        </span>
      </span>
      <span className='text-muted-foreground inline-flex items-center gap-1'>
        <span>{props.outputLabel}</span>
        <span className='text-foreground/85 font-mono font-semibold tabular-nums'>
          {formatTokenStat(props.completionTokens)}
        </span>
      </span>
    </span>
  )
}

export function CommonLogsStats() {
  const { t } = useTranslation()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const searchParams = route.useSearch()
  const { sensitiveVisible } = useUsageLogsContext()

  const { data: stats, isLoading } = useQuery({
    queryKey: ['usage-logs-stats', isAdmin, searchParams],
    queryFn: async () => {
      const params = buildApiParams({
        page: 1,
        pageSize: 1,
        searchParams,
        columnFilters: [],
        isAdmin,
      })

      const result = isAdmin
        ? await getLogStats(params)
        : await getUserLogStats(params)

      return result.success
        ? result.data || DEFAULT_LOG_STATS
        : DEFAULT_LOG_STATS
    },
    placeholderData: (previousData) => previousData,
  })

  if (isLoading) {
    return (
      <div className='flex items-center gap-2'>
        <Skeleton className='h-7 w-[150px] rounded-md' />
        <Skeleton className='h-7 w-[100px] rounded-md' />
        <Skeleton className='h-7 w-[120px] rounded-md' />
        <Skeleton className='h-7 w-[130px] rounded-md' />
        <Skeleton className='h-7 w-[110px] rounded-md' />
        <Skeleton className='h-7 w-[130px] rounded-md' />
      </div>
    )
  }

  const modelStats = stats?.model_stats || []
  const usageLabel = t('Usage')
  const inputLabel = t('Input Tokens')
  const cacheLabel = t('Cache')
  const outputLabel = t('Output Tokens')

  return (
    <div className='flex min-w-0 flex-col gap-2'>
      <div className='flex flex-wrap items-center gap-2'>
        <StatBadge
          label={t('Usage')}
          value={sensitiveVisible ? formatLogQuota(stats?.quota || 0) : '••••'}
          accent='bg-sky-500/70'
        />
        <StatBadge
          label={t('RPM')}
          value={stats?.rpm || 0}
          accent='bg-rose-500/65'
        />
        <StatBadge
          label={t('TPM')}
          value={stats?.tpm || 0}
          accent='bg-slate-400/70'
        />
        <StatBadge
          label={inputLabel}
          value={formatTokenStat(stats?.input_tokens)}
          accent='bg-emerald-500/70'
        />
        <StatBadge
          label={cacheLabel}
          value={formatTokenStat(stats?.cache_tokens)}
          accent='bg-amber-500/70'
        />
        <StatBadge
          label={outputLabel}
          value={formatTokenStat(stats?.completion_tokens)}
          accent='bg-violet-500/70'
        />
      </div>
      {modelStats.length > 0 && (
        <div className='flex max-h-24 flex-wrap items-center gap-2 overflow-y-auto pr-1'>
          {modelStats.map((item) => (
            <ModelStatBadge
              key={item.model_name || '__empty_model__'}
              modelName={item.model_name}
              quotaValue={
                sensitiveVisible ? formatLogQuota(item.quota || 0) : '••••'
              }
              inputTokens={item.input_tokens}
              cacheTokens={item.cache_tokens}
              completionTokens={item.completion_tokens}
              usageLabel={usageLabel}
              inputLabel={inputLabel}
              cacheLabel={cacheLabel}
              outputLabel={outputLabel}
            />
          ))}
        </div>
      )}
    </div>
  )
}
