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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'
import { formatQuota } from '@/lib/format'

import { getReferralModelConsumeReport } from '../api'

const PAGE_SIZE = 6

type ModelConsumeDialogProps = {
  inviterId?: number
  inviteeId?: number
  inviteeName?: string
  startTimestamp: number
  endTimestamp: number
  onClose: () => void
}

export function ModelConsumeDialog(props: ModelConsumeDialogProps) {
  const { t, i18n } = useTranslation()
  const [page, setPage] = useState(1)
  const reportQuery = useQuery({
    queryKey: [
      'referral-model-consumption',
      props.inviterId ?? 'self',
      props.inviteeId ?? 'all',
      props.startTimestamp,
      props.endTimestamp,
      page,
    ],
    queryFn: async () => {
      const response = await getReferralModelConsumeReport({
        inviterId: props.inviterId,
        inviteeId: props.inviteeId,
        startTimestamp: props.startTimestamp,
        endTimestamp: props.endTimestamp,
        page,
        pageSize: PAGE_SIZE,
      })
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to load invitee consumption')
        )
      }
      return response.data
    },
    placeholderData: keepPreviousData,
  })

  const report = reportQuery.data
  const pageCount = Math.max(
    1,
    Math.ceil((report?.models.total ?? 0) / PAGE_SIZE)
  )
  const locale = toIntlLocale(i18n.resolvedLanguage)
  const numberFormat = new Intl.NumberFormat(locale)
  const shareFormat = new Intl.NumberFormat(locale, {
    style: 'percent',
    maximumFractionDigits: 2,
  })
  const summaries = [
    {
      label: t('Period consumption'),
      value: formatQuota(report?.consume_quota ?? 0),
    },
    {
      label: t('Requests'),
      value: numberFormat.format(report?.request_count ?? 0),
    },
    { label: t('Tokens'), value: numberFormat.format(report?.token_used ?? 0) },
  ]

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
    >
      <DialogContent className='flex max-h-[85dvh] flex-col sm:max-w-3xl'>
        <DialogHeader className='shrink-0 pr-7'>
          <DialogTitle>{t('Consumption by model')}</DialogTitle>
          <DialogDescription className='break-words'>
            {props.inviteeName ?? t('All invited users')}
            {props.inviteeId != null ? ` (ID: ${props.inviteeId})` : null}
            <span className='mt-1 block text-xs tabular-nums'>
              {t('Selected period')}:{' '}
              {dayjs.unix(props.startTimestamp).format('YYYY-MM-DD')}
              {' ~ '}
              {dayjs
                .unix(props.endTimestamp)
                .subtract(1, 'second')
                .format('YYYY-MM-DD')}
            </span>
          </DialogDescription>
        </DialogHeader>

        {reportQuery.isError ? (
          <Alert variant='destructive'>
            <AlertTitle>{t('Failed to load invitee consumption')}</AlertTitle>
            <AlertDescription>
              <p>{reportQuery.error.message}</p>
              <Button
                variant='outline'
                size='sm'
                onClick={() => void reportQuery.refetch()}
              >
                {t('Retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : (
          <>
            <div className='grid shrink-0 grid-cols-3 gap-2'>
              {summaries.map((summary) => (
                <div
                  key={summary.label}
                  className='min-w-0 rounded-lg border p-3'
                >
                  <p className='text-muted-foreground text-xs'>
                    {summary.label}
                  </p>
                  {reportQuery.isPending ? (
                    <Skeleton className='mt-2 h-6 w-16' />
                  ) : (
                    <p className='mt-1 text-lg font-semibold break-all tabular-nums'>
                      {summary.value}
                    </p>
                  )}
                </div>
              ))}
            </div>
            <div
              className='min-h-0 overflow-auto rounded-lg border'
              aria-busy={reportQuery.isFetching}
            >
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Model')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Requests')}
                    </TableHead>
                    <TableHead className='text-right'>{t('Tokens')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Period consumption')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Consumption share')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody className='[&>tr]:h-12'>
                  {reportQuery.isPending
                    ? Array.from({ length: 3 }, (_, index) => (
                        <TableRow key={index}>
                          <TableCell colSpan={5}>
                            <Skeleton className='h-6 w-full' />
                          </TableCell>
                        </TableRow>
                      ))
                    : report?.models.items.map((model) => (
                        <TableRow key={model.model_name}>
                          <TableCell
                            className='max-w-60 truncate font-medium'
                            title={model.model_name}
                          >
                            {model.model_name || t('Unknown model')}
                          </TableCell>
                          <TableCell className='text-right'>
                            {numberFormat.format(model.request_count)}
                          </TableCell>
                          <TableCell className='text-right'>
                            {numberFormat.format(model.token_used)}
                          </TableCell>
                          <TableCell className='text-right font-medium'>
                            {formatQuota(model.consume_quota)}
                          </TableCell>
                          <TableCell className='text-right'>
                            {shareFormat.format(
                              report.consume_quota > 0
                                ? model.consume_quota / report.consume_quota
                                : 0
                            )}
                          </TableCell>
                        </TableRow>
                      ))}
                </TableBody>
              </Table>
              {report?.models.items.length === 0 ? (
                <Empty className='py-10'>
                  <EmptyHeader>
                    <EmptyTitle>
                      {t('No consumption in this period')}
                    </EmptyTitle>
                  </EmptyHeader>
                </Empty>
              ) : null}
            </div>
          </>
        )}

        <DialogFooter className='shrink-0 flex-row flex-wrap items-center justify-between gap-3 sm:justify-between'>
          <p className='text-muted-foreground text-xs' aria-live='polite'>
            {t('Page {{page}} of {{pageCount}}', { page, pageCount })}
          </p>
          <div className='flex gap-2'>
            <Button
              variant='outline'
              size='sm'
              disabled={page <= 1 || reportQuery.isFetching}
              onClick={() => setPage(page - 1)}
            >
              {t('Previous page')}
            </Button>
            <Button
              variant='outline'
              size='sm'
              disabled={
                page >= pageCount ||
                reportQuery.isFetching ||
                reportQuery.isError
              }
              onClick={() => setPage(page + 1)}
            >
              {t('Next page')}
            </Button>
            <DialogClose render={<Button size='sm' />}>
              {t('Close')}
            </DialogClose>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
