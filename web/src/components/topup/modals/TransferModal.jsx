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
import { Modal, Typography, Input, InputNumber } from '@douyinfe/semi-ui';
import { CreditCard } from 'lucide-react';

const TransferModal = ({
  t,
  openTransfer,
  transfer,
  handleTransferCancel,
  userState,
  renderQuota,
  getQuotaPerUnit,
  transferAmount,
  setTransferAmount,
}) => {
  // This dialog is USD-only throughout: the underlying QuotaPerUnit conversion
  // is USD-denominated, and mixing user display currency with USD input causes
  // the kind of ¥... / $... inconsistency that misled users before.
  const formatUsd = (rawQuota) => {
    const unit = getQuotaPerUnit();
    if (!Number.isFinite(unit) || unit <= 0) return '$...';
    return `$${((Number(rawQuota) || 0) / unit).toFixed(2)}`;
  };

  return (
    <Modal
      title={
        <div className='flex items-center'>
          <CreditCard className='mr-2' size={18} />
          {t('划转邀请额度')}
        </div>
      }
      visible={openTransfer}
      onOk={transfer}
      onCancel={handleTransferCancel}
      maskClosable={false}
      centered
    >
      <div className='space-y-4'>
        <div>
          <Typography.Text strong className='block mb-2'>
            {t('可用邀请额度')}
          </Typography.Text>
          <Input
            value={formatUsd(userState?.user?.aff_quota)}
            disabled
            className='!rounded-lg'
          />
        </div>
        <div>
          <Typography.Text strong className='block mb-2'>
            {t('划转额度')} · {t('最低') + ' ' + formatUsd(getQuotaPerUnit())}
          </Typography.Text>
          <InputNumber
            min={getQuotaPerUnit()}
            max={userState?.user?.aff_quota || 0}
            step={getQuotaPerUnit()}
            value={transferAmount}
            formatter={(value) => {
              const unit = getQuotaPerUnit();
              if (!Number.isFinite(unit) || unit <= 0 || value === '' || value === null || value === undefined) {
                return value ?? '';
              }
              return `$ ${(Number(value) / unit).toFixed(2)}`;
            }}
            parser={(displayValue) => {
              const unit = getQuotaPerUnit();
              const cleaned = String(displayValue ?? '').replace(/[^\d.]/g, '');
              const parsed = parseFloat(cleaned);
              if (!Number.isFinite(parsed) || !Number.isFinite(unit) || unit <= 0) {
                return 0;
              }
              return Math.round(parsed * unit);
            }}
            onChange={(value) => setTransferAmount(value)}
            className='w-full !rounded-lg'
          />
        </div>
      </div>
    </Modal>
  );
};

export default TransferModal;
