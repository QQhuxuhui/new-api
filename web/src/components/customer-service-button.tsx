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
import { Headset } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card'
import { useStatus } from '@/hooks/use-status'
import { isHttpUrl } from '@/lib/content-format'
import { cn } from '@/lib/utils'

type SupportQRCodeProps = {
  src: string
  className?: string
}

/** The configured QR code image, or a notice when it cannot be loaded. */
export function SupportQRCode(props: SupportQRCodeProps) {
  const { t } = useTranslation()
  // Remember which URL failed, so a new URL is tried again.
  const [failedSrc, setFailedSrc] = useState<string | null>(null)

  if (failedSrc === props.src) {
    return (
      <p className='text-muted-foreground py-6 text-center text-sm'>
        {t('QR code failed to load')}
      </p>
    )
  }
  return (
    <img
      src={props.src}
      alt={t('Support QR code')}
      onError={() => setFailedSrc(props.src)}
      className={cn('mx-auto h-auto w-full object-contain', props.className)}
    />
  )
}

/**
 * Header button that shows the customer service QR code (for example a QQ
 * group) configured by the administrator. Hovering shows it on desktop;
 * clicking, tapping, or pressing Enter opens it larger in a dialog. Nothing
 * renders until a valid http(s) image URL is configured.
 */
export function CustomerServiceButton(props: { className?: string }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [dialogOpen, setDialogOpen] = useState(false)
  const qrCode =
    typeof status?.CustomerServiceQRCode === 'string'
      ? status.CustomerServiceQRCode.trim()
      : ''

  if (!isHttpUrl(qrCode)) return null

  return (
    <>
      <HoverCard>
        <HoverCardTrigger
          delay={150}
          render={
            <Button
              type='button'
              size='sm'
              aria-label={t('Contact support')}
              onClick={() => setDialogOpen(true)}
              className={cn(
                'h-8 rounded-lg px-2.5 text-xs font-medium sm:px-3',
                props.className
              )}
            />
          }
        >
          <Headset aria-hidden='true' className='size-3.5' />
          <span className='hidden sm:inline'>{t('Contact support')}</span>
        </HoverCardTrigger>
        <HoverCardContent align='end' className='w-56 p-3'>
          <p className='mb-2 text-center text-sm font-medium'>
            {t('Scan to join our QQ group')}
          </p>
          <SupportQRCode src={qrCode} className='max-h-96' />
        </HoverCardContent>
      </HoverCard>
      <Dialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={t('Contact support')}
        description={t('Scan to join our QQ group')}
        contentClassName='sm:max-w-sm'
      >
        <SupportQRCode src={qrCode} className='max-h-[70vh]' />
      </Dialog>
    </>
  )
}
