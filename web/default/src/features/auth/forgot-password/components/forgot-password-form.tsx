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
import { ArrowRight, CheckIcon, CopyIcon, Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import type { z } from 'zod'

import { Turnstile } from '@/components/turnstile'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  resetPasswordByPhone,
  sendPasswordResetEmail,
  sendPasswordResetPhone,
} from '@/features/auth/api'
import {
  forgotPasswordEmailFormSchema,
  forgotPasswordFormSchema,
  forgotPasswordPhoneFormSchema,
  PASSWORD_RESET_COUNTDOWN,
  PHONE_VERIFICATION_COUNTDOWN,
} from '@/features/auth/constants'
import { useTurnstile } from '@/features/auth/hooks/use-turnstile'
import { useCountdown } from '@/hooks/use-countdown'
import { useStatus } from '@/hooks/use-status'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { cn } from '@/lib/utils'

type ResetMethod = 'email' | 'phone'

export function ForgotPasswordForm({
  className,
  ...props
}: React.HTMLAttributes<HTMLFormElement>) {
  const { t } = useTranslation()
  const [isLoading, setIsLoading] = useState(false)
  const [isSendingPhoneCode, setIsSendingPhoneCode] = useState(false)
  const [resetMethod, setResetMethod] = useState<ResetMethod>('email')
  const [newPassword, setNewPassword] = useState('')
  const [copied, setCopied] = useState(false)
  const { status } = useStatus()

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
  } = useCountdown({ initialSeconds: PASSWORD_RESET_COUNTDOWN })
  const {
    secondsLeft: phoneSecondsLeft,
    isActive: isPhoneCountdownActive,
    start: startPhoneCountdown,
  } = useCountdown({ initialSeconds: PHONE_VERIFICATION_COUNTDOWN })

  const form = useForm<z.infer<typeof forgotPasswordFormSchema>>({
    resolver: zodResolver(forgotPasswordFormSchema),
    defaultValues: { email: '', phone: '', code: '' },
  })
  const turnstileReady = !isTurnstileEnabled || Boolean(turnstileToken)
  const phoneResetEnabled = Boolean(
    status?.sms_verification_enabled ?? status?.data?.sms_verification_enabled
  )
  const phoneValue = form.watch('phone')

  useEffect(() => {
    if (!phoneResetEnabled && resetMethod === 'phone') {
      setResetMethod('email')
    }
  }, [phoneResetEnabled, resetMethod])

  async function onSubmit(data: z.infer<typeof forgotPasswordFormSchema>) {
    if (resetMethod === 'phone') {
      const parsed = forgotPasswordPhoneFormSchema.safeParse({
        phone: data.phone,
        code: data.code,
      })
      if (!parsed.success) {
        for (const issue of parsed.error.issues) {
          const field = issue.path[0]
          if (field === 'phone' || field === 'code') {
            form.setError(field, { message: t(issue.message) })
          }
        }
        return
      }

      setIsLoading(true)
      try {
        const res = await resetPasswordByPhone(
          parsed.data.phone,
          parsed.data.code
        )
        if (res?.success && typeof res.data === 'string') {
          const password = res.data
          setNewPassword(password)
          const copySuccess = await copyToClipboard(password)
          if (copySuccess) {
            setCopied(true)
            toast.success(
              t('Password reset and copied to clipboard: {{password}}', {
                password,
              })
            )
            setTimeout(() => setCopied(false), 2000)
          } else {
            toast.success(t('Password reset: {{password}}', { password }))
          }
        } else {
          toast.error(res?.message || t('Failed to reset password'))
        }
      } catch {
        // Errors are handled by global interceptor
      } finally {
        setIsLoading(false)
      }
      return
    }

    const parsed = forgotPasswordEmailFormSchema.safeParse({
      email: data.email,
    })
    if (!parsed.success) {
      form.setError('email', {
        message: t('Please enter a valid email address'),
      })
      return
    }

    if (!validateTurnstile()) return

    setIsLoading(true)
    try {
      const res = await sendPasswordResetEmail(
        parsed.data.email,
        turnstileToken
      )
      if (res?.success) {
        form.reset({ email: '', phone: form.getValues('phone'), code: '' })
        startCountdown()
        toast.success(t('Reset email sent, please check your inbox'))
      } else {
        toast.error(res?.message || t('Failed to send reset email'))
      }
    } catch {
      // Errors are handled by global interceptor
    } finally {
      setIsLoading(false)
    }
  }

  async function handleSendPhoneCode() {
    const phone = phoneValue?.trim()
    if (!phone) {
      form.setError('phone', {
        message: t('Please enter your phone number'),
      })
      return
    }
    if (!validateTurnstile()) return

    setIsSendingPhoneCode(true)
    try {
      const res = await sendPasswordResetPhone(phone, turnstileToken)
      if (res?.success) {
        startPhoneCountdown()
        toast.success(t('Password reset SMS sent'))
      } else {
        toast.error(res?.message || t('Failed to send password reset SMS'))
      }
    } catch {
      // Errors are handled by global interceptor
    } finally {
      setIsSendingPhoneCode(false)
    }
  }

  async function handleCopyPassword() {
    if (!newPassword) return

    const copySuccess = await copyToClipboard(newPassword)
    if (copySuccess) {
      setCopied(true)
      toast.success(
        t('Password copied to clipboard: {{password}}', {
          password: newPassword,
        })
      )
      setTimeout(() => setCopied(false), 2000)
    }
  }

  function handleMethodChange(value: string) {
    setResetMethod(value as ResetMethod)
    setNewPassword('')
    setCopied(false)
  }

  let phoneCodeButtonContent = <>{t('Send code')}</>
  if (isPhoneCountdownActive) {
    phoneCodeButtonContent = (
      <>{t('Resend ({{seconds}}s)', { seconds: phoneSecondsLeft })}</>
    )
  } else if (isSendingPhoneCode) {
    phoneCodeButtonContent = <Loader2 className='h-4 w-4 animate-spin' />
  }

  let submitButtonContent = <>{t('Send reset email')}</>
  if (resetMethod === 'phone') {
    submitButtonContent = (
      <>
        {isLoading ? <Loader2 className='animate-spin' /> : null}
        {t('Reset password')}
      </>
    )
  } else if (isActive) {
    submitButtonContent = (
      <>{t('Resend ({{seconds}}s)', { seconds: secondsLeft })}</>
    )
  }

  let submitButtonIcon = null
  if (resetMethod === 'email') {
    submitButtonIcon = isLoading ? (
      <Loader2 className='animate-spin' />
    ) : (
      <ArrowRight />
    )
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className={cn('grid gap-4', className)}
        {...props}
      >
        {phoneResetEnabled ? (
          <Tabs value={resetMethod} onValueChange={handleMethodChange}>
            <TabsList className='w-full'>
              <TabsTrigger value='email'>{t('Reset by email')}</TabsTrigger>
              <TabsTrigger value='phone'>{t('Reset by phone')}</TabsTrigger>
            </TabsList>
            <TabsContent value='email' className='grid gap-4'>
              <FormField
                control={form.control}
                name='email'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Email')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('name@example.com')}
                        type='email'
                        autoComplete='email'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>
            <TabsContent value='phone' className='grid gap-4'>
              <FormField
                control={form.control}
                name='phone'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Phone Number')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Enter your phone number')}
                        inputMode='tel'
                        autoComplete='tel'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className='flex items-end gap-2'>
                <FormField
                  control={form.control}
                  name='code'
                  render={({ field }) => (
                    <FormItem className='flex-1'>
                      <FormLabel>{t('SMS verification code')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t('SMS verification code')}
                          inputMode='numeric'
                          autoComplete='one-time-code'
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <Button
                  variant='outline'
                  type='button'
                  disabled={
                    isLoading ||
                    isSendingPhoneCode ||
                    isPhoneCountdownActive ||
                    !phoneValue ||
                    !turnstileReady
                  }
                  onClick={handleSendPhoneCode}
                >
                  {phoneCodeButtonContent}
                </Button>
              </div>

              {newPassword && (
                <div className='grid gap-2'>
                  <Label>{t('New password')}</Label>
                  <div className='flex gap-2'>
                    <Input value={newPassword} readOnly className='font-mono' />
                    <Button
                      type='button'
                      size='icon'
                      variant='outline'
                      aria-label={t('Copy')}
                      onClick={handleCopyPassword}
                    >
                      {copied ? (
                        <CheckIcon className='h-4 w-4' />
                      ) : (
                        <CopyIcon className='h-4 w-4' />
                      )}
                    </Button>
                  </div>
                  <p className='text-muted-foreground text-xs'>
                    {t('Password has been copied to clipboard')}
                  </p>
                </div>
              )}
            </TabsContent>
          </Tabs>
        ) : (
          <FormField
            control={form.control}
            name='email'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Email')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('name@example.com')}
                    type='email'
                    autoComplete='email'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        )}

        <Button
          type='submit'
          className='mt-2'
          disabled={
            isLoading ||
            (resetMethod === 'email' && (isActive || !turnstileReady))
          }
        >
          {submitButtonContent}
          {submitButtonIcon}
        </Button>

        {isTurnstileEnabled && (
          <div className='mt-2'>
            <Turnstile
              siteKey={turnstileSiteKey}
              onVerify={setTurnstileToken}
            />
          </div>
        )}
      </form>
    </Form>
  )
}
