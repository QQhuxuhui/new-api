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

import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import {
  Card,
  Typography,
  Button,
  Space,
  Spin,
  Divider,
  Tag,
  Radio,
  RadioGroup,
  Banner,
  Toast,
} from '@douyinfe/semi-ui';
import {
  IconAlipayCircle,
  IconWechatpay,
  IconClock,
  IconCheckCircle,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../helpers';

const { Title, Text } = Typography;

const OrderConfirm = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { orderId } = useParams();
  const [loading, setLoading] = useState(true);
  const [paying, setPaying] = useState(false);
  const [order, setOrder] = useState(null);
  const [paymentMethod, setPaymentMethod] = useState('alipay');
  const [countdown, setCountdown] = useState(0);

  // Load order details
  const loadOrder = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/user/plan/purchase/my-orders?page=1&page_size=100');
      const { success, message, data } = res.data;
      if (success && data && data.orders) {
        const targetOrder = data.orders.find(o => o.order_id === parseInt(orderId));
        if (targetOrder) {
          setOrder(targetOrder);
          // Calculate countdown
          const now = Date.now();
          const expiredAt = targetOrder.expired_at;
          const remaining = Math.max(0, Math.floor((expiredAt - now) / 1000));
          setCountdown(remaining);
        } else {
          showError(t('订单不存在'));
          navigate('/plans');
        }
      } else {
        showError(message || t('加载订单失败'));
        navigate('/plans');
      }
    } catch (e) {
      showError(e.message || t('网络错误'));
      navigate('/plans');
    }
    setLoading(false);
  };

  useEffect(() => {
    if (orderId) {
      loadOrder();
    }
  }, [orderId]);

  // Countdown timer
  useEffect(() => {
    if (countdown > 0) {
      const timer = setTimeout(() => {
        setCountdown(countdown - 1);
      }, 1000);
      return () => clearTimeout(timer);
    } else if (countdown === 0 && order && order.status === 'pending') {
      // Order expired
      Toast.warning(t('订单已过期'));
      setTimeout(() => {
        navigate('/plans');
      }, 2000);
    }
  }, [countdown, order]);

  // Handle payment
  const handlePay = async () => {
    if (!order || order.status !== 'pending') {
      showError(t('订单状态异常'));
      return;
    }

    if (countdown <= 0) {
      showError(t('订单已过期'));
      return;
    }

    setPaying(true);
    try {
      const res = await API.post('/api/user/plan/purchase/pay', {
        order_id: order.order_id,
        payment_method: paymentMethod,
      });
      const { success, message, data } = res.data;
      if (success && data) {
        // Open payment URL in new window
        if (data.payment_url) {
          window.location.href = data.payment_url;
        } else if (data.params) {
          // Handle payment form submission
          const form = document.createElement('form');
          form.method = 'POST';
          form.action = data.url || '';
          Object.keys(data.params).forEach(key => {
            const input = document.createElement('input');
            input.type = 'hidden';
            input.name = key;
            input.value = data.params[key];
            form.appendChild(input);
          });
          document.body.appendChild(form);
          form.submit();
        }
      } else {
        showError(message || t('支付失败'));
        setPaying(false);
      }
    } catch (e) {
      showError(e.message || t('网络错误'));
      setPaying(false);
    }
  };

  // Format countdown
  const formatCountdown = (seconds) => {
    const minutes = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
  };

  if (loading) {
    return (
      <div className='flex items-center justify-center min-h-screen'>
        <Spin size='large' />
      </div>
    );
  }

  if (!order) {
    return null;
  }

  // Show different UI for different order statuses
  if (order.status === 'delivered') {
    return (
      <div className='min-h-screen bg-[var(--semi-color-bg-0)] py-12 px-4'>
        <div className='max-w-2xl mx-auto'>
          <Card className='text-center p-12'>
            <IconCheckCircle size='extra-large' style={{ fontSize: 80, color: 'var(--semi-color-success)' }} />
            <Title heading={3} className='mt-6 mb-4'>{t('支付成功')}</Title>
            <Text type='secondary' className='block mb-8'>
              {t('您的套餐已成功开通，可以开始使用了')}
            </Text>
            <Space spacing='medium'>
              <Button
                theme='solid'
                type='primary'
                onClick={() => navigate('/console/myplans')}
              >
                {t('查看我的套餐')}
              </Button>
              <Button
                onClick={() => navigate('/plans')}
              >
                {t('返回套餐列表')}
              </Button>
            </Space>
          </Card>
        </div>
      </div>
    );
  }

  if (order.status === 'expired') {
    return (
      <div className='min-h-screen bg-[var(--semi-color-bg-0)] py-12 px-4'>
        <div className='max-w-2xl mx-auto'>
          <Card className='text-center p-12'>
            <IconClock size='extra-large' style={{ fontSize: 80, color: 'var(--semi-color-warning)' }} />
            <Title heading={3} className='mt-6 mb-4'>{t('订单已过期')}</Title>
            <Text type='secondary' className='block mb-8'>
              {t('订单超过30分钟未支付，已自动取消')}
            </Text>
            <Button
              theme='solid'
              type='primary'
              onClick={() => navigate('/plans')}
            >
              {t('重新购买')}
            </Button>
          </Card>
        </div>
      </div>
    );
  }

  // Order confirmation UI (pending status)
  const discount = order.original_price > order.final_price
    ? order.original_price - order.final_price
    : 0;

  return (
    <div className='min-h-screen bg-[var(--semi-color-bg-0)] py-12 px-4'>
      <div className='max-w-2xl mx-auto'>
        {/* Countdown Banner */}
        <Banner
          type='warning'
          description={
            <div className='flex items-center justify-between'>
              <span>{t('订单剩余时间')}</span>
              <Text strong style={{ fontSize: 18 }}>
                {formatCountdown(countdown)}
              </Text>
            </div>
          }
          className='mb-6'
        />

        {/* Order Details Card */}
        <Card title={t('订单确认')} className='mb-6'>
          <div className='space-y-4'>
            {/* Order Number */}
            <div className='flex justify-between'>
              <Text type='secondary'>{t('订单号')}</Text>
              <Text strong>{order.order_no}</Text>
            </div>

            <Divider margin='12px' />

            {/* Plan Info */}
            <div className='flex justify-between'>
              <Text type='secondary'>{t('套餐')}</Text>
              <Text strong>{order.plan_name}</Text>
            </div>

            <Divider margin='12px' />

            {/* Price Info */}
            {discount > 0 && (
              <>
                <div className='flex justify-between'>
                  <Text type='secondary'>{t('原价')}</Text>
                  <Text delete type='tertiary'>${order.original_price.toFixed(2)}</Text>
                </div>
                <div className='flex justify-between'>
                  <Text type='secondary'>{t('优惠')}</Text>
                  <Text type='success'>-${discount.toFixed(2)}</Text>
                </div>
                <Divider margin='12px' />
              </>
            )}

            <div className='flex justify-between items-center'>
              <Text strong style={{ fontSize: 16 }}>{t('实付金额')}</Text>
              <Text
                strong
                style={{ fontSize: 24, color: 'var(--semi-color-primary)' }}
              >
                ${order.final_price.toFixed(2)}
              </Text>
            </div>
          </div>
        </Card>

        {/* Payment Method Card */}
        <Card title={t('支付方式')} className='mb-6'>
          <RadioGroup
            type='button'
            buttonSize='large'
            value={paymentMethod}
            onChange={(e) => setPaymentMethod(e.target.value)}
            style={{ width: '100%' }}
          >
            <Radio value='alipay' style={{ width: '48%', marginRight: '4%' }}>
              <div className='flex items-center gap-2'>
                <IconAlipayCircle size='extra-large' />
                <span>{t('支付宝')}</span>
              </div>
            </Radio>
            <Radio value='wechat' style={{ width: '48%' }}>
              <div className='flex items-center gap-2'>
                <IconWechatpay size='extra-large' />
                <span>{t('微信支付')}</span>
              </div>
            </Radio>
          </RadioGroup>
        </Card>

        {/* Action Buttons */}
        <div className='flex gap-4'>
          <Button
            block
            onClick={() => navigate('/plans')}
            disabled={paying}
          >
            {t('返回')}
          </Button>
          <Button
            block
            theme='solid'
            type='primary'
            size='large'
            loading={paying}
            onClick={handlePay}
            disabled={countdown <= 0}
          >
            {t('确认支付')}
          </Button>
        </div>

        {/* Notice */}
        <div className='mt-6'>
          <Text type='tertiary' size='small' className='block text-center'>
            {t('支付完成后，套餐将自动开通。如遇问题请联系客服。')}
          </Text>
        </div>
      </div>
    </div>
  );
};

export default OrderConfirm;
