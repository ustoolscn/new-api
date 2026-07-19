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
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { formatNumber } from '@/lib/format'

import {
  invoiceApplicationSchema,
  type InvoiceApplicationFormValues,
} from '../lib/invoice-schema'
import type { UserOrder } from '../types'

type InvoiceApplicationDialogProps = {
  open: boolean
  orders: UserOrder[]
  submitting: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (invoiceTitle: string) => Promise<boolean>
}

export function InvoiceApplicationDialog(props: InvoiceApplicationDialogProps) {
  const { t } = useTranslation()
  const form = useForm<InvoiceApplicationFormValues>({
    resolver: zodResolver(invoiceApplicationSchema),
    defaultValues: { invoiceTitle: '' },
  })
  const totalAmount = props.orders.reduce((sum, order) => sum + order.money, 0)

  useEffect(() => {
    if (!props.open) {
      form.reset({ invoiceTitle: '' })
    }
  }, [form, props.open])

  const handleSubmit = form.handleSubmit(async (values) => {
    const success = await props.onSubmit(values.invoiceTitle.trim())
    if (success) {
      form.reset({ invoiceTitle: '' })
      props.onOpenChange(false)
    }
  })

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Apply for invoice')}</DialogTitle>
          <DialogDescription>
            {t(
              'The selected successful orders will be combined into one invoice request.'
            )}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={handleSubmit} className='space-y-4'>
            <div className='bg-muted/40 grid grid-cols-2 gap-3 rounded-lg border p-3'>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Selected orders')}
                </p>
                <p className='mt-1 text-lg font-semibold tabular-nums'>
                  {props.orders.length}
                </p>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Invoice amount')}
                </p>
                <p className='mt-1 text-lg font-semibold tabular-nums'>
                  {formatNumber(totalAmount)}
                </p>
              </div>
            </div>
            <FormField
              control={form.control}
              name='invoiceTitle'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Invoice title')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('Enter company or personal invoice title')}
                      autoComplete='organization'
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'The title will be submitted to the administrator for review.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                disabled={props.submitting}
                onClick={() => props.onOpenChange(false)}
              >
                {t('Cancel')}
              </Button>
              <Button type='submit' disabled={props.submitting}>
                {props.submitting ? <Spinner data-icon='inline-start' /> : null}
                {t('Submit invoice request')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
