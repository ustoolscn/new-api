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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { assignReferralInvitee } from '../api'

type AddReferralInviteeDialogProps = {
  inviterId: number
  inviterName?: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function AddReferralInviteeDialog(props: AddReferralInviteeDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [identifier, setIdentifier] = useState('')

  const mutation = useMutation({
    mutationFn: async () => {
      const value = identifier.trim()
      if (!value) {
        throw new Error(t('Please enter a user ID or username'))
      }
      const asId = Number(value)
      const payload =
        Number.isInteger(asId) && asId > 0
          ? { user_id: asId }
          : { username: value }
      const response = await assignReferralInvitee(props.inviterId, payload)
      if (!response.success) {
        throw new Error(response.message || t('Failed to add invitee'))
      }
      return response.data
    },
    onSuccess: async () => {
      toast.success(t('Invitee added successfully'))
      setIdentifier('')
      props.onOpenChange(false)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['admin-referral-overview'] }),
        queryClient.invalidateQueries({ queryKey: ['admin-referral-users'] }),
        queryClient.invalidateQueries({ queryKey: ['invitee-activity-report'] }),
        queryClient.invalidateQueries({ queryKey: ['users'] }),
      ])
      props.onSuccess?.()
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to add invitee'))
    },
  })

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          setIdentifier('')
        }
        props.onOpenChange(open)
      }}
    >
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Add invitee')}</DialogTitle>
          <DialogDescription>
            {props.inviterName
              ? t(
                  'Link an existing user as an invitee of {{username}}. This only sets the invite relationship and does not grant registration rewards.',
                  { username: props.inviterName }
                )
              : t(
                  'Link an existing user as an invitee. This only sets the invite relationship and does not grant registration rewards.'
                )}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-2 py-2'>
          <Label htmlFor='add-invitee-identifier'>
            {t('User ID or username')}
          </Label>
          <Input
            id='add-invitee-identifier'
            value={identifier}
            onChange={(event) => setIdentifier(event.target.value)}
            placeholder={t('Enter user ID or username')}
            autoFocus
            onKeyDown={(event) => {
              if (event.key === 'Enter' && !mutation.isPending) {
                event.preventDefault()
                mutation.mutate()
              }
            }}
          />
        </div>

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={mutation.isPending}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending || !identifier.trim()}
          >
            {mutation.isPending ? t('Adding...') : t('Add invitee')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
