/*
Copyright (C) 2025 QuantumNous

Modal for admin to record an offline reward payout.
*/

import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Modal,
  Form,
  InputNumber,
  Input,
  Typography,
  Space,
} from '@douyinfe/semi-ui';
import { InviterRewardAPI } from '../../../../services/inviterRewardApi';
import { showSuccess } from '../../../../helpers';
import { formatUSDAmount } from '../../../../utils/currency';

const { Text } = Typography;

const PayoutInviterRewardModal = ({
  visible,
  inviterId,
  pendingTotalUsd = 0,
  defaultPercent = 10,
  onClose,
  onSuccess,
}) => {
  const { t } = useTranslation();
  const [submitting, setSubmitting] = useState(false);
  const [actualAmount, setActualAmount] = useState(0);
  const [note, setNote] = useState('');

  const suggested = useMemo(() => {
    const v = (Number(pendingTotalUsd) || 0) * (Number(defaultPercent) || 0) / 100;
    return Math.round(v * 100) / 100;
  }, [pendingTotalUsd, defaultPercent]);

  useEffect(() => {
    if (visible) {
      setActualAmount(suggested);
      setNote('');
    }
  }, [visible, suggested]);

  const canSubmit = Number(actualAmount) > 0 && Number(pendingTotalUsd) > 0 && !submitting;

  const handleSubmit = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    try {
      const created = await InviterRewardAPI.createPayout(inviterId, {
        payout_amount_usd: Number(actualAmount),
        note,
      });
      showSuccess(
        t('已发放 {{amount}}，覆盖充值 {{recharge}}', {
          amount: formatUSDAmount(created.payout_amount_usd),
          recharge: formatUSDAmount(created.recharge_total_usd),
        })
      );
      onSuccess && onSuccess(created);
      onClose && onClose();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title={t('发放邀请激励')}
      visible={visible}
      onCancel={onClose}
      onOk={handleSubmit}
      okButtonProps={{ disabled: !canSubmit, loading: submitting }}
      cancelText={t('取消')}
      okText={t('确认发放')}
      width={460}
    >
      <Space vertical style={{ width: '100%' }} spacing="loose">
        <div>
          <Text type="tertiary">{t('待激励充值总额')}</Text>
          <div>
            <Text strong style={{ fontSize: 18 }}>
              {formatUSDAmount(pendingTotalUsd)}
            </Text>
          </div>
        </div>
        <div>
          <Text type="tertiary">
            {t('系统默认比例')}：{defaultPercent}%
            <br />
            {t('建议奖励金额')}：{formatUSDAmount(suggested)}
          </Text>
        </div>
        <Form labelPosition="top">
          <Form.Slot label={t('实际奖励金额')}>
            <InputNumber
              value={actualAmount}
              min={0}
              step={0.01}
              precision={2}
              prefix="$"
              style={{ width: '100%' }}
              onChange={setActualAmount}
            />
          </Form.Slot>
          <Form.Slot label={t('备注（可选）')}>
            <Input
              value={note}
              onChange={setNote}
              placeholder={t('如：微信转账 流水号 xxx')}
              maxLength={500}
            />
          </Form.Slot>
        </Form>
      </Space>
    </Modal>
  );
};

export default PayoutInviterRewardModal;
