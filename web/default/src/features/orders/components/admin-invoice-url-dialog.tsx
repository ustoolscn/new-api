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

import {
  invoiceURLSchema,
  type InvoiceURLFormValues,
} from '../lib/invoice-schema'
import type { InvoiceRequest } from '../types'

type AdminInvoiceURLDialogProps = {
  request: InvoiceRequest | null
  submitting: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (invoiceURL: string) => Promise<boolean>
}

export function AdminInvoiceURLDialog(props: AdminInvoiceURLDialogProps) {
  const { t } = useTranslation()
  const form = useForm<InvoiceURLFormValues>({
    resolver: zodResolver(invoiceURLSchema),
    defaultValues: { invoiceURL: '' },
  })

  useEffect(() => {
    form.reset({ invoiceURL: props.request?.invoice_url ?? '' })
  }, [form, props.request])

  const handleSubmit = form.handleSubmit(async (values) => {
    const success = await props.onSubmit(values.invoiceURL)
    if (success) {
      props.onOpenChange(false)
    }
  })

  return (
    <Dialog open={props.request !== null} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>
            {props.request?.status === 'issued'
              ? t('Replace link')
              : t('Provide PDF link')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Saving the link marks the approved request as issued and makes it available to the user.'
            )}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={handleSubmit} className='space-y-4'>
            <FormField
              control={form.control}
              name='invoiceURL'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('PDF download URL')}</FormLabel>
                  <FormControl>
                    <Input
                      type='url'
                      inputMode='url'
                      autoComplete='url'
                      placeholder='https://example.com/invoice.pdf'
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Enter a public or signed PDF download URL')}
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
                {t('Save and issue invoice')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
