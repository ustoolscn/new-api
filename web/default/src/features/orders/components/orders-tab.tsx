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
import { FileText, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
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
import { formatNumber } from '@/lib/format'

import { createInvoiceRequest, getUserOrders } from '../api'
import { formatOrderTimestamp, getInvoiceStatusConfig } from '../lib/format'
import type { UserOrder } from '../types'
import { InvoiceApplicationDialog } from './invoice-application-dialog'
import { ListPagination } from './list-pagination'

const PAGE_SIZE = 20
const MAX_SELECTED_ORDERS = 100

function getOrderSelectionKey(order: UserOrder): string {
  return `${order.order_type}-${order.id}`
}

export function OrdersTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [searchInput, setSearchInput] = useState('')
  const [keyword, setKeyword] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [selectedOrders, setSelectedOrders] = useState<Map<string, UserOrder>>(
    new Map()
  )

  const ordersQuery = useQuery({
    queryKey: ['orders', page, PAGE_SIZE, keyword],
    queryFn: async () => {
      const response = await getUserOrders({
        page,
        pageSize: PAGE_SIZE,
        keyword,
      })
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load orders'))
      }
      return response.data
    },
    placeholderData: keepPreviousData,
  })

  const createMutation = useMutation({
    mutationFn: async (invoiceTitle: string) => {
      const response = await createInvoiceRequest({
        invoice_title: invoiceTitle,
        top_up_ids: [...selectedOrders.values()].map(
          (order) => order.invoice_source_id
        ),
      })
      if (!response.success) {
        throw new Error(
          response.message || t('Failed to submit invoice request')
        )
      }
      return response.data
    },
    onSuccess: async () => {
      toast.success(t('Invoice request submitted'))
      setSelectedOrders(new Map())
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['orders'] }),
        queryClient.invalidateQueries({ queryKey: ['invoice-requests'] }),
      ])
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to submit invoice request'))
    },
  })

  const orders = ordersQuery.data?.items ?? []
  const eligibleOrders = orders.filter((order) => order.invoice_eligible)
  const selectedList = useMemo(
    () => [...selectedOrders.values()],
    [selectedOrders]
  )
  const selectedTotal = selectedList.reduce(
    (total, order) => total + order.money,
    0
  )
  const selectedEligibleCount = eligibleOrders.filter((order) =>
    selectedOrders.has(getOrderSelectionKey(order))
  ).length
  const allEligibleSelected =
    eligibleOrders.length > 0 && selectedEligibleCount === eligibleOrders.length

  const toggleOrder = (order: UserOrder, checked: boolean) => {
    const selectionKey = getOrderSelectionKey(order)
    if (
      checked &&
      !selectedOrders.has(selectionKey) &&
      selectedOrders.size >= MAX_SELECTED_ORDERS
    ) {
      toast.error(t('You can select up to 100 orders per invoice request'))
      return
    }
    setSelectedOrders((current) => {
      const next = new Map(current)
      if (!checked) {
        next.delete(selectionKey)
        return next
      }
      next.set(selectionKey, order)
      return next
    })
  }

  const toggleCurrentPage = (checked: boolean) => {
    const additions = eligibleOrders.filter(
      (order) => !selectedOrders.has(getOrderSelectionKey(order))
    )
    if (
      checked &&
      selectedOrders.size + additions.length > MAX_SELECTED_ORDERS
    ) {
      toast.error(t('You can select up to 100 orders per invoice request'))
      return
    }
    setSelectedOrders((current) => {
      const next = new Map(current)
      if (!checked) {
        eligibleOrders.forEach((order) =>
          next.delete(getOrderSelectionKey(order))
        )
        return next
      }
      additions.forEach((order) => next.set(getOrderSelectionKey(order), order))
      return next
    })
  }

  const submitSearch = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setPage(1)
    setKeyword(searchInput.trim())
  }

  const submitInvoice = async (invoiceTitle: string) => {
    if (selectedOrders.size === 0) return false
    try {
      await createMutation.mutateAsync(invoiceTitle)
      return true
    } catch {
      return false
    }
  }

  return (
    <>
      <Card data-card-hover='false'>
        <CardHeader className='gap-4'>
          <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
            <div>
              <CardTitle>{t('Completed orders')}</CardTitle>
              <p className='text-muted-foreground mt-1 text-sm'>
                {t(
                  'Completed recharge and externally paid subscription orders are shown here.'
                )}
              </p>
            </div>
            <div className='flex flex-col gap-2 sm:flex-row'>
              <form onSubmit={submitSearch} className='flex gap-2'>
                <div className='relative min-w-0 flex-1 sm:w-72'>
                  <Search className='text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2' />
                  <Input
                    value={searchInput}
                    onChange={(event) => setSearchInput(event.target.value)}
                    placeholder={t('Search by order number...')}
                    className='pl-9'
                  />
                </div>
                <Button type='submit' variant='outline'>
                  {t('Search')}
                </Button>
              </form>
            </div>
          </div>
          {selectedOrders.size > 0 ? (
            <div className='bg-primary/5 border-primary/20 flex flex-col gap-3 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between'>
              <div>
                <p className='font-medium'>
                  {t('{{count}} orders selected', {
                    count: selectedOrders.size,
                  })}
                </p>
                <p className='text-muted-foreground text-sm'>
                  {t('Combined invoice amount: {{amount}}', {
                    amount: formatNumber(selectedTotal),
                  })}
                </p>
              </div>
              <Button type='button' onClick={() => setDialogOpen(true)}>
                <FileText data-icon='inline-start' />
                {t('Apply for invoice')}
              </Button>
            </div>
          ) : null}
        </CardHeader>
        <CardContent className='px-0'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className='w-12 pl-4'>
                  <Checkbox
                    checked={allEligibleSelected}
                    indeterminate={
                      selectedEligibleCount > 0 && !allEligibleSelected
                    }
                    disabled={eligibleOrders.length === 0}
                    onCheckedChange={(checked) => toggleCurrentPage(!!checked)}
                    aria-label={t('Select eligible orders on this page')}
                  />
                </TableHead>
                <TableHead>{t('Order number')}</TableHead>
                <TableHead>{t('Completed at')}</TableHead>
                <TableHead>{t('Payment method')}</TableHead>
                <TableHead className='text-right'>
                  {t('Payment amount')}
                </TableHead>
                <TableHead>{t('Order type')}</TableHead>
                <TableHead className='pr-4'>{t('Invoice status')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {ordersQuery.isLoading
                ? Array.from({ length: 6 }, (_, index) => (
                    <TableRow key={index}>
                      <TableCell className='pl-4'>
                        <Skeleton className='size-4' />
                      </TableCell>
                      <TableCell>
                        <Skeleton className='h-4 w-44' />
                      </TableCell>
                      <TableCell>
                        <Skeleton className='h-4 w-32' />
                      </TableCell>
                      <TableCell>
                        <Skeleton className='h-4 w-20' />
                      </TableCell>
                      <TableCell>
                        <Skeleton className='ml-auto h-4 w-20' />
                      </TableCell>
                      <TableCell>
                        <Skeleton className='h-5 w-16' />
                      </TableCell>
                      <TableCell className='pr-4'>
                        <Skeleton className='h-5 w-24' />
                      </TableCell>
                    </TableRow>
                  ))
                : orders.map((order) => {
                    const isSelected =
                      order.invoice_eligible &&
                      selectedOrders.has(getOrderSelectionKey(order))
                    const invoiceStatus = order.invoice_status
                      ? getInvoiceStatusConfig(order.invoice_status, t)
                      : null
                    let invoiceStatusContent = (
                      <span className='text-muted-foreground'>-</span>
                    )
                    if (invoiceStatus) {
                      invoiceStatusContent = (
                        <StatusBadge
                          label={invoiceStatus.label}
                          variant={invoiceStatus.variant}
                          copyable={false}
                          showDot
                        />
                      )
                    } else {
                      invoiceStatusContent = (
                        <span className='text-muted-foreground text-sm'>
                          {t('Not requested')}
                        </span>
                      )
                    }
                    return (
                      <TableRow
                        key={`${order.order_type}-${order.id}`}
                        data-state={isSelected ? 'selected' : undefined}
                      >
                        <TableCell className='pl-4'>
                          <Checkbox
                            checked={isSelected}
                            disabled={!order.invoice_eligible}
                            onCheckedChange={(checked) =>
                              toggleOrder(order, !!checked)
                            }
                            aria-label={t('Select order {{orderNumber}}', {
                              orderNumber: order.trade_no,
                            })}
                          />
                        </TableCell>
                        <TableCell>
                          <code className='font-mono text-xs'>
                            {order.trade_no}
                          </code>
                          <p className='text-muted-foreground mt-1 text-xs'>
                            {order.product_name ||
                              (order.order_type === 'subscription'
                                ? t('Subscription')
                                : t('Account top-up'))}
                          </p>
                        </TableCell>
                        <TableCell>
                          {formatOrderTimestamp(
                            order.complete_time || order.create_time
                          )}
                        </TableCell>
                        <TableCell>
                          {order.payment_method ||
                            order.payment_provider ||
                            '-'}
                        </TableCell>
                        <TableCell className='text-right font-medium'>
                          {formatNumber(order.money)}
                        </TableCell>
                        <TableCell>
                          <StatusBadge
                            label={
                              order.order_type === 'subscription'
                                ? t('Subscription')
                                : t('Recharge')
                            }
                            variant={
                              order.order_type === 'subscription'
                                ? 'info'
                                : 'neutral'
                            }
                            copyable={false}
                            showDot
                          />
                        </TableCell>
                        <TableCell className='pr-4'>
                          {invoiceStatusContent}
                        </TableCell>
                      </TableRow>
                    )
                  })}
            </TableBody>
          </Table>
          {!ordersQuery.isLoading && orders.length === 0 ? (
            <div className='text-muted-foreground py-14 text-center text-sm'>
              {t('No completed orders found')}
            </div>
          ) : null}
          {(ordersQuery.data?.total ?? 0) > 0 ? (
            <ListPagination
              page={page}
              pageSize={PAGE_SIZE}
              total={ordersQuery.data?.total ?? 0}
              disabled={ordersQuery.isFetching}
              onPageChange={setPage}
            />
          ) : null}
        </CardContent>
      </Card>

      <InvoiceApplicationDialog
        open={dialogOpen}
        orders={selectedList}
        submitting={createMutation.isPending}
        onOpenChange={setDialogOpen}
        onSubmit={submitInvoice}
      />
    </>
  )
}
