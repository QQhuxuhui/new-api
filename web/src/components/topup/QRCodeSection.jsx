/*
Copyright (C) 2025 QuantumNous

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

import React from 'react';
import { Card, Image } from '@douyinfe/semi-ui';

const QRCodeSection = ({ customerServiceQRCode, xianyuQRCode, t }) => {
  // Don't render if both QR codes are not configured
  if (!customerServiceQRCode && !xianyuQRCode) {
    return null;
  }

  return (
    <div className='w-full'>
      <div className='mb-4 text-lg font-semibold text-semi-color-text-0'>
        {t('更多服务')}
      </div>
      <div className='grid grid-cols-1 md:grid-cols-2 gap-4'>
        {/* Customer Service QR Code */}
        {customerServiceQRCode && (
          <Card
            className='w-full'
            bodyStyle={{ padding: '20px', textAlign: 'center' }}
          >
            <div className='mb-3 font-medium text-semi-color-text-0'>
              {t('客服联系方式')}
            </div>
            <Image
              src={customerServiceQRCode}
              width={180}
              height={360}
              preview={{
                src: customerServiceQRCode,
              }}
              alt={t('扫描二维码联系客服')}
              style={{ borderRadius: 8, margin: '0 auto', objectFit: 'contain' }}
            />
            <div className='mt-3 text-sm text-semi-color-text-2'>
              {t('扫描二维码联系客服')}
            </div>
          </Card>
        )}

        {/* Xianyu Shop QR Code */}
        {xianyuQRCode && (
          <Card
            className='w-full'
            bodyStyle={{ padding: '20px', textAlign: 'center' }}
          >
            <div className='mb-3 font-medium text-semi-color-text-0'>
              {t('闲鱼店铺')}
            </div>
            <Image
              src={xianyuQRCode}
              width={180}
              height={360}
              preview={{
                src: xianyuQRCode,
              }}
              alt={t('扫描二维码访问闲鱼店铺')}
              style={{ borderRadius: 8, margin: '0 auto', objectFit: 'contain' }}
            />
            <div className='mt-3 text-sm text-semi-color-text-2'>
              {t('扫描二维码访问闲鱼店铺')}
            </div>
          </Card>
        )}
      </div>
    </div>
  );
};

export default QRCodeSection;
