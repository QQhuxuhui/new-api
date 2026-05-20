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
import { Spin, Toast } from '@douyinfe/semi-ui';
import {
  ArrowLeft,
  Clock,
  CheckCircle2,
  AlertTriangle,
  Wallet,
  Package,
  ShieldCheck,
  Info,
} from 'lucide-react';
import { API, showError } from '../../helpers';
import UsdtPaymentModal from '../../components/topup/modals/UsdtPaymentModal';

const PAGE_BG_GRADIENT =
  'linear-gradient(180deg, var(--semi-color-bg-0) 0%, rgba(37, 99, 235, 0.04) 100%)';
const PRICE_GRADIENT = 'linear-gradient(135deg, #1D4ED8, #3B82F6)';
const CTA_GRADIENT = 'linear-gradient(135deg, #2563EB, #3B82F6)';
const CTA_GRADIENT_HOVER = 'linear-gradient(135deg, #1D4ED8, #2563EB)';
const POPPINS_STACK = "'Poppins', 'Inter', system-ui, sans-serif";
const MONO_STACK = "'SF Mono', Menlo, Monaco, Consolas, monospace";

const ALIPAY_BRAND = '#1677FF';
const WECHAT_BRAND = '#07C160';
const USDT_BRAND = '#26A17B';

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
  const [usdtOpen, setUsdtOpen] = useState(false);
  const [usdtData, setUsdtData] = useState(null);

  // Load payment methods configuration
  const loadPaymentMethods = async () => {
    try {
      const res = await API.get('/api/user/topup/info');
      const { success, data } = res.data;
      if (success && data && data.pay_methods && Array.isArray(data.pay_methods)) {
        // IMPORTANT: Epay SDK only accepts 'alipay' and 'wxpay', NOT 'wechat'.
        // USDT 走独立网关, 与 epay 并列展示。
        const methodMap = new Map();
        data.pay_methods.forEach((method) => {
          if (method.type === 'alipay') {
            methodMap.set('alipay', method);
          } else if (method.type === 'wxpay' || method.type === 'wechat') {
            if (!methodMap.has('wxpay')) {
              methodMap.set('wxpay', {
                ...method,
                type: 'wxpay',
                name: method.name || '微信支付',
              });
            }
          } else if (method.type === 'usdt') {
            methodMap.set('usdt', { ...method, name: method.name || 'USDT (TRC20)' });
          }
        });

        const methods = Array.from(methodMap.values());
        if (methods.length > 0) {
          setPaymentMethods(methods);
          setPaymentMethod(methods[0].type);
        } else {
          setPaymentMethods([]);
          setPaymentMethod('');
          showError(t('暂无可用支付方式，请联系管理员配置支付宝或微信支付'));
        }
      } else {
        setPaymentMethods([
          { name: '支付宝', type: 'alipay' },
          { name: '微信支付', type: 'wxpay' },
        ]);
        setPaymentMethod('alipay');
      }
    } catch (e) {
      console.error('Failed to load payment methods:', e);
      setPaymentMethods([
        { name: '支付宝', type: 'alipay' },
        { name: '微信支付', type: 'wxpay' },
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
        const res = await API.get(`/api/user/topup/order/${orderId}`);
        const { success, message, data } = res.data;
        if (success && data) {
          targetOrder = {
            order_id: data.order_id,
            order_no: data.order_no,
            order_type: 'topup',
            plan_name: `$${data.amount} ${t('额度')}`,
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
        const res = await API.get(`/api/user/plan/purchase/order/${orderId}`);
        const { success, message, data } = res.data;
        if (success && data) {
          targetOrder = { ...data, order_type: 'plan' };
        } else {
          showError(message || t('加载订单失败'));
          navigate('/plans');
          return;
        }
      }

      if (targetOrder) {
        setOrder(targetOrder);
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
  }, [orderId, orderType]);

  // Countdown timer
  useEffect(() => {
    if (countdown > 0) {
      const timer = setTimeout(() => setCountdown(countdown - 1), 1000);
      return () => clearTimeout(timer);
    } else if (countdown === 0 && order && order.status === 'pending') {
      Toast.warning(t('订单已过期'));
      const target =
        order.order_type === 'topup' ? '/plans?category=payg' : '/plans';
      const timer = setTimeout(() => navigate(target), 2000);
      return () => clearTimeout(timer);
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
    if (!paymentMethod) {
      showError(t('请选择支付方式'));
      return;
    }
    if (paymentMethods.length === 0) {
      showError(t('暂无可用支付方式'));
      return;
    }
    const isValidMethod = paymentMethods.some((m) => m.type === paymentMethod);
    if (!isValidMethod) {
      showError(t('所选支付方式不可用'));
      return;
    }

    setPaying(true);
    let shouldResetPaying = true;
    try {
      const payApiUrl =
        order.order_type === 'topup'
          ? '/api/user/topup/order/pay'
          : '/api/user/plan/purchase/pay';
      const res = await API.post(payApiUrl, {
        order_id: order.order_id,
        payment_method: paymentMethod,
      });
      const { success, message, data } = res.data;
      if (success && data) {
        // USDT 走独立网关: 拿到 token/actual_amount/payment_url 展示在弹窗
        if (paymentMethod === 'usdt' || data.payment_method === 'usdt') {
          setUsdtData(data);
          setUsdtOpen(true);
          shouldResetPaying = true;
        } else if (data.payment_url && data.params) {
          const form = document.createElement('form');
          form.method = 'POST';
          form.action = data.payment_url;
          const isSafari =
            navigator.userAgent.indexOf('Safari') > -1 &&
            navigator.userAgent.indexOf('Chrome') < 1;
          if (!isSafari) {
            form.target = '_blank';
          }
          Object.keys(data.params).forEach((key) => {
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
            shouldResetPaying = true;
            Toast.info({ content: t('paymentWindowOpened'), duration: 5 });
          } else {
            shouldResetPaying = false;
          }
        } else if (data.payment_url) {
          shouldResetPaying = false;
          window.location.href = data.payment_url;
        } else {
          showError(t('支付数据格式错误'));
        }
      } else {
        showError(message || t('支付失败'));
      }
    } catch (e) {
      showError(e.message || t('网络错误'));
    } finally {
      if (shouldResetPaying) setPaying(false);
    }
  };

  const formatCountdown = (seconds) => {
    const minutes = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
  };

  const formatDateTime = (ts) => {
    if (!ts) return '';
    const d = new Date(ts);
    if (Number.isNaN(d.getTime())) return '';
    const pad = (n) => n.toString().padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
  };

  // Split price into integer and decimal parts for visual hierarchy
  const splitPrice = (value) => {
    const num = Number(value || 0);
    const fixed = num.toFixed(2);
    const [intPart, decPart] = fixed.split('.');
    return { intPart, decPart };
  };

  if (loading) {
    return (
      <div
        className='flex items-center justify-center min-h-screen'
        style={{ background: PAGE_BG_GRADIENT }}
      >
        <Spin size='large' />
      </div>
    );
  }

  if (!order) return null;

  const isTopup = order.order_type === 'topup';

  // ========== Success state ==========
  // Plan order's `paid` means payment confirmed but provisioning still in progress.
  // Topup order has no `delivered`; `paid` IS the terminal success.
  if (order.status === 'delivered' || order.status === 'paid') {
    const planProvisioning = !isTopup && order.status === 'paid';
    const successTitle = isTopup
      ? t('充值成功')
      : planProvisioning
        ? t('支付成功，正在开通')
        : t('支付成功');
    const successDesc = isTopup
      ? t('您的账户已成功充值，余额已到账')
      : planProvisioning
        ? t('支付已确认，套餐正在开通中，请稍候片刻')
        : t('您的套餐已成功开通，可以开始使用了');
    return (
      <div
        className='min-h-screen px-4 py-12 sm:py-16'
        style={{ background: PAGE_BG_GRADIENT }}
      >
        <div className='max-w-md mx-auto'>
          <div
            className='rounded-3xl p-8 sm:p-10 text-center relative overflow-hidden'
            style={{
              background: 'var(--semi-color-bg-1)',
              border: '1px solid var(--semi-color-border)',
              boxShadow: '0 1px 3px rgba(15,23,42,0.04), 0 24px 48px -16px rgba(15,23,42,0.12)',
            }}
          >
            {/* Decorative aurora */}
            <div
              className='absolute -top-20 -right-20 w-64 h-64 rounded-full opacity-30 pointer-events-none'
              style={{
                background: 'radial-gradient(circle, #34D399 0%, transparent 70%)',
              }}
            />
            <div className='relative'>
              <div
                className='inline-flex items-center justify-center w-20 h-20 rounded-full mb-6'
                style={{
                  background: 'linear-gradient(135deg, #10B981, #34D399)',
                  boxShadow: '0 12px 28px -8px rgba(16,185,129,0.5)',
                }}
              >
                <CheckCircle2 size={44} strokeWidth={2.5} color='white' />
              </div>
              <h1
                className='text-2xl sm:text-3xl font-bold mb-2'
                style={{
                  color: 'var(--semi-color-text-0)',
                  fontFamily: POPPINS_STACK,
                  letterSpacing: '-0.5px',
                }}
              >
                {successTitle}
              </h1>
              <p
                className='text-sm mb-8'
                style={{ color: 'var(--semi-color-text-2)' }}
              >
                {successDesc}
              </p>

              {/* Mini summary */}
              <div
                className='rounded-2xl p-4 mb-8 text-left'
                style={{
                  background: 'var(--semi-color-fill-0)',
                  border: '1px solid var(--semi-color-border)',
                }}
              >
                <div className='flex justify-between items-center mb-2'>
                  <span
                    className='text-xs uppercase tracking-wider font-semibold'
                    style={{ color: 'var(--semi-color-text-2)' }}
                  >
                    {isTopup ? t('充值金额') : t('已开通套餐')}
                  </span>
                  <span
                    className='text-sm font-semibold'
                    style={{ color: 'var(--semi-color-text-0)' }}
                  >
                    {order.plan_name}
                  </span>
                </div>
                <div className='flex justify-between items-center'>
                  <span
                    className='text-xs'
                    style={{ color: 'var(--semi-color-text-2)' }}
                  >
                    {t('订单号')}
                  </span>
                  <span
                    className='text-xs'
                    style={{
                      color: 'var(--semi-color-text-1)',
                      fontFamily: MONO_STACK,
                    }}
                  >
                    {order.order_no}
                  </span>
                </div>
              </div>

              <div className='flex flex-col sm:flex-row gap-3'>
                <button
                  type='button'
                  onClick={() => navigate('/console/myplans')}
                  className='flex-1 h-12 rounded-xl font-semibold text-white transition-all'
                  style={{
                    background: CTA_GRADIENT,
                    boxShadow: '0 8px 20px -6px rgba(37,99,235,0.45)',
                    fontFamily: POPPINS_STACK,
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.background = CTA_GRADIENT_HOVER;
                    e.currentTarget.style.transform = 'translateY(-1px)';
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.background = CTA_GRADIENT;
                    e.currentTarget.style.transform = 'translateY(0)';
                  }}
                >
                  {t('查看我的套餐')}
                </button>
                <button
                  type='button'
                  onClick={() =>
                    navigate(isTopup ? '/plans?category=payg' : '/plans')
                  }
                  className='flex-1 h-12 rounded-xl font-medium transition-all'
                  style={{
                    background: 'transparent',
                    color: 'var(--semi-color-text-1)',
                    border: '1px solid var(--semi-color-border)',
                    fontFamily: POPPINS_STACK,
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.background = 'var(--semi-color-fill-0)';
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.background = 'transparent';
                  }}
                >
                  {isTopup ? t('继续充值') : t('返回套餐列表')}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // ========== Expired state ==========
  if (order.status === 'expired') {
    return (
      <div
        className='min-h-screen px-4 py-12 sm:py-16'
        style={{ background: PAGE_BG_GRADIENT }}
      >
        <div className='max-w-md mx-auto'>
          <div
            className='rounded-3xl p-8 sm:p-10 text-center'
            style={{
              background: 'var(--semi-color-bg-1)',
              border: '1px solid var(--semi-color-border)',
              boxShadow: '0 1px 3px rgba(15,23,42,0.04), 0 24px 48px -16px rgba(15,23,42,0.12)',
            }}
          >
            <div
              className='inline-flex items-center justify-center w-20 h-20 rounded-full mb-6'
              style={{
                background: 'linear-gradient(135deg, #F59E0B, #FBBF24)',
                boxShadow: '0 12px 28px -8px rgba(245,158,11,0.5)',
              }}
            >
              <Clock size={42} strokeWidth={2.5} color='white' />
            </div>
            <h1
              className='text-2xl sm:text-3xl font-bold mb-2'
              style={{
                color: 'var(--semi-color-text-0)',
                fontFamily: POPPINS_STACK,
                letterSpacing: '-0.5px',
              }}
            >
              {t('订单已过期')}
            </h1>
            <p
              className='text-sm mb-8'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              {t('订单超过30分钟未支付，已自动取消')}
            </p>
            <button
              type='button'
              onClick={() =>
                navigate(isTopup ? '/plans?category=payg' : '/plans')
              }
              className='h-12 px-8 rounded-xl font-semibold text-white transition-all'
              style={{
                background: CTA_GRADIENT,
                boxShadow: '0 8px 20px -6px rgba(37,99,235,0.45)',
                fontFamily: POPPINS_STACK,
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.background = CTA_GRADIENT_HOVER;
                e.currentTarget.style.transform = 'translateY(-1px)';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.background = CTA_GRADIENT;
                e.currentTarget.style.transform = 'translateY(0)';
              }}
            >
              {t('重新购买')}
            </button>
          </div>
        </div>
      </div>
    );
  }

  // ========== Pending state ==========
  const discount =
    order.original_price > order.final_price
      ? order.original_price - order.final_price
      : 0;
  const finalPrice = splitPrice(order.final_price);

  const orderTypeLabel = isTopup ? 'TOP-UP' : 'SUBSCRIPTION';
  const orderTypeColor = isTopup
    ? { from: '#FEF3C7', to: '#FDE68A', text: '#92400E' }
    : { from: '#DBEAFE', to: '#BFDBFE', text: '#1D4ED8' };

  const subtitle = isTopup
    ? t('完成支付后余额将自动到账')
    : t('完成支付后套餐立即生效');

  // Countdown chip color: warning when < 5 min
  const countdownLow = countdown > 0 && countdown < 300;
  const countdownChipBg = countdownLow
    ? 'linear-gradient(135deg, #FEE2E2, #FECACA)'
    : 'linear-gradient(135deg, #FEF3C7, #FDE68A)';
  const countdownChipText = countdownLow ? '#991B1B' : '#92400E';

  const renderPaymentTile = (method) => {
    const isAlipay = method.type === 'alipay';
    const isUsdt = method.type === 'usdt';
    const brandColor = isAlipay
      ? ALIPAY_BRAND
      : isUsdt
        ? USDT_BRAND
        : WECHAT_BRAND;
    const isActive = paymentMethod === method.type;
    const label =
      method.name ||
      (isAlipay ? t('支付宝') : isUsdt ? 'USDT (TRC20)' : t('微信支付'));
    const sub = isAlipay
      ? t('扫码 / 网页支付')
      : isUsdt
        ? t('TRC20 链上转账')
        : t('扫码支付');
    const initial = isAlipay ? '支' : isUsdt ? '₮' : '微';

    return (
      <button
        key={method.type}
        type='button'
        onClick={() => setPaymentMethod(method.type)}
        className='w-full text-left rounded-2xl p-4 flex items-center gap-3 transition-all'
        style={{
          background: isActive
            ? 'linear-gradient(180deg, rgba(37,99,235,0.06), transparent)'
            : 'var(--semi-color-bg-1)',
          border: isActive
            ? '1.5px solid #2563EB'
            : '1.5px solid var(--semi-color-border)',
          boxShadow: isActive
            ? '0 0 0 4px rgba(37,99,235,0.08)'
            : 'none',
          cursor: 'pointer',
        }}
        onMouseEnter={(e) => {
          if (!isActive) {
            e.currentTarget.style.borderColor = 'rgba(37,99,235,0.4)';
          }
        }}
        onMouseLeave={(e) => {
          if (!isActive) {
            e.currentTarget.style.borderColor = 'var(--semi-color-border)';
          }
        }}
      >
        <div
          className='flex items-center justify-center w-10 h-10 rounded-xl flex-shrink-0 text-white font-bold text-base'
          style={{
            background: brandColor,
            fontFamily: POPPINS_STACK,
            boxShadow: `0 4px 10px -3px ${brandColor}66`,
          }}
        >
          {initial}
        </div>
        <div className='flex-1 min-w-0'>
          <div
            className='font-semibold text-[15px] leading-tight'
            style={{ color: 'var(--semi-color-text-0)' }}
          >
            {label}
          </div>
          <div
            className='text-xs mt-0.5'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {sub}
          </div>
        </div>
        <span
          className='w-5 h-5 rounded-full relative flex-shrink-0 transition-all'
          style={{
            border: isActive ? '6px solid #2563EB' : '1.5px solid var(--semi-color-border)',
            background: isActive ? '#fff' : 'transparent',
          }}
        />
      </button>
    );
  };

  const expired = countdown <= 0;
  const canPay =
    order.status === 'pending' &&
    !expired &&
    paymentMethod &&
    paymentMethods.length > 0 &&
    !paying;

  return (
    <div
      className='min-h-screen px-4 py-8 sm:py-12 pb-32 lg:pb-12'
      style={{ background: PAGE_BG_GRADIENT }}
    >
      <div className='max-w-4xl mx-auto'>
        {/* Header bar */}
        <div className='flex items-center justify-between mb-6'>
          <button
            type='button'
            onClick={() =>
              navigate(isTopup ? '/plans?category=payg' : '/plans')
            }
            disabled={paying}
            className='inline-flex items-center gap-2 text-sm transition-colors'
            style={{
              color: 'var(--semi-color-text-2)',
              cursor: paying ? 'not-allowed' : 'pointer',
            }}
            onMouseEnter={(e) => {
              if (!paying)
                e.currentTarget.style.color = 'var(--semi-color-text-0)';
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.color = 'var(--semi-color-text-2)';
            }}
          >
            <ArrowLeft size={16} />
            <span>
              {isTopup ? t('返回充值') : t('返回套餐列表')}
            </span>
          </button>

          <span
            className='inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-semibold'
            style={{
              background: countdownChipBg,
              color: countdownChipText,
              fontVariantNumeric: 'tabular-nums',
            }}
          >
            <Clock size={13} strokeWidth={2.5} />
            <span>
              {expired ? t('已过期') : `${formatCountdown(countdown)} ${t('后过期')}`}
            </span>
          </span>
        </div>

        <div className='grid grid-cols-1 lg:grid-cols-[1.35fr_1fr] gap-5'>
          {/* ============ Left: Order Summary ============ */}
          <div
            className='rounded-2xl relative'
            style={{
              background: 'var(--semi-color-bg-1)',
              border: '1px solid var(--semi-color-border)',
              boxShadow:
                '0 1px 3px rgba(15,23,42,0.04), 0 16px 40px -16px rgba(15,23,42,0.10)',
            }}
          >
            <div className='p-6 sm:p-7'>
              {/* Title row */}
              <div className='flex items-start justify-between gap-3'>
                <div className='min-w-0 flex-1'>
                  <div
                    className='text-[11px] uppercase tracking-[1.2px] font-semibold mb-2'
                    style={{ color: 'var(--semi-color-text-2)' }}
                  >
                    {isTopup ? t('充值订单确认') : t('订单确认')}
                  </div>
                  <h2
                    className='text-2xl font-bold leading-tight truncate'
                    style={{
                      color: 'var(--semi-color-text-0)',
                      fontFamily: POPPINS_STACK,
                      letterSpacing: '-0.5px',
                    }}
                  >
                    {order.plan_name}
                  </h2>
                  <div
                    className='text-[13px] mt-1.5'
                    style={{ color: 'var(--semi-color-text-2)' }}
                  >
                    {subtitle}
                  </div>
                </div>
                <span
                  className='inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-[10px] font-bold tracking-wider flex-shrink-0'
                  style={{
                    background: `linear-gradient(135deg, ${orderTypeColor.from}, ${orderTypeColor.to})`,
                    color: orderTypeColor.text,
                  }}
                >
                  {isTopup ? <Wallet size={11} /> : <Package size={11} />}
                  {orderTypeLabel}
                </span>
              </div>

              {/* Receipt-style dashed divider */}
              <div
                className='my-5 border-t-2 border-dashed'
                style={{ borderColor: 'var(--semi-color-border)' }}
              />

              {/* Meta rows */}
              <div className='space-y-3'>
                <div className='flex justify-between items-center'>
                  <span
                    className='text-[13px]'
                    style={{ color: 'var(--semi-color-text-2)' }}
                  >
                    {t('订单号')}
                  </span>
                  <span
                    className='text-[13px]'
                    style={{
                      color: 'var(--semi-color-text-1)',
                      fontFamily: MONO_STACK,
                    }}
                  >
                    {order.order_no}
                  </span>
                </div>
                {order.created_at && (
                  <div className='flex justify-between items-center'>
                    <span
                      className='text-[13px]'
                      style={{ color: 'var(--semi-color-text-2)' }}
                    >
                      {t('创建时间')}
                    </span>
                    <span
                      className='text-[13px]'
                      style={{ color: 'var(--semi-color-text-1)' }}
                    >
                      {formatDateTime(order.created_at)}
                    </span>
                  </div>
                )}
              </div>

              {/* Soft divider */}
              <div
                className='my-5 h-px'
                style={{
                  background:
                    'linear-gradient(90deg, transparent, var(--semi-color-border), transparent)',
                }}
              />

              {/* Price breakdown */}
              {discount > 0 && (
                <div className='space-y-3 mb-2'>
                  <div className='flex justify-between items-center'>
                    <span
                      className='text-[13px]'
                      style={{ color: 'var(--semi-color-text-2)' }}
                    >
                      {t('原价')}
                    </span>
                    <span
                      className='text-[14px] line-through'
                      style={{ color: 'var(--semi-color-text-2)' }}
                    >
                      ¥{Number(order.original_price).toFixed(2)}
                    </span>
                  </div>
                  <div className='flex justify-between items-center'>
                    <span
                      className='text-[13px]'
                      style={{ color: 'var(--semi-color-text-2)' }}
                    >
                      {t('优惠')}
                    </span>
                    <span
                      className='text-[14px] font-semibold'
                      style={{ color: '#10B981' }}
                    >
                      − ¥{discount.toFixed(2)}
                    </span>
                  </div>
                </div>
              )}

              {discount > 0 && (
                <div
                  className='my-4 h-px'
                  style={{
                    background:
                      'linear-gradient(90deg, transparent, var(--semi-color-border), transparent)',
                  }}
                />
              )}

              {/* Final price */}
              <div className='flex items-end justify-between pt-2'>
                <span
                  className='text-[14px] font-medium'
                  style={{ color: 'var(--semi-color-text-1)' }}
                >
                  {t('实付金额')}
                </span>
                <span
                  className='font-bold leading-none'
                  style={{
                    fontFamily: POPPINS_STACK,
                    fontSize: 40,
                    letterSpacing: '-1.5px',
                    background: PRICE_GRADIENT,
                    WebkitBackgroundClip: 'text',
                    WebkitTextFillColor: 'transparent',
                    backgroundClip: 'text',
                    fontVariantNumeric: 'tabular-nums',
                  }}
                >
                  ¥{finalPrice.intPart}
                  <span style={{ fontSize: 22 }}>.{finalPrice.decPart}</span>
                </span>
              </div>
            </div>
          </div>

          {/* ============ Right: Payment Method ============ */}
          <div
            className='rounded-2xl flex flex-col'
            style={{
              background: 'var(--semi-color-bg-1)',
              border: '1px solid var(--semi-color-border)',
              boxShadow:
                '0 1px 3px rgba(15,23,42,0.04), 0 16px 40px -16px rgba(15,23,42,0.10)',
            }}
          >
            <div className='p-6 sm:p-7 flex flex-col flex-1'>
              <div
                className='text-[11px] uppercase tracking-[1.2px] font-semibold mb-4'
                style={{ color: 'var(--semi-color-text-2)' }}
              >
                {t('选择支付方式')}
              </div>

              {paymentMethods.length > 0 ? (
                <div className='flex flex-col gap-2.5'>
                  {paymentMethods.map(renderPaymentTile)}
                </div>
              ) : (
                <div
                  className='rounded-xl p-4 text-sm flex items-start gap-2'
                  style={{
                    background: 'rgba(245, 158, 11, 0.08)',
                    color: '#92400E',
                    border: '1px solid rgba(245, 158, 11, 0.2)',
                  }}
                >
                  <AlertTriangle size={16} className='flex-shrink-0 mt-0.5' />
                  <span>{t('暂无可用支付方式，请联系管理员配置')}</span>
                </div>
              )}

              <div className='flex-1' />

              {/* Desktop CTA — hidden on mobile (sticky bar takes over) */}
              <button
                type='button'
                onClick={handlePay}
                disabled={!canPay}
                className='hidden lg:flex items-center justify-center gap-2 h-12 rounded-xl font-semibold text-white mt-6 transition-all'
                style={{
                  background: canPay
                    ? CTA_GRADIENT
                    : 'var(--semi-color-disabled-bg)',
                  color: canPay ? '#fff' : 'var(--semi-color-disabled-text)',
                  boxShadow: canPay
                    ? '0 8px 20px -6px rgba(37,99,235,0.45)'
                    : 'none',
                  fontFamily: POPPINS_STACK,
                  letterSpacing: '0.3px',
                  cursor: canPay ? 'pointer' : 'not-allowed',
                }}
                onMouseEnter={(e) => {
                  if (canPay) {
                    e.currentTarget.style.background = CTA_GRADIENT_HOVER;
                    e.currentTarget.style.transform = 'translateY(-1px)';
                  }
                }}
                onMouseLeave={(e) => {
                  if (canPay) {
                    e.currentTarget.style.background = CTA_GRADIENT;
                    e.currentTarget.style.transform = 'translateY(0)';
                  }
                }}
              >
                {paying && (
                  <span
                    className='inline-block w-4 h-4 rounded-full border-2 border-white border-t-transparent animate-spin'
                  />
                )}
                <span>
                  {paying
                    ? t('正在跳转...')
                    : `${t('确认支付')} ¥${Number(order.final_price).toFixed(2)}`}
                </span>
              </button>

              {/* Trust footer */}
              <div
                className='flex items-center justify-center gap-1.5 mt-4 text-[11.5px]'
                style={{ color: 'var(--semi-color-text-2)' }}
              >
                <ShieldCheck size={13} />
                <span>{t('通过 Epay 安全支付 · 订单 30 分钟内有效')}</span>
              </div>
            </div>
          </div>
        </div>

        {/* ============ Notice block ============ */}
        <div
          className='mt-5 rounded-2xl p-4 sm:p-5 flex gap-3'
          style={{
            background: 'var(--semi-color-fill-0)',
            border: '1px solid var(--semi-color-border)',
          }}
        >
          <div
            className='flex items-center justify-center w-7 h-7 rounded-lg flex-shrink-0'
            style={{
              background: 'rgba(37, 99, 235, 0.10)',
              color: '#2563EB',
            }}
          >
            <Info size={15} strokeWidth={2.5} />
          </div>
          <div className='flex-1 min-w-0 text-[12.5px] leading-relaxed'>
            <div style={{ color: 'var(--semi-color-text-1)' }}>
              {isTopup
                ? t('支付完成后，余额将自动到账，无需手动确认。')
                : t('支付完成后，套餐将自动开通，可立即使用。')}
            </div>
            <div className='mt-1' style={{ color: 'var(--semi-color-text-2)' }}>
              {t(
                '如使用不满意，可随时联系管理员，按剩余额度（扣除支付平台手续费）退款。'
              )}
            </div>
          </div>
        </div>
      </div>

      {/* ============ Mobile sticky bottom bar ============ */}
      <div
        className='lg:hidden fixed bottom-0 left-0 right-0 px-4 py-3 z-40'
        style={{
          background: 'var(--semi-color-bg-1)',
          borderTop: '1px solid var(--semi-color-border)',
          boxShadow: '0 -8px 24px -8px rgba(15,23,42,0.10)',
          paddingBottom: 'calc(0.75rem + env(safe-area-inset-bottom))',
        }}
      >
        <div className='max-w-4xl mx-auto flex items-center gap-3'>
          <div className='flex-1 min-w-0'>
            <div
              className='text-[11px]'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              {t('实付金额')}
            </div>
            <div
              className='font-bold leading-tight'
              style={{
                fontFamily: POPPINS_STACK,
                fontSize: 22,
                background: PRICE_GRADIENT,
                WebkitBackgroundClip: 'text',
                WebkitTextFillColor: 'transparent',
                backgroundClip: 'text',
                fontVariantNumeric: 'tabular-nums',
                letterSpacing: '-0.5px',
              }}
            >
              ¥{Number(order.final_price).toFixed(2)}
            </div>
          </div>
          <button
            type='button'
            onClick={handlePay}
            disabled={!canPay}
            className='inline-flex items-center justify-center gap-2 h-12 px-7 rounded-xl font-semibold text-white transition-all'
            style={{
              background: canPay ? CTA_GRADIENT : 'var(--semi-color-disabled-bg)',
              color: canPay ? '#fff' : 'var(--semi-color-disabled-text)',
              boxShadow: canPay
                ? '0 8px 20px -6px rgba(37,99,235,0.45)'
                : 'none',
              fontFamily: POPPINS_STACK,
              cursor: canPay ? 'pointer' : 'not-allowed',
              minWidth: 140,
            }}
          >
            {paying && (
              <span
                className='inline-block w-4 h-4 rounded-full border-2 border-white border-t-transparent animate-spin'
              />
            )}
            <span>
              {paying ? t('正在跳转...') : t('确认支付')}
            </span>
          </button>
        </div>
      </div>

      <UsdtPaymentModal
        visible={usdtOpen}
        onClose={() => {
          setUsdtOpen(false);
          // 关闭后刷新订单状态, 以便用户看到 paid
          loadOrder();
        }}
        data={usdtData}
        t={t}
      />
    </div>
  );
};

export default OrderConfirm;
