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
import * as z from 'zod'

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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const botProtectionSchema = z.object({
  LoginCaptchaMode: z.enum(['image', 'turnstile']),
  TurnstileCheckEnabled: z.boolean(),
  TurnstileSiteKey: z.string().optional(),
  TurnstileSecretKey: z.string().optional(),
})

type BotProtectionFormValues = z.infer<typeof botProtectionSchema>

type BotProtectionSectionProps = {
  defaultValues: BotProtectionFormValues
}

export function BotProtectionSection({
  defaultValues,
}: BotProtectionSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const form = useForm<BotProtectionFormValues>({
    resolver: zodResolver(botProtectionSchema),
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (data: BotProtectionFormValues) => {
    const updates = Object.entries(data).filter(
      ([key, value]) =>
        value !== defaultValues[key as keyof BotProtectionFormValues]
    )
    // Save credentials before enabling a provider. Switch to image mode first
    // when clearing credentials so server-side validation cannot be bypassed.
    const orderedUpdates = updates.sort(([left], [right]) => {
      const priority = (key: string) => {
        if (key === 'LoginCaptchaMode')
          {return data.LoginCaptchaMode === 'image' ? -1 : 1}
        if (key === 'TurnstileCheckEnabled') return 2
        return 0
      }
      return priority(left) - priority(right)
    })

    for (const [key, value] of orderedUpdates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }
  }

  return (
    <SettingsSection title={t('Bot Protection')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='LoginCaptchaMode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Login CAPTCHA method')}</FormLabel>
                <Select
                  items={[
                    { value: 'image', label: t('Local image CAPTCHA') },
                    { value: 'turnstile', label: 'Cloudflare Turnstile' },
                  ]}
                  value={field.value}
                  onValueChange={field.onChange}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value='image'>
                      {t('Local image CAPTCHA')}
                    </SelectItem>
                    <SelectItem value='turnstile'>
                      Cloudflare Turnstile
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t(
                    'Password login always requires CAPTCHA. Image challenges expire after 10 minutes and can only be submitted once.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='TurnstileCheckEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable Turnstile')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Also require Turnstile for registration and other protected operations'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='TurnstileSiteKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Site Key')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Your Turnstile site key')}
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='TurnstileSecretKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Secret Key')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    placeholder={t('Your Turnstile secret key')}
                    autoComplete='new-password'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
