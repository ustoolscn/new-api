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
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { Download, FileCheck2, Link2, SearchCheck } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
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

import {
  getAdminInvoiceRequests,
  issueInvoiceRequest,
  reviewInvoiceRequest,
} from './api'
import { AdminInvoiceURLDialog } from './components/admin-invoice-url-dialog'
import { AdminReviewDialog } from './components/admin-review-dialog'
import { ListPagination } from './components/list-pagination'
import { formatOrderTimestamp, getInvoiceStatusConfig } from './lib/format'
import type {
  InvoiceRequest,
  InvoiceStatus,
  IssueInvoiceRequestInput,
  ReviewInvoiceRequestInput,
} from './types'

const PAGE_SIZE = 20

export function InvoiceManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<InvoiceStatus | ''>('pending')
  const [reviewRequest, setReviewRequest] = useState<InvoiceRequest | null>(
    null
  )
  const [issueRequest, setIssueRequest] = useState<InvoiceRequest | null>(null)

  const invoicesQuery = useQuery({
    queryKey: ['invoice-requests', 'admin', page, PAGE_SIZE, status],
    queryFn: async () => {
      const response = await getAdminInvoiceRequests({
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

  const reviewMutation = useMutation({
    mutationFn: async (variables: {
      requestId: number
      input: ReviewInvoiceRequestInput
    }) => {
      const response = await reviewInvoiceRequest(
        variables.requestId,
        variables.input
      )
      if (!response.success) {
        throw new Error(
          response.message || t('Failed to review invoice request')
        )
      }
    },
    onSuccess: async () => {
      toast.success(t('Invoice request reviewed'))
      await queryClient.invalidateQueries({ queryKey: ['invoice-requests'] })
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to review invoice request'))
    },
  })

  const issueMutation = useMutation({
    mutationFn: async (variables: {
      requestId: number
      input: IssueInvoiceRequestInput
    }) => {
      const response = await issueInvoiceRequest(
        variables.requestId,
        variables.input
      )
      if (!response.success) {
        throw new Error(response.message || t('Failed to save invoice link'))
      }
    },
    onSuccess: async () => {
      toast.success(t('Invoice link saved'))
      await queryClient.invalidateQueries({ queryKey: ['invoice-requests'] })
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to save invoice link'))
    },
  })

  const submitReview = async (action: 'approve' | 'reject', remark: string) => {
    if (!reviewRequest) return false
    try {
      await reviewMutation.mutateAsync({
        requestId: reviewRequest.id,
        input: { action, remark },
      })
      return true
    } catch {
      return false
    }
  }

  const submitInvoiceURL = async (invoiceURL: string) => {
    if (!issueRequest) return false
    try {
      await issueMutation.mutateAsync({
        requestId: issueRequest.id,
        input: { invoice_url: invoiceURL },
      })
      return true
    } catch {
      return false
    }
  }

  const invoices = invoicesQuery.data?.items ?? []

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Invoice Management')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t(
            'Review invoice applications, provide PDF download links, and manage issued invoices.'
          )}
        </SectionPageLayout.Description>
        <SectionPageLayout.Content>
          <div className='mx-auto w-full max-w-7xl'>
            <Card data-card-hover='false'>
              <CardHeader className='gap-4 sm:flex-row sm:items-center sm:justify-between'>
                <CardTitle>{t('Invoice requests')}</CardTitle>
                <Select
                  items={[
                    { value: 'all', label: t('All statuses') },
                    { value: 'pending', label: t('Pending review') },
                    {
                      value: 'approved',
                      label: t('Approved, awaiting invoice'),
                    },
                    { value: 'rejected', label: t('Rejected') },
                    { value: 'issued', label: t('Invoice issued') },
                  ]}
                  value={status || 'all'}
                  onValueChange={(value) => {
                    setPage(1)
                    setStatus(
                      value === 'all' || value === null
                        ? ''
                        : (value as InvoiceStatus)
                    )
                  }}
                >
                  <SelectTrigger className='w-full sm:w-56'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem value='all'>{t('All statuses')}</SelectItem>
                      <SelectItem value='pending'>
                        {t('Pending review')}
                      </SelectItem>
                      <SelectItem value='approved'>
                        {t('Approved, awaiting invoice')}
                      </SelectItem>
                      <SelectItem value='rejected'>{t('Rejected')}</SelectItem>
                      <SelectItem value='issued'>
                        {t('Invoice issued')}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </CardHeader>
              <CardContent className='px-0'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className='pl-4'>{t('Request')}</TableHead>
                      <TableHead>{t('User')}</TableHead>
                      <TableHead>{t('Invoice title')}</TableHead>
                      <TableHead>{t('Included orders')}</TableHead>
                      <TableHead className='text-right'>
                        {t('Invoice amount')}
                      </TableHead>
                      <TableHead>{t('Submitted at')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead className='pr-4 text-right'>
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {invoicesQuery.isLoading
                      ? Array.from({ length: 6 }, (_, index) => (
                          <TableRow key={index}>
                            <TableCell className='pl-4'>
                              <Skeleton className='h-4 w-16' />
                            </TableCell>
                            <TableCell>
                              <Skeleton className='h-4 w-28' />
                            </TableCell>
                            <TableCell>
                              <Skeleton className='h-4 w-40' />
                            </TableCell>
                            <TableCell>
                              <Skeleton className='h-4 w-40' />
                            </TableCell>
                            <TableCell>
                              <Skeleton className='ml-auto h-4 w-20' />
                            </TableCell>
                            <TableCell>
                              <Skeleton className='h-4 w-32' />
                            </TableCell>
                            <TableCell>
                              <Skeleton className='h-5 w-24' />
                            </TableCell>
                            <TableCell className='pr-4'>
                              <Skeleton className='ml-auto h-8 w-28' />
                            </TableCell>
                          </TableRow>
                        ))
                      : invoices.map((invoice) => {
                          const statusConfig = getInvoiceStatusConfig(
                            invoice.status,
                            t
                          )
                          return (
                            <TableRow key={invoice.id}>
                              <TableCell className='pl-4 font-medium'>
                                #{invoice.id}
                              </TableCell>
                              <TableCell>
                                <div className='max-w-40 truncate font-medium'>
                                  {invoice.display_name || invoice.username}
                                </div>
                                <div className='text-muted-foreground text-xs'>
                                  ID: {invoice.user_id}
                                </div>
                              </TableCell>
                              <TableCell>
                                <div className='max-w-52 truncate font-medium'>
                                  {invoice.invoice_title}
                                </div>
                                {invoice.review_remark ? (
                                  <div
                                    className='text-muted-foreground mt-1 max-w-52 truncate text-xs'
                                    title={invoice.review_remark}
                                  >
                                    {invoice.review_remark}
                                  </div>
                                ) : null}
                              </TableCell>
                              <TableCell>
                                <div className='max-w-64 space-y-1'>
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
                              <TableCell className='pr-4'>
                                <div className='flex justify-end gap-2'>
                                  {invoice.status === 'pending' ? (
                                    <Button
                                      type='button'
                                      size='sm'
                                      variant='outline'
                                      onClick={() => setReviewRequest(invoice)}
                                    >
                                      <SearchCheck data-icon='inline-start' />
                                      {t('Review')}
                                    </Button>
                                  ) : null}
                                  {invoice.status === 'approved' ||
                                  invoice.status === 'issued' ? (
                                    <Button
                                      type='button'
                                      size='sm'
                                      variant='outline'
                                      onClick={() => setIssueRequest(invoice)}
                                    >
                                      <Link2 data-icon='inline-start' />
                                      {invoice.status === 'issued'
                                        ? t('Replace link')
                                        : t('Provide PDF link')}
                                    </Button>
                                  ) : null}
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
                                      size='icon-sm'
                                      variant='outline'
                                      aria-label={t('Download PDF')}
                                      title={invoice.invoice_url}
                                    >
                                      <Download />
                                    </Button>
                                  ) : null}
                                </div>
                              </TableCell>
                            </TableRow>
                          )
                        })}
                  </TableBody>
                </Table>
                {!invoicesQuery.isLoading && invoices.length === 0 ? (
                  <div className='flex flex-col items-center gap-2 py-14 text-center'>
                    <FileCheck2 className='text-muted-foreground size-10' />
                    <p className='font-medium'>
                      {t('No invoice requests found')}
                    </p>
                    <p className='text-muted-foreground text-sm'>
                      {t('New user invoice applications will appear here.')}
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
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <AdminReviewDialog
        request={reviewRequest}
        submitting={reviewMutation.isPending}
        onOpenChange={(open) => !open && setReviewRequest(null)}
        onSubmit={submitReview}
      />
      <AdminInvoiceURLDialog
        request={issueRequest}
        submitting={issueMutation.isPending}
        onOpenChange={(open) => !open && setIssueRequest(null)}
        onSubmit={submitInvoiceURL}
      />
    </>
  )
}
