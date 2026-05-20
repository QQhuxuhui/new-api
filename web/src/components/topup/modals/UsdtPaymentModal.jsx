/*
USDT (TRC20) payment instruction modal.
Shown after a successful create-order response from /api/user/pay/usdt or
/api/user/plan/purchase/pay (with payment_method=usdt).
*/

import React, { useEffect, useState } from 'react';
import { Modal, Typography, Button, Space, Tag, Divider } from '@douyinfe/semi-ui';
import { QRCodeSVG } from 'qrcode.react';
import { useTranslation } from 'react-i18next';
import { copy, showSuccess } from '../../../helpers';

const { Text, Title } = Typography;

// Compute remaining seconds till expirationTime (Unix seconds)
const useCountdown = (expirationTime) => {
  const [remaining, setRemaining] = useState(0);
  useEffect(() => {
    if (!expirationTime) {
      setRemaining(0);
      return;
    }
    const tick = () => {
      const left = Math.max(0, Math.floor(expirationTime - Date.now() / 1000));
      setRemaining(left);
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [expirationTime]);
  return remaining;
};

const formatCountdown = (sec) => {
  if (sec <= 0) return '00:00';
  const m = Math.floor(sec / 60).toString().padStart(2, '0');
  const s = (sec % 60).toString().padStart(2, '0');
  return `${m}:${s}`;
};

const UsdtPaymentModal = ({ visible, onClose, data, t: tProp }) => {
  const { t: tHook } = useTranslation();
  const t = tProp || tHook;

  const remaining = useCountdown(data?.expiration_time);
  const expired = data?.expiration_time && remaining <= 0;

  const handleCopy = async (val, label) => {
    if (!val) return;
    await copy(val);
    showSuccess(t(label || '已复制'));
  };

  if (!data) return null;

  return (
    <Modal
      title={t('USDT (TRC20) 支付')}
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={460}
      closable
    >
      <div className='flex flex-col items-center gap-3'>
        <div className='flex items-center gap-2'>
          <Tag color={expired ? 'red' : 'green'} size='large'>
            {expired
              ? t('订单已过期')
              : `${t('剩余支付时间')}: ${formatCountdown(remaining)}`}
          </Tag>
        </div>

        <Title heading={3} style={{ marginTop: 8, marginBottom: 0 }}>
          {Number(data.actual_amount).toFixed(2)} <span style={{ fontSize: 16 }}>USDT</span>
        </Title>
        <Text type='tertiary' size='small'>
          {t('请按上方实际金额精确转账（防碰撞机制，多/少都识别不到订单）')}
        </Text>

        <div className='p-3 bg-white rounded' style={{ border: '1px solid var(--semi-color-border)' }}>
          <QRCodeSVG value={data.token || ''} size={180} />
        </div>

        <div className='w-full'>
          <Text size='small' type='tertiary'>{t('收款地址 (TRC20)')}</Text>
          <div className='flex items-center gap-2 mt-1 p-2 rounded' style={{ background: 'var(--semi-color-fill-0)' }}>
            <Text
              className='break-all flex-1'
              style={{ fontFamily: 'monospace', fontSize: 12 }}
            >
              {data.token}
            </Text>
            <Button
              size='small'
              onClick={() => handleCopy(data.token, '收款地址已复制')}
            >
              {t('复制')}
            </Button>
          </div>
        </div>

        <div className='w-full flex gap-2 mt-1'>
          <Button
            size='small'
            block
            onClick={() => handleCopy(String(data.actual_amount), '金额已复制')}
          >
            {t('复制金额')}
          </Button>
          {data.payment_url && (
            <Button
              size='small'
              theme='solid'
              type='primary'
              block
              onClick={() => window.open(data.payment_url, '_blank')}
            >
              {t('打开支付页')}
            </Button>
          )}
        </div>

        <Divider style={{ margin: '6px 0' }} />
        <div className='w-full text-xs' style={{ color: 'var(--semi-color-text-2)', lineHeight: 1.6 }}>
          <div>· {t('仅支持 TRC20 网络（波场链）USDT')}</div>
          <div>· {t('金额必须与上方完全一致，否则系统无法识别')}</div>
          <div>· {t('转账完成后将自动到账，请勿关闭页面太久')}</div>
          {data.trade_id && (
            <div className='mt-1'>
              {t('网关订单号')}: <span style={{ fontFamily: 'monospace' }}>{data.trade_id}</span>
            </div>
          )}
        </div>
      </div>
    </Modal>
  );
};

export default UsdtPaymentModal;
