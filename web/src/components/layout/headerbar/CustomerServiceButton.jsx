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

import React, { useState } from 'react';
import { Button, Popover, Modal, Image } from '@douyinfe/semi-ui';

const CustomerServiceButton = ({ customerServiceQRCode, isMobile, t }) => {
  const [modalVisible, setModalVisible] = useState(false);

  // If no QR code is configured, don't render the button
  if (!customerServiceQRCode) {
    return null;
  }

  const qrCodeContent = (
    <div style={{ padding: 8, textAlign: 'center' }}>
      <div style={{ marginBottom: 8, fontWeight: 500 }}>
        {t('客服二维码')}
      </div>
      <Image
        src={customerServiceQRCode}
        width={200}
        height={400}
        preview={{
          src: customerServiceQRCode,
        }}
        alt={t('客服二维码')}
        style={{ borderRadius: 4, objectFit: 'contain' }}
      />
    </div>
  );

  const buttonProps = {
    'aria-label': t('联系客服'),
    theme: 'solid',
    type: 'primary',
    size: 'small',
    style: {
      background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
      border: 'none',
      fontWeight: 500,
    },
    className: '!shadow-sm hover:!shadow-md transition-shadow',
  };

  // Mobile: use modal on click
  if (isMobile) {
    return (
      <>
        <Button {...buttonProps} onClick={() => setModalVisible(true)}>
          {t('联系客服')}
        </Button>
        <Modal
          title={t('联系客服')}
          visible={modalVisible}
          onCancel={() => setModalVisible(false)}
          footer={null}
          width={280}
        >
          {qrCodeContent}
        </Modal>
      </>
    );
  }

  // Desktop: use popover on hover
  return (
    <Popover
      content={qrCodeContent}
      position='bottomRight'
      trigger='hover'
      showArrow
    >
      <Button {...buttonProps}>
        {t('联系客服')}
      </Button>
    </Popover>
  );
};

export default CustomerServiceButton;
