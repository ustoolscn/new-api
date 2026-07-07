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
import { Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Turnstile } from '@/components/turnstile'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useTurnstile } from '@/features/auth/hooks/use-turnstile'
import { useCountdown } from '@/hooks/use-countdown'

import { bindPhone, sendPhoneVerification } from '../../api'

interface PhoneBindDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentPhone?: string
  onSuccess: () => void
}

export function PhoneBindDialog({
  open,
  onOpenChange,
  currentPhone,
  onSuccess,
}: PhoneBindDialogProps) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [sendingCode, setSendingCode] = useState(false)
  const [phone, setPhone] = useState('')
  const [code, setCode] = useState('')
  const {
    isTurnstileEnabled,
    turnstileSiteKey,
    turnstileToken,
    setTurnstileToken,
    validateTurnstile,
  } = useTurnstile()
  const {
    secondsLeft,
    isActive,
    start: startCountdown,
    reset: resetCountdown,
  } = useCountdown({
    initialSeconds: 60,
  })

  const handleSendCode = async () => {
    if (!phone) {
      toast.error(t('Please enter your phone number'))
      return
    }
    if (!validateTurnstile()) return

    try {
      setSendingCode(true)
      const response = await sendPhoneVerification(phone, turnstileToken)

      if (response.success) {
        toast.success(t('Verification SMS sent'))
        startCountdown()
      } else {
        toast.error(response.message || t('Failed to send verification SMS'))
      }
    } catch {
      toast.error(t('Failed to send verification SMS'))
    } finally {
      setSendingCode(false)
    }
  }

  const handleBind = async () => {
    if (!phone || !code) {
      toast.error(t('Please enter phone number and verification code'))
      return
    }

    try {
      setLoading(true)
      const response = await bindPhone(phone, code)

      if (response.success) {
        toast.success(t('Phone bound successfully!'))
        onOpenChange(false)
        onSuccess()
        setPhone('')
        setCode('')
        resetCountdown()
      } else {
        toast.error(response.message || t('Failed to bind phone'))
      }
    } catch {
      toast.error(t('Failed to bind phone'))
    } finally {
      setLoading(false)
    }
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (!loading) {
      onOpenChange(nextOpen)
      if (!nextOpen) {
        setPhone('')
        setCode('')
        resetCountdown()
      }
    }
  }

  let sendButtonContent = t('Send')
  if (isActive) {
    sendButtonContent = `${secondsLeft}s`
  } else if (sendingCode) {
    sendButtonContent = t('Sending...')
  }

  return (
    <Dialog
      open={open}
      onOpenChange={handleOpenChange}
      title={t('Bind Phone Number')}
      description={
        currentPhone
          ? t('Current phone: {{phone}}. Enter a new phone number to change.', {
              phone: currentPhone,
            })
          : t('Bind a phone number to your account.')
      }
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => handleOpenChange(false)}
            disabled={loading}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            onClick={handleBind}
            disabled={loading || !phone || !code}
          >
            {loading && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {loading ? t('Binding...') : t('Bind Phone Number')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-4'>
        <div className='space-y-2'>
          <Label htmlFor='phone'>{t('Phone Number')}</Label>
          <Input
            id='phone'
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            placeholder={t('Enter your phone number')}
            disabled={loading}
            inputMode='tel'
            autoComplete='tel'
          />
        </div>

        <div className='space-y-2'>
          <Label htmlFor='phone-code'>{t('Verification Code')}</Label>
          <div className='flex gap-2'>
            <Input
              id='phone-code'
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder={t('Enter code')}
              disabled={loading}
              inputMode='numeric'
              autoComplete='one-time-code'
              maxLength={6}
            />
            <Button
              type='button'
              variant='outline'
              onClick={handleSendCode}
              disabled={sendingCode || isActive || !phone}
            >
              {sendButtonContent}
            </Button>
          </div>
        </div>

        {isTurnstileEnabled && (
          <Turnstile
            siteKey={turnstileSiteKey}
            onVerify={setTurnstileToken}
          />
        )}
      </div>
    </Dialog>
  )
}
