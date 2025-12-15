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
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
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
  IconClock,
  IconTickCircle,
  IconCreditCard,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../helpers';

const { Title, Text } = Typography;

const OrderConfirm = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { orderId } = useParams();
  const [searchParams] = useSearchParams();
  const orderType = searchParams.get('type') || 'plan'; // 'plan' or 'topup'
  const [loading, setLoading] = useState(true);
  const [paying, setPaying] = useState(false);
  const [order, setOrder] = useState(null);
  const [paymentMethod, setPaymentMethod] = useState('');
  const [paymentMethods, setPaymentMethods] = useState([]);
  const [countdown, setCountdown] = useState(0);

  // Load payment methods configuration
  const loadPaymentMethods = async () => {
    try {
      const res = await API.get('/api/user/topup/info');
      const { success, data } = res.data;
      if (success && data && data.pay_methods && Array.isArray(data.pay_methods)) {
        // IMPORTANT: Epay SDK only accepts 'alipay' and 'wxpay', NOT 'wechat'
        // Filter and normalize payment methods:
        // 1. Only keep alipay and wxpay/wechat
        // 2. Convert 'wechat' to 'wxpay' for Epay compatibility
        // 3. Deduplicate if both wxpay and wechat exist
        const methodMap = new Map();

        data.pay_methods.forEach(method => {
          if (method.type === 'alipay') {
            methodMap.set('alipay', method);
          } else if (method.type === 'wxpay' || method.type === 'wechat') {
            // Normalize to wxpay (Epay only accepts 'wxpay')
            if (!methodMap.has('wxpay')) {
              methodMap.set('wxpay', {
                ...method,
                type: 'wxpay',
                name: method.name || '微信支付'
              });
            }
          }
        });

        const methods = Array.from(methodMap.values());

        if (methods.length > 0) {
          setPaymentMethods(methods);
          // Set first method as default
          setPaymentMethod(methods[0].type);
        } else {
          // No Epay standard payment methods configured
          setPaymentMethods([]);
          setPaymentMethod('');
          showError(t('暂无可用支付方式，请联系管理员配置支付宝或微信支付'));
        }
      } else {
        // Fallback to default methods
        setPaymentMethods([
          { name: '支付宝', type: 'alipay', color: 'rgba(var(--semi-blue-5), 1)' },
          { name: '微信支付', type: 'wxpay', color: 'rgba(var(--semi-green-5), 1)' },
        ]);
        setPaymentMethod('alipay');
      }
    } catch (e) {
      console.error('Failed to load payment methods:', e);
      // Fallback to default methods
      setPaymentMethods([
        { name: '支付宝', type: 'alipay', color: 'rgba(var(--semi-blue-5), 1)' },
        { name: '微信支付', type: 'wxpay', color: 'rgba(var(--semi-green-5), 1)' },
      ]);
      setPaymentMethod('alipay');
    }
  };

  // Load order details
  const loadOrder = async () => {
    setLoading(true);
    try {
      let targetOrder = null;

      if (orderType === 'topup') {
        // Load topup order directly by ID
        const res = await API.get(`/api/user/topup/order/${orderId}`);
        const { success, message, data } = res.data;
        if (success && data) {
          // Transform topup order to common format
          targetOrder = {
            order_id: data.order_id,
            order_no: data.order_no,
            order_type: 'topup',
            plan_name: `$${data.amount} ${t('额度')}`, // Display as amount
            amount: data.amount,
            quota: data.quota,
            original_price: data.original_price,
            final_price: data.final_price,
            discount_rate: data.discount_rate,
            status: data.status,
            created_at: data.created_at,
            expired_at: data.expired_at,
            paid_at: data.paid_at,
          };
        } else {
          showError(message || t('订单不存在'));
          navigate('/plans?category=payg');
          return;
        }
      } else {
        // Load plan order from list
        const res = await API.get('/api/user/plan/purchase/my-orders?page=1&page_size=100');
        const { success, message, data } = res.data;
        if (success && data && data.orders) {
          const foundOrder = data.orders.find(o => o.order_id === parseInt(orderId));
          if (foundOrder) {
            targetOrder = {
              ...foundOrder,
              order_type: 'plan',
            };
          } else {
            showError(t('订单不存在'));
            navigate('/plans');
            return;
          }
        } else {
          showError(message || t('加载订单失败'));
          navigate('/plans');
          return;
        }
      }

      if (targetOrder) {
        setOrder(targetOrder);
        // Calculate countdown
        const now = Date.now();
        const expiredAt = targetOrder.expired_at;
        const remaining = Math.max(0, Math.floor((expiredAt - now) / 1000));
        setCountdown(remaining);
      }
    } catch (e) {
      showError(e.message || t('网络错误'));
      navigate(orderType === 'topup' ? '/plans?category=payg' : '/plans');
    }
    setLoading(false);
  };

  useEffect(() => {
    if (orderId) {
      loadOrder();
      loadPaymentMethods();
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

    // Validate payment method is selected
    if (!paymentMethod) {
      showError(t('请选择支付方式'));
      return;
    }

    // Validate payment method is in available methods
    if (paymentMethods.length === 0) {
      showError(t('暂无可用支付方式'));
      return;
    }

    const isValidMethod = paymentMethods.some(m => m.type === paymentMethod);
    if (!isValidMethod) {
      showError(t('所选支付方式不可用'));
      return;
    }

    setPaying(true);
    let shouldResetPaying = true; // Control whether to reset paying state in finally
    try {
      // Use different API based on order type
      const payApiUrl = order.order_type === 'topup'
        ? '/api/user/topup/order/pay'
        : '/api/user/plan/purchase/pay';

      const res = await API.post(payApiUrl, {
        order_id: order.order_id,
        payment_method: paymentMethod,
      });
      const { success, message, data } = res.data;
      if (success && data) {
        // IMPORTANT: Must use form submission with params, not direct URL navigation
        // Epay SDK returns payment_url (action) and params (form fields)
        // Direct navigation to payment_url without params will fail with missing parameters error
        if (data.payment_url && data.params) {
          // Handle payment form submission (standard Epay flow)
          const form = document.createElement('form');
          form.method = 'POST';
          form.action = data.payment_url;
          // Check if Safari to handle target differently
          const isSafari =
            navigator.userAgent.indexOf('Safari') > -1 &&
            navigator.userAgent.indexOf('Chrome') < 1;
          if (!isSafari) {
            form.target = '_blank';
          }
          Object.keys(data.params).forEach(key => {
            const input = document.createElement('input');
            input.type = 'hidden';
            input.name = key;
            input.value = data.params[key];
            form.appendChild(input);
          });
          document.body.appendChild(form);
          form.submit();
          document.body.removeChild(form);

          if (!isSafari) {
            // For _blank submission, reset paying state to allow retry
            // User stays on current page, can close payment window and retry
            shouldResetPaying = true;
            // Show popup blocker warning
            Toast.info({
              content: t('paymentWindowOpened'),
              duration: 5,
            });
          } else {
            // For Safari same-page POST navigation, keep button disabled
            // Prevents double-click before page redirect
            shouldResetPaying = false;
          }
        } else if (data.payment_url) {
          // Fallback: direct URL navigation (for payment methods that support it)
          // This will navigate away from current page, so keep paying=true to prevent double-click
          shouldResetPaying = false;
          window.location.href = data.payment_url;
          // Note: finally block will execute before navigation, but shouldResetPaying=false prevents reset
        } else {
          showError(t('支付数据格式错误'));
        }
      } else {
        showError(message || t('支付失败'));
      }
    } catch (e) {
      showError(e.message || t('网络错误'));
    } finally {
      // Reset paying state to allow retry, but only if not doing whole page navigation
      // IMPORTANT: For _blank form submission, user stays on current page
      // If payment window is closed or blocked, user can click again
      // For whole page navigation, keep paying=true to prevent double-click before redirect
      if (shouldResetPaying) {
        setPaying(false);
      }
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
  // For topup orders, 'paid' status means success (no 'delivered' status)
  if (order.status === 'delivered' || (order.order_type === 'topup' && order.status === 'paid')) {
    const isTopup = order.order_type === 'topup';
    return (
      <div className='min-h-screen bg-[var(--semi-color-bg-0)] py-12 px-4'>
        <div className='max-w-2xl mx-auto'>
          <Card className='text-center p-12'>
            <IconTickCircle size='extra-large' style={{ fontSize: 80, color: 'var(--semi-color-success)' }} />
            <Title heading={3} className='mt-6 mb-4'>
              {isTopup ? t('充值成功') : t('支付成功')}
            </Title>
            <Text type='secondary' className='block mb-8'>
              {isTopup
                ? t('您的账户已成功充值，余额已到账')
                : t('您的套餐已成功开通，可以开始使用了')}
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
                onClick={() => navigate(isTopup ? '/plans?category=payg' : '/plans')}
              >
                {isTopup ? t('继续充值') : t('返回套餐列表')}
              </Button>
            </Space>
          </Card>
        </div>
      </div>
    );
  }

  if (order.status === 'expired') {
    const isTopup = order.order_type === 'topup';
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
              onClick={() => navigate(isTopup ? '/plans?category=payg' : '/plans')}
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
        <Card title={order.order_type === 'topup' ? t('充值订单确认') : t('订单确认')} className='mb-6'>
          <div className='space-y-4'>
            {/* Order Number */}
            <div className='flex justify-between'>
              <Text type='secondary'>{t('订单号')}</Text>
              <Text strong>{order.order_no}</Text>
            </div>

            <Divider margin='12px' />

            {/* Plan/Topup Info */}
            <div className='flex justify-between'>
              <Text type='secondary'>
                {order.order_type === 'topup' ? t('充值金额') : t('套餐')}
              </Text>
              <Text strong>{order.plan_name}</Text>
            </div>

            <Divider margin='12px' />

            {/* Price Info */}
            {discount > 0 && (
              <>
                <div className='flex justify-between'>
                  <Text type='secondary'>{t('原价')}</Text>
                  <Text delete type='tertiary'>¥{order.original_price.toFixed(2)}</Text>
                </div>
                <div className='flex justify-between'>
                  <Text type='secondary'>{t('优惠')}</Text>
                  <Text type='success'>-¥{discount.toFixed(2)}</Text>
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
                ¥{order.final_price.toFixed(2)}
              </Text>
            </div>
          </div>
        </Card>

        {/* Payment Method Card */}
        <Card title={t('支付方式')} className='mb-6'>
          {paymentMethods.length > 0 ? (
            <RadioGroup
              type='button'
              buttonSize='large'
              value={paymentMethod}
              onChange={(e) => setPaymentMethod(e.target.value)}
              style={{ width: '100%' }}
            >
              {paymentMethods.map((method, index) => (
                <Radio
                  key={method.type}
                  value={method.type}
                  style={{
                    width: paymentMethods.length === 1 ? '100%' : '48%',
                    marginRight: index % 2 === 0 ? '4%' : '0',
                    marginBottom: '8px'
                  }}
                >
                  <div className='flex items-center gap-2'>
                    <span style={{ fontSize: '20px' }}>
                      {method.type === 'alipay' ? '💳' : '💰'}
                    </span>
                    <span>{method.name || t(method.type)}</span>
                  </div>
                </Radio>
              ))}
            </RadioGroup>
          ) : (
            <Banner
              type='warning'
              description={t('暂无可用支付方式，请联系管理员配置')}
            />
          )}
        </Card>

        {/* Action Buttons */}
        <div className='flex gap-4'>
          <Button
            block
            onClick={() => navigate(order.order_type === 'topup' ? '/plans?category=payg' : '/plans')}
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
            disabled={countdown <= 0 || !paymentMethod || paymentMethods.length === 0}
          >
            {t('确认支付')}
          </Button>
        </div>

        {/* Notice */}
        <div className='mt-6'>
          <Text type='tertiary' size='small' className='block text-center'>
            {order.order_type === 'topup'
              ? t('支付完成后，余额将自动到账。如遇问题请联系客服。')
              : t('支付完成后，套餐将自动开通。如遇问题请联系客服。')}
          </Text>
        </div>
      </div>
    </div>
  );
};

export default OrderConfirm;
