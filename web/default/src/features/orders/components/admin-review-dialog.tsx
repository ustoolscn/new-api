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
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { formatNumber } from '@/lib/format'

import {
  invoiceReviewSchema,
  type InvoiceReviewFormValues,
} from '../lib/invoice-schema'
import type { InvoiceRequest } from '../types'

type AdminReviewDialogProps = {
  request: InvoiceRequest | null
  submitting: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (action: 'approve' | 'reject', remark: string) => Promise<boolean>
}

export function AdminReviewDialog(props: AdminReviewDialogProps) {
  const { t } = useTranslation()
  const form = useForm<InvoiceReviewFormValues>({
    resolver: zodResolver(invoiceReviewSchema),
    defaultValues: { remark: '' },
  })

  useEffect(() => {
    form.reset({ remark: props.request?.review_remark ?? '' })
  }, [form, props.request])

  const submitReview = (action: 'approve' | 'reject') =>
    form.handleSubmit(async (values) => {
      const success = await props.onSubmit(action, values.remark.trim())
      if (success) {
        props.onOpenChange(false)
      }
    })()

  return (
    <Dialog open={props.request !== null} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Review invoice request')}</DialogTitle>
          <DialogDescription>
            {t(
              'Approve the request before providing the final PDF download link.'
            )}
          </DialogDescription>
        </DialogHeader>
        {props.request ? (
          <Form {...form}>
            <div className='space-y-4'>
              <div className='bg-muted/40 grid grid-cols-2 gap-3 rounded-lg border p-3 text-sm'>
                <div>
                  <p className='text-muted-foreground'>{t('Invoice title')}</p>
                  <p className='mt-1 font-medium'>
                    {props.request.invoice_title}
                  </p>
                </div>
                <div>
                  <p className='text-muted-foreground'>{t('Invoice amount')}</p>
                  <p className='mt-1 font-semibold tabular-nums'>
                    {formatNumber(props.request.amount)}
                  </p>
                </div>
              </div>
              <FormField
                control={form.control}
                name='remark'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Review remark')}</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder={t(
                          'Optional for approval, recommended when rejecting'
                        )}
                        rows={4}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('The user can see this remark in the invoice folder.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <DialogFooter>
                <Button
                  type='button'
                  variant='destructive'
                  disabled={props.submitting}
                  onClick={() => void submitReview('reject')}
                >
                  {props.submitting ? (
                    <Spinner data-icon='inline-start' />
                  ) : null}
                  {t('Reject')}
                </Button>
                <Button
                  type='button'
                  disabled={props.submitting}
                  onClick={() => void submitReview('approve')}
                >
                  {props.submitting ? (
                    <Spinner data-icon='inline-start' />
                  ) : null}
                  {t('Approve')}
                </Button>
              </DialogFooter>
            </div>
          </Form>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
