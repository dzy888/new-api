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
import { RefreshCw } from 'lucide-react'
import {
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
  type Ref,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'

export type ImageVerification = { id: string; code: string }
export type LoginCaptchaHandle = { reload: () => Promise<void> }

export function LoginCaptcha({
  onChange,
  ref,
}: {
  onChange: (value: ImageVerification) => void
  ref?: Ref<LoginCaptchaHandle>
}) {
  const { t } = useTranslation()
  const [captcha, setCaptcha] = useState<{ id: string; image: string } | null>(
    null
  )
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const sequence = useRef(0)
  const reload = useCallback(async () => {
    const current = ++sequence.current
    setBusy(true)
    setFailed(false)
    setCode('')
    setCaptcha(null)
    onChange({ id: '', code: '' })
    try {
      const response = await api.get('/api/user/login/captcha', {
        skipAuthRefresh: true,
        disableDuplicate: true,
      })
      const data = response.data?.data
      if (
        !response.data?.success ||
        typeof data?.id !== 'string' ||
        !data?.image?.startsWith('data:image/png;base64,')
      )
        {throw new Error('Invalid CAPTCHA')}
      if (sequence.current === current) setCaptcha(data)
    } catch {
      if (sequence.current === current) setFailed(true)
    } finally {
      if (sequence.current === current) setBusy(false)
    }
  }, [onChange])
  useImperativeHandle(ref, () => ({ reload }), [reload])
  useEffect(() => {
    void reload()
    return () => {
      sequence.current += 1
    }
  }, [reload])
  return (
    <div className='grid gap-2'>
      <Label htmlFor='login-captcha-code'>{t('Image CAPTCHA')}</Label>
      <div className='flex items-center gap-2'>
        <Input
          id='login-captcha-code'
          value={code}
          autoComplete='off'
          inputMode='numeric'
          placeholder={t('Enter the code shown')}
          disabled={busy || !captcha}
          onChange={(event) => {
            setCode(event.target.value)
            onChange({ id: captcha?.id || '', code: event.target.value })
          }}
        />
        <Button
          type='button'
          variant='outline'
          className='h-11 w-32 shrink-0 p-0'
          disabled={busy}
          onClick={() => void reload()}
          aria-label={t('Refresh CAPTCHA')}
        >
          {captcha ? (
            <img
              src={captcha.image}
              alt={t('Login CAPTCHA')}
              className='h-full w-full rounded-md object-contain'
            />
          ) : (
            <RefreshCw className={busy ? 'animate-spin' : ''} />
          )}
        </Button>
      </div>
      {failed && (
        <p role='alert' className='text-destructive text-xs'>
          {t('Unable to load CAPTCHA. Click refresh to try again.')}
        </p>
      )}
    </div>
  )
}
