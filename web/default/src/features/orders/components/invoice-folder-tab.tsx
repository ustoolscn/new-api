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
import { Download, FileCheck2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatNumber } from '@/lib/format'

import { getUserInvoiceRequests } from '../api'
import { formatOrderTimestamp, getInvoiceStatusConfig } from '../lib/format'
import type { InvoiceStatus } from '../types'
import { ListPagination } from './list-pagination'

const PAGE_SIZE = 20

export function InvoiceFolderTab() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<InvoiceStatus | ''>('')

  const invoicesQuery = useQuery({
    queryKey: ['invoice-requests', 'self', page, PAGE_SIZE, status],
    queryFn: async () => {
      const response = await getUserInvoiceRequests({
        page,
        pageSize: PAGE_SIZE,
        status,
      })
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to load invoice requests')
        )
      }
      return response.data
    },
    placeholderData: keepPreviousData,
  })

  const invoices = invoicesQuery.data?.items ?? []

  return (
    <Card data-card-hover='false'>
      <CardHeader className='gap-4 sm:flex-row sm:items-center sm:justify-between'>
        <div>
          <CardTitle>{t('Invoice folder')}</CardTitle>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('Track review progress and open issued PDF invoices.')}
          </p>
        </div>
        <Select
          items={[
            { value: 'all', label: t('All statuses') },
            { value: 'pending', label: t('Pending review') },
            { value: 'approved', label: t('Approved, awaiting invoice') },
            { value: 'rejected', label: t('Rejected') },
            { value: 'issued', label: t('Invoice issued') },
          ]}
          value={status || 'all'}
          onValueChange={(value) => {
            setPage(1)
            setStatus(
              value === 'all' || value === null ? '' : (value as InvoiceStatus)
            )
          }}
        >
          <SelectTrigger className='w-full sm:w-56'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='all'>{t('All statuses')}</SelectItem>
              <SelectItem value='pending'>{t('Pending review')}</SelectItem>
              <SelectItem value='approved'>
                {t('Approved, awaiting invoice')}
              </SelectItem>
              <SelectItem value='rejected'>{t('Rejected')}</SelectItem>
              <SelectItem value='issued'>{t('Invoice issued')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </CardHeader>
      <CardContent className='px-0'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='pl-4'>{t('Request')}</TableHead>
              <TableHead>{t('Invoice title')}</TableHead>
              <TableHead>{t('Included orders')}</TableHead>
              <TableHead className='text-right'>
                {t('Invoice amount')}
              </TableHead>
              <TableHead>{t('Submitted at')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead className='pr-4 text-right'>
                {t('Invoice PDF')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {invoicesQuery.isLoading
              ? Array.from({ length: 5 }, (_, index) => (
                  <TableRow key={index}>
                    <TableCell className='pl-4'>
                      <Skeleton className='h-4 w-16' />
                    </TableCell>
                    <TableCell>
                      <Skeleton className='h-4 w-40' />
                    </TableCell>
                    <TableCell>
                      <Skeleton className='h-4 w-48' />
                    </TableCell>
                    <TableCell>
                      <Skeleton className='ml-auto h-4 w-20' />
                    </TableCell>
                    <TableCell>
                      <Skeleton className='h-4 w-32' />
                    </TableCell>
                    <TableCell>
                      <Skeleton className='h-5 w-28' />
                    </TableCell>
                    <TableCell className='pr-4'>
                      <Skeleton className='ml-auto h-8 w-24' />
                    </TableCell>
                  </TableRow>
                ))
              : invoices.map((invoice) => {
                  const statusConfig = getInvoiceStatusConfig(invoice.status, t)
                  return (
                    <TableRow key={invoice.id}>
                      <TableCell className='pl-4 font-medium'>
                        #{invoice.id}
                      </TableCell>
                      <TableCell>
                        <div className='max-w-56 truncate font-medium'>
                          {invoice.invoice_title}
                        </div>
                        {invoice.review_remark ? (
                          <div
                            className='text-muted-foreground mt-1 max-w-56 truncate text-xs'
                            title={invoice.review_remark}
                          >
                            {invoice.review_remark}
                          </div>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        <div className='max-w-72 space-y-1'>
                          {invoice.orders.slice(0, 2).map((order) => (
                            <code
                              key={order.top_up_id}
                              className='block truncate font-mono text-xs'
                            >
                              {order.trade_no}
                            </code>
                          ))}
                          {invoice.order_count > 2 ? (
                            <span className='text-muted-foreground text-xs'>
                              {t('{{count}} more orders', {
                                count: invoice.order_count - 2,
                              })}
                            </span>
                          ) : null}
                        </div>
                      </TableCell>
                      <TableCell className='text-right font-semibold'>
                        {formatNumber(invoice.amount)}
                      </TableCell>
                      <TableCell>
                        {formatOrderTimestamp(invoice.created_at)}
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          label={statusConfig.label}
                          variant={statusConfig.variant}
                          copyable={false}
                          showDot
                        />
                      </TableCell>
                      <TableCell className='pr-4 text-right'>
                        {invoice.download_available ? (
                          <Button
                            render={
                              <a
                                href={invoice.invoice_url}
                                target='_blank'
                                rel='noopener noreferrer'
                              />
                            }
                            nativeButton={false}
                            variant='outline'
                            size='sm'
                          >
                            <Download data-icon='inline-start' />
                            {t('Download PDF')}
                          </Button>
                        ) : (
                          <span className='text-muted-foreground text-sm'>
                            -
                          </span>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
          </TableBody>
        </Table>
        {!invoicesQuery.isLoading && invoices.length === 0 ? (
          <div className='flex flex-col items-center gap-2 py-14 text-center'>
            <FileCheck2 className='text-muted-foreground size-10' />
            <p className='font-medium'>{t('No invoice requests yet')}</p>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Select successful orders to submit your first invoice request.'
              )}
            </p>
          </div>
        ) : null}
        {(invoicesQuery.data?.total ?? 0) > 0 ? (
          <ListPagination
            page={page}
            pageSize={PAGE_SIZE}
            total={invoicesQuery.data?.total ?? 0}
            disabled={invoicesQuery.isFetching}
            onPageChange={setPage}
          />
        ) : null}
      </CardContent>
    </Card>
  )
}
