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

import React, { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Typography,
  Tag,
  Button,
  Progress,
  Space,
  Empty,
  Spin,
  Switch,
  Popconfirm,
  Banner,
  Divider,
  Tooltip,
  Modal,
  List,
  Badge,
} from '@douyinfe/semi-ui';
import {
  IconTick,
  IconRefresh,
  IconClock,
  IconLock,
  IconAlertTriangle,
  IconBox,
  IconCalendarClock,
  IconBolt,
  IconCreditCard,
  IconArrowRight,
  IconShoppingBag,
  IconMoon,
  IconArrowUp,
  IconArrowDown,
  IconUndo,
  IconList,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess, renderQuota } from '../../helpers';

const { Title, Text, Paragraph } = Typography;

const MyPlans = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [userPlans, setUserPlans] = useState([]);
  const [quotaStatus, setQuotaStatus] = useState(null);
  const [billingStatus, setBillingStatus] = useState(null);
  const [showQueueModal, setShowQueueModal] = useState(false);
  const [showRefundModal, setShowRefundModal] = useState(false);
  const [refundPlan, setRefundPlan] = useState(null);
  const [refundReason, setRefundReason] = useState('');
  const [refundLoading, setRefundLoading] = useState(false);

  // Load user's plans
  const loadMyPlans = useCallback(async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/my_plans/');
      const { success, message, data } = res.data;
      if (success) {
        // data is UserPlanSummary object with { plans, current_plan, total_quota, total_used }
        setUserPlans(data?.plans || []);
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  }, []);

  // Load quota status for current plan
  const loadQuotaStatus = useCallback(async () => {
    try {
      const res = await API.get('/api/my_plans/quota-status');
      const { success, data } = res.data;
      if (success && data) {
        setQuotaStatus(data);
      }
    } catch (e) {
      // Silent fail - quota status is optional
      console.error('Failed to load quota status:', e);
    }
  }, []);

  // Load billing status (includes daily pool and queue)
  const loadBillingStatus = useCallback(async () => {
    try {
      const res = await API.get('/api/my_plans/billing-status');
      const { success, data } = res.data;
      if (success && data) {
        setBillingStatus(data);
      }
    } catch (e) {
      console.error('Failed to load billing status:', e);
    }
  }, []);

  useEffect(() => {
    loadMyPlans();
    loadQuotaStatus();
    loadBillingStatus();
  }, [loadMyPlans, loadQuotaStatus, loadBillingStatus]);

  // Switch to a different plan (uses user_plan_id, not plan_id)
  const handleSwitchPlan = async (userPlanId) => {
    setLoading(true);
    try {
      const res = await API.post('/api/my_plans/switch', { user_plan_id: userPlanId });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('套餐切换成功'));
        loadMyPlans();
        loadQuotaStatus();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  };

  // Toggle auto-switch
  const handleToggleAutoSwitch = async (userPlanId, enabled) => {
    setLoading(true);
    try {
      const res = await API.put(`/api/my_plans/${userPlanId}/auto_switch`, {
        enabled: enabled,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t(enabled ? '已开启自动切换' : '已关闭自动切换'));
        loadMyPlans();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  };

  // Request refund for a queued plan
  const handleRequestRefund = async () => {
    if (!refundPlan) return;
    setRefundLoading(true);
    try {
      const res = await API.post(`/api/my_plans/${refundPlan.id}/refund`, {
        reason: refundReason,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('退款申请已提交'));
        setShowRefundModal(false);
        setRefundPlan(null);
        setRefundReason('');
        loadMyPlans();
        loadBillingStatus();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setRefundLoading(false);
  };

  // Check if it's late night (after 22:00)
  const isLateNight = () => {
    const hour = new Date().getHours();
    return hour >= 22 || hour < 6;
  };

  // Render plan type tag
  const renderPlanType = (type) => {
    const typeConfig = {
      subscription: {
        color: 'blue',
        label: t('订阅套餐'),
        icon: <IconCalendarClock />,
      },
      consumption: {
        color: 'green',
        label: t('按量付费'),
        icon: <IconCreditCard />,
      },
      trial: {
        color: 'orange',
        label: t('试用套餐'),
        icon: <IconShoppingBag />,
      },
      enterprise: {
        color: 'purple',
        label: t('企业套餐'),
        icon: <IconBox />,
      },
    };
    const config = typeConfig[type] || {
      color: 'grey',
      label: type,
      icon: <IconBox />,
    };
    return (
      <Tag
        color={config.color}
        type='ghost'
        shape='circle'
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '4px',
          fontWeight: 600,
        }}
      >
        {config.icon}
        {config.label}
      </Tag>
    );
  };

  // Render quota progress
  const renderQuotaProgress = (userPlan) => {
    const used = parseInt(userPlan.used_quota) || 0;
    const remain = parseInt(userPlan.quota) || 0;
    const total = used + remain;
    const percent = total > 0 ? (remain / total) * 100 : 0;

    return (
      <div className='w-full p-4 bg-[var(--semi-color-fill-0)] rounded-xl'>
        <div className='flex justify-between items-center mb-2'>
          <div className='flex items-center gap-2'>
            <div className='p-1.5 rounded-lg bg-[var(--semi-color-primary-light-default)] text-[var(--semi-color-primary)]'>
              <IconCreditCard />
            </div>
            <Text strong size='normal'>
              {t('总额度使用情况')}
            </Text>
          </div>
          <Text strong className='text-[var(--semi-color-text-0)]'>
            {renderQuota(remain)} <span className='text-[var(--semi-color-text-2)] font-normal text-xs'>/ {renderQuota(total)}</span>
          </Text>
        </div>
        <Progress
          percent={percent}
          showInfo
          format={() => `${percent.toFixed(1)}%`}
          stroke={
            percent > 20
              ? 'var(--semi-color-success)'
              : 'var(--semi-color-danger)'
          }
          className='my-2'
          style={{ height: '8px' }}
        />
        <div className='flex justify-between mt-1'>
          <Text type='tertiary' size='small'>
            {t('已使用')}: {renderQuota(used)}
          </Text>
          <Text type={percent > 20 ? 'success' : 'danger'} size='small' strong>
            {t('剩余')} {percent.toFixed(1)}%
          </Text>
        </div>
      </div>
    );
  };

  // Render daily quota progress (if available)
  const renderDailyQuotaProgress = () => {
    if (!quotaStatus || quotaStatus.daily_quota_limit === 0) {
      return null;
    }

    const used = quotaStatus.daily_quota_used || 0;
    const total = quotaStatus.daily_quota_limit || 0;
    // 后端返回的字段是 daily_quota_remaining，备用计算防止负值
    const remain =
      quotaStatus.daily_quota_remaining ?? Math.max(total - used, 0);
    // 进度条显示已使用百分比
    const usedPercent = total > 0 ? (used / total) * 100 : 0;

    // Format reset time
    const resetTime = quotaStatus.daily_reset_time
      ? new Date(quotaStatus.daily_reset_time * 1000).toLocaleTimeString()
      : t('明日 00:00');

    return (
      <div className='mt-3 p-4 bg-gradient-to-br from-blue-50 to-indigo-50 dark:from-blue-900/20 dark:to-indigo-900/20 border border-blue-100 dark:border-blue-900/30 rounded-xl'>
        <div className='flex justify-between items-center mb-3'>
          <div className='flex items-center gap-2'>
            <div className='p-1.5 rounded-lg bg-blue-100 dark:bg-blue-800 text-blue-600 dark:text-blue-200'>
              <IconBolt />
            </div>
            <Text strong className='text-blue-900 dark:text-blue-100'>
              {t('每日限额')}
            </Text>
          </div>
          <Tag color='blue' size='small' type='solid'>
            <IconClock className='mr-1' />
            {t('重置时间')}: {resetTime}
          </Tag>
        </div>
        
        <div className='flex justify-between mb-1'>
          <Text className='text-blue-700 dark:text-blue-300' size='small'>
            {t('今日已用')} {renderQuota(used)}
          </Text>
          <Text strong className='text-blue-900 dark:text-blue-100'>
            {usedPercent.toFixed(1)}%
          </Text>
        </div>
        
        <Progress
          percent={usedPercent}
          showInfo={false}
          stroke={
            usedPercent > 80
              ? 'var(--semi-color-danger)'
              : 'var(--semi-color-primary)'
          }
          className='mb-2'
          style={{ height: '8px' }}
        />
        
        <div className='flex justify-end'>
          <Text className='text-blue-700 dark:text-blue-300' size='small'>
             {t('总限额')}: {renderQuota(total)}
          </Text>
        </div>
      </div>
    );
  };

  // Render rate limit status
  const renderRateLimitStatus = () => {
    if (!quotaStatus || !quotaStatus.rate_limited) {
      return null;
    }

    const waitSec = quotaStatus.rate_limit_wait_sec || 0;
    const waitMin = Math.ceil(waitSec / 60);
    const message = quotaStatus.rate_limit_message || t('速率限制：请稍后重试');

    return (
      <div className='mt-3'>
        <Banner
          type='warning'
          icon={<IconAlertTriangle />}
          description={
            <div>
              <Text strong>{message}</Text>
              <br />
              <Text type='secondary'>
                {t('预计等待时间')}: {waitMin} {t('分钟')} ({waitSec} {t('秒')})
              </Text>
            </div>
          }
          className='rounded-xl border-none shadow-sm'
        />
      </div>
    );
  };

  // Render daily pool status (from billing status)
  const renderDailyPoolCard = () => {
    if (!billingStatus || !billingStatus.daily_pool || billingStatus.daily_pool.total === 0) {
      return null;
    }

    const { available, total, used, expires_at } = billingStatus.daily_pool;
    const usedPercent = total > 0 ? (used / total) * 100 : 0;
    const remainPercent = 100 - usedPercent;

    return (
      <div className='bg-gradient-to-br from-purple-50 to-pink-50 dark:from-purple-900/20 dark:to-pink-900/20 border border-purple-100 dark:border-purple-900/30 rounded-2xl p-5 shadow-sm'>
        <div className='flex justify-between items-center mb-4'>
          <div className='flex items-center gap-3'>
            <div className='p-2 rounded-xl bg-purple-100 dark:bg-purple-800 text-purple-600 dark:text-purple-200'>
              <IconBolt size='large' />
            </div>
            <div>
              <Text strong className='text-purple-900 dark:text-purple-100 text-lg'>
                {t('今日日卡池')}
              </Text>
              <Text type='tertiary' size='small' className='block'>
                {t('有效期至')}: {expires_at}
              </Text>
            </div>
          </div>
          {isLateNight() && (
            <Tooltip content={t('当前为深夜时段，日卡将在明日凌晨重置，请合理安排使用')}>
              <Tag color='orange' type='solid' shape='circle'>
                <IconMoon className='mr-1' />
                {t('深夜提醒')}
              </Tag>
            </Tooltip>
          )}
        </div>

        <div className='grid grid-cols-3 gap-4 mb-4'>
          <div className='text-center p-3 bg-white/50 dark:bg-gray-800/50 rounded-xl'>
            <Text type='tertiary' size='small' className='block mb-1'>{t('总额度')}</Text>
            <Text strong className='text-lg text-purple-700 dark:text-purple-300'>{renderQuota(total)}</Text>
          </div>
          <div className='text-center p-3 bg-white/50 dark:bg-gray-800/50 rounded-xl'>
            <Text type='tertiary' size='small' className='block mb-1'>{t('已使用')}</Text>
            <Text strong className='text-lg text-orange-600 dark:text-orange-400'>{renderQuota(used)}</Text>
          </div>
          <div className='text-center p-3 bg-white/50 dark:bg-gray-800/50 rounded-xl'>
            <Text type='tertiary' size='small' className='block mb-1'>{t('剩余')}</Text>
            <Text strong className='text-lg text-green-600 dark:text-green-400'>{renderQuota(available)}</Text>
          </div>
        </div>

        <Progress
          percent={remainPercent}
          showInfo={false}
          stroke={remainPercent > 20 ? 'linear-gradient(90deg, #8b5cf6, #ec4899)' : 'var(--semi-color-danger)'}
          style={{ height: '10px' }}
        />
        <div className='flex justify-between mt-2'>
          <Text type='tertiary' size='small'>
            {t('使用进度')}: {usedPercent.toFixed(1)}%
          </Text>
          <Text strong size='small' className={remainPercent > 20 ? 'text-purple-600' : 'text-red-500'}>
            {t('剩余')} {remainPercent.toFixed(1)}%
          </Text>
        </div>
      </div>
    );
  };

  // Render queued plans
  const renderQueuedPlansSection = () => {
    if (!billingStatus || !billingStatus.queued_plans || billingStatus.queued_plans.length === 0) {
      return null;
    }

    const queuedPlans = billingStatus.queued_plans;

    return (
      <Card className='mt-6 rounded-2xl border-none shadow-sm'>
        <div className='flex justify-between items-center mb-4'>
          <div className='flex items-center gap-3'>
            <div className='p-2 rounded-xl bg-indigo-100 dark:bg-indigo-800 text-indigo-600 dark:text-indigo-200'>
              <IconList size='large' />
            </div>
            <div>
              <Title heading={5} className='m-0'>
                {t('排队中的套餐')}
              </Title>
              <Text type='tertiary' size='small'>
                {queuedPlans.length} {t('个套餐排队中')}
              </Text>
            </div>
          </div>
          <Button
            theme='light'
            type='tertiary'
            icon={<IconList />}
            onClick={() => setShowQueueModal(true)}
          >
            {t('查看队列')}
          </Button>
        </div>

        <div className='space-y-3'>
          {queuedPlans.slice(0, 3).map((plan, index) => (
            <div
              key={plan.id}
              className='flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-800/50 rounded-xl'
            >
              <div className='flex items-center gap-3'>
                <Badge count={plan.queue_position} type='primary' />
                <div>
                  <Text strong>{plan.plan_display_name || plan.plan_name || plan.name || plan.plan?.display_name || t('未知套餐')}</Text>
                  <Text type='tertiary' size='small' className='block'>
                    {t('额度')}: {renderQuota(plan.quota)}
                  </Text>
                </div>
              </div>
              {plan.is_refundable && (
                <Tooltip content={t('申请退款')}>
                  <Button
                    size='small'
                    type='warning'
                    theme='light'
                    icon={<IconUndo />}
                    onClick={() => {
                      setRefundPlan(plan);
                      setShowRefundModal(true);
                    }}
                  />
                </Tooltip>
              )}
            </div>
          ))}
          {queuedPlans.length > 3 && (
            <div className='text-center'>
              <Button theme='borderless' onClick={() => setShowQueueModal(true)}>
                {t('查看全部')} {queuedPlans.length} {t('个套餐')}
              </Button>
            </div>
          )}
        </div>
      </Card>
    );
  };

  // Render expiration info
  const renderExpiration = (userPlan) => {
    if (!userPlan.expires_at) {
      return (
        <Tag color='green' type='light' shape='circle'>
          {t('永久有效')}
        </Tag>
      );
    }

    const expiresAt = new Date(userPlan.expires_at);
    const now = new Date();
    const daysLeft = Math.ceil((expiresAt - now) / (1000 * 60 * 60 * 24));

    if (daysLeft <= 0) {
      return (
        <Tag color='red' type='solid' shape='circle'>
          {t('已过期')}
        </Tag>
      );
    } else if (daysLeft <= 7) {
      return (
        <Tag color='orange' type='solid' shape='circle'>
          {t('剩余')} {daysLeft} {t('天')}
        </Tag>
      );
    } else {
      return (
        <Tag color='blue' type='light' shape='circle'>
          {t('剩余')} {daysLeft} {t('天')}
        </Tag>
      );
    }
  };

  // Render a single plan card
  const renderPlanCard = (userPlan) => {
    const isCurrent = userPlan.is_current === 1;
    const isLocked = userPlan.locked === 1;
    const canSwitch = userPlan.can_switch === 1;
    const canToggleAuto = userPlan.can_toggle_auto === 1;
    const autoSwitchEnabled = userPlan.auto_switch === 1;
    const isQueued = (userPlan.queue_position || 0) > 0; // 在排队中
    const plan = userPlan.plan || {};

    return (
      <div
        key={userPlan.id}
        className={`group relative mb-6 transition-all duration-300 ease-in-out transform hover:-translate-y-1 ${isCurrent ? 'z-10' : 'z-0'}`}
      >
        {/* Glow effect for current plan */}
        {isCurrent && (
          <div className='absolute -inset-0.5 bg-gradient-to-r from-blue-400 to-indigo-500 rounded-2xl blur opacity-30 dark:opacity-40 transition-opacity duration-500'></div>
        )}
        
        <Card
          className={`relative h-full border-none shadow-sm hover:shadow-lg transition-shadow duration-300 ${isCurrent ? 'bg-white dark:bg-gray-800' : 'bg-gray-50 dark:bg-gray-800/50'}`}
          style={{
            borderRadius: '16px',
            opacity: isLocked ? 0.75 : 1,
            overflow: 'hidden',
          }}
          bodyStyle={{ padding: '24px' }}
        >
          {/* Header Section */}
          <div className='flex flex-col md:flex-row justify-between items-start md:items-center mb-6 gap-4'>
            <div className='flex items-start gap-4'>
              <div className={`p-3 rounded-2xl ${isCurrent ? 'bg-blue-600 text-white shadow-lg shadow-blue-500/30' : 'bg-gray-200 dark:bg-gray-700 text-gray-500 dark:text-gray-400'}`}>
                 <IconBox size="large" />
              </div>
              <div>
                <div className='flex items-center gap-2 mb-1'>
                  <Title heading={5} className='m-0 text-xl font-bold'>
                    {userPlan.plan_display_name || userPlan.plan_name || plan.display_name || plan.name || t('未知套餐')}
                  </Title>
                  {isCurrent && (
                    <Tag 
                      style={{ backgroundColor: 'rgba(var(--semi-blue-5), 1)', color: 'white', borderColor: 'transparent' }}
                      size='small'
                      shape='circle'
                      className='shadow-sm'
                    >
                      <IconTick size='small' className='mr-1' />
                      {t('当前使用')}
                    </Tag>
                  )}
                  {isLocked && (
                    <Tag color='red' size='small' shape='circle' type='solid'>
                      <IconLock size='small' className='mr-1' />
                      {t('已锁定')}
                    </Tag>
                  )}
                </div>
                <Space className='flex-wrap gap-y-2 mt-2'>
                  {renderPlanType(userPlan.plan_type || plan.type)}
                  {renderExpiration(userPlan)}
                  <Tag className='bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300' shape='circle' type="ghost">
                    {t('优先级')}: {userPlan.plan_priority ?? plan.priority ?? 0}
                  </Tag>
                </Space>
              </div>
            </div>

            {/* Switch Action (Top Right for Desktop) */}
            {!isCurrent && canSwitch && !isLocked && !isQueued && (
              <Popconfirm
                title={t('确认切换到此套餐？')}
                content={t('切换后将使用此套餐的额度和渠道配置')}
                onConfirm={() => handleSwitchPlan(userPlan.id)}
                okType="primary"
              >
                <Button
                  theme='light'
                  type='primary'
                  icon={<IconArrowRight />}
                  iconPosition="right"
                  className='hidden md:flex bg-blue-50 hover:bg-blue-100 text-blue-600 border-blue-200'
                  style={{ borderRadius: '12px' }}
                >
                  {t('切换到此套餐')}
                </Button>
              </Popconfirm>
            )}
          </div>

          <Divider className='mb-6 opacity-50' />

          {/* Quota Progress Sections */}
          <div className='grid grid-cols-1 gap-4'>
            {renderQuotaProgress(userPlan)}
            
            {/* Daily Quota & Rate Limit (Current Plan Only) */}
            {isCurrent && (
              <div className='animate-fade-in'>
                {renderDailyQuotaProgress()}
                {renderRateLimitStatus()}
              </div>
            )}
          </div>

          {/* Description */}
          {plan.description && (
            <div className='mt-5 p-4 bg-gray-50 dark:bg-gray-700/30 rounded-xl border border-dashed border-gray-200 dark:border-gray-700'>
              <Text type='secondary' className='text-sm leading-relaxed block'>
                {plan.description}
              </Text>
            </div>
          )}

          {/* Footer Actions */}
          <div className='flex flex-col sm:flex-row justify-between items-center mt-6 pt-2 gap-4'>
            <div className='w-full sm:w-auto'>
              {/* Auto-switch toggle */}
              {canToggleAuto && !isLocked ? (
                <div className='flex items-center justify-between sm:justify-start gap-3 p-3 bg-[var(--semi-color-bg-1)] rounded-xl border border-transparent hover:border-[var(--semi-color-border)] transition-colors'>
                  <div className='flex flex-col'>
                    <Text strong size='small'>
                      {t('自动切换')}
                    </Text>
                    <Text type='tertiary' size='small' className='text-xs'>
                      {autoSwitchEnabled ? t('余额不足时自动切换') : t('需手动切换套餐')}
                    </Text>
                  </div>
                  <Switch
                    checked={autoSwitchEnabled}
                    onChange={(checked) =>
                      handleToggleAutoSwitch(userPlan.id, checked)
                    }
                  />
                </div>
              ) : (
                <div className='flex items-center gap-2 p-2'>
                  <div className='w-2 h-2 rounded-full bg-gray-300'></div>
                  <Text type='tertiary' size='small'>
                    {t('自动切换由管理员控制')}
                  </Text>
                </div>
              )}
            </div>

            <div className='w-full sm:w-auto flex justify-end'>
               {/* Mobile Switch Button */}
              {!isCurrent && canSwitch && !isLocked && !isQueued && (
                <Popconfirm
                  title={t('确认切换到此套餐？')}
                  content={t('切换后将使用此套餐的额度和渠道配置')}
                  onConfirm={() => handleSwitchPlan(userPlan.id)}
                  okType="primary"
                >
                  <Button
                    theme='solid'
                    type='primary'
                    block
                    className='md:hidden w-full'
                    style={{ borderRadius: '12px', height: '40px' }}
                  >
                    {t('切换到此套餐')}
                  </Button>
                </Popconfirm>
              )}

              {!isCurrent && !canSwitch && !isLocked && !isQueued && (
                <Tooltip content={t('请联系管理员或满足特定条件后切换')}>
                  <Tag color='grey' style={{ borderRadius: '8px', padding: '6px 12px' }}>
                    {t('暂不可手动切换')}
                  </Tag>
                </Tooltip>
              )}

              {!isCurrent && isQueued && (
                <Tooltip content={t('排队中的套餐将在当前套餐耗尽或过期后自动激活，无法手动切换')}>
                  <Tag color='blue' style={{ borderRadius: '8px', padding: '6px 12px' }}>
                    <IconClock className='mr-1' />
                    {t('排队中')} #{userPlan.queue_position}
                  </Tag>
                </Tooltip>
              )}

              {isLocked && (
                <Tag color='red' style={{ borderRadius: '8px', padding: '6px 12px' }}>
                  {t('套餐锁定中')}
                </Tag>
              )}
            </div>
          </div>
        </Card>
      </div>
    );
  };

  // Find current plan
  const currentPlan = userPlans.find((p) => p.is_current === 1);

  return (
    <div className='min-h-screen bg-[var(--semi-color-bg-0)] pb-12'>
      {/* Page Header Background */}
      <div className='h-[200px] bg-gradient-to-br from-blue-600 to-indigo-700 relative overflow-hidden'>
        <div className='absolute inset-0 bg-[url("https://www.transparenttextures.com/patterns/cubes.png")] opacity-10'></div>
        <div className='absolute -bottom-10 -right-10 w-64 h-64 bg-white opacity-5 rounded-full blur-3xl'></div>
        <div className='absolute top-10 left-10 w-32 h-32 bg-purple-500 opacity-20 rounded-full blur-2xl'></div>
      </div>

      <div className='px-4 max-w-5xl mx-auto -mt-[120px] relative z-10'>
        {/* Header Content */}
        <div className='flex flex-col sm:flex-row justify-between items-start sm:items-end mb-8 text-white'>
          <div>
            <Title heading={2} className='m-0 text-white font-bold tracking-tight'>
              {t('我的套餐')}
            </Title>
            <Text className='text-blue-100 mt-2 block opacity-90 text-lg'>
              {t('管理您的订阅计划与额度使用详情')}
            </Text>
          </div>
          <Button
            icon={<IconRefresh />}
            theme='borderless'
            className='mt-4 sm:mt-0 text-white bg-white/20 hover:bg-white/30 border border-white/30 backdrop-blur-md'
            onClick={() => {
              loadMyPlans();
              loadQuotaStatus();
              loadBillingStatus();
            }}
            loading={loading}
            style={{ borderRadius: '12px', height: '40px', padding: '0 20px' }}
          >
            {t('刷新数据')}
          </Button>
        </div>

        {/* Current Plan Quick Stats (if exists) */}
        {currentPlan && (
          <div className='grid grid-cols-1 md:grid-cols-3 gap-4 mb-8'>
            <div className='bg-white/95 dark:bg-gray-800/95 backdrop-blur-xl p-5 rounded-2xl shadow-lg border border-white/20 flex flex-col justify-center'>
               <Text type="secondary" className="mb-1">{t('当前套餐')}</Text>
               <div className="flex items-center gap-2">
                 <div className="w-2 h-8 rounded-full bg-blue-500"></div>
                 <Title heading={4} className="m-0 truncate">{currentPlan.plan_display_name || currentPlan.plan_name || currentPlan.plan?.display_name || currentPlan.plan?.name || t('未知套餐')}</Title>
               </div>
            </div>
            <div className='bg-white/95 dark:bg-gray-800/95 backdrop-blur-xl p-5 rounded-2xl shadow-lg border border-white/20 flex flex-col justify-center'>
               <Text type="secondary" className="mb-1">{t('剩余总额度')}</Text>
               <div className="flex items-center gap-2">
                 <div className="w-2 h-8 rounded-full bg-green-500"></div>
                 <Title heading={4} className="m-0 truncate">{renderQuota(currentPlan.quota || 0)}</Title>
               </div>
            </div>
            <div className='bg-white/95 dark:bg-gray-800/95 backdrop-blur-xl p-5 rounded-2xl shadow-lg border border-white/20 flex flex-col justify-center'>
               <Text type="secondary" className="mb-1">{t('套餐状态')}</Text>
               <div className="flex items-center gap-2">
                 <div className="w-2 h-8 rounded-full bg-indigo-500"></div>
                 <div className="flex items-center">
                    <Text strong>{t('正常使用中')}</Text>
                    {currentPlan.auto_switch === 1 && <Tag size="small" className="ml-2" color="blue">{t('自动切换开启')}</Tag>}
                 </div>
               </div>
            </div>
          </div>
        )}

        {/* Daily Pool Card (if has daily pool) */}
        <div className='mb-8'>
          {renderDailyPoolCard()}
        </div>

        {/* Plans List */}
        <Spin spinning={loading} size="large" tip={t('加载套餐信息...')}>
          {userPlans.length > 0 ? (
            <div className='space-y-6'>
              {/* Current plan first */}
              {currentPlan && renderPlanCard(currentPlan)}

              {/* Separator if other plans exist */}
              {userPlans.length > 1 && (
                <div className="flex items-center my-8">
                  <div className="flex-grow h-px bg-gray-200 dark:bg-gray-700"></div>
                  <span className="px-4 text-gray-400 font-medium text-sm uppercase tracking-wider">{t('其他可用套餐')}</span>
                  <div className="flex-grow h-px bg-gray-200 dark:bg-gray-700"></div>
                </div>
              )}

              {/* Other plans */}
              <div className="grid grid-cols-1 gap-6">
                {userPlans
                  .filter((p) => p.is_current !== 1)
                  .map((userPlan) => renderPlanCard(userPlan))}
              </div>

              {/* Queued Plans Section */}
              {renderQueuedPlansSection()}
            </div>
          ) : (
            <div className='mt-12'>
              <Empty
                image={<IconBox size="extra-large" className="text-gray-300 text-6xl" />}
                title={t('暂无套餐')}
                description={t('您当前没有任何可用的套餐订阅，请联系管理员获取。')}
                className='bg-white dark:bg-gray-800 p-12 rounded-3xl shadow-sm'
              />
            </div>
          )}
        </Spin>
        
        {/* Footer info */}
        <div className="mt-12 text-center text-gray-400 text-sm pb-8">
           <p>{t('套餐额度仅供参考，具体扣费以实际使用量为准')}</p>
        </div>
      </div>

      {/* Queue Details Modal */}
      <Modal
        title={t('套餐队列详情')}
        visible={showQueueModal}
        onCancel={() => setShowQueueModal(false)}
        footer={
          <Button onClick={() => setShowQueueModal(false)}>
            {t('关闭')}
          </Button>
        }
        width={600}
      >
        <div className='mb-4'>
          <Banner
            type='info'
            description={t('套餐将按照队列顺序自动激活。当前套餐额度耗尽或过期后，下一个套餐将自动生效。')}
          />
        </div>
        {billingStatus?.queued_plans && billingStatus.queued_plans.length > 0 ? (
          <List
            dataSource={billingStatus.queued_plans}
            renderItem={(plan) => (
              <List.Item
                key={plan.id}
                main={
                  <div className='flex items-center justify-between w-full'>
                    <div className='flex items-center gap-3'>
                      <Badge count={plan.queue_position} type='primary' />
                      <div>
                        <Text strong>{plan.plan_display_name || plan.plan_name || plan.name || plan.plan?.display_name || t('未知套餐')}</Text>
                        <div className='flex items-center gap-2 mt-1'>
                          <Tag size='small' color='blue'>
                            {t('额度')}: {renderQuota(plan.quota)}
                          </Tag>
                          {plan.estimated_activation_time > 0 && (
                            <Tag size='small' color='grey'>
                              {t('预计激活')}: {new Date(plan.estimated_activation_time).toLocaleDateString()}
                            </Tag>
                          )}
                        </div>
                      </div>
                    </div>
                    {plan.is_refundable && (
                      <Button
                        size='small'
                        type='warning'
                        theme='light'
                        icon={<IconUndo />}
                        onClick={() => {
                          setRefundPlan(plan);
                          setShowRefundModal(true);
                          setShowQueueModal(false);
                        }}
                      >
                        {t('申请退款')}
                      </Button>
                    )}
                  </div>
                }
              />
            )}
          />
        ) : (
          <Empty description={t('暂无排队中的套餐')} />
        )}
      </Modal>

      {/* Refund Request Modal */}
      <Modal
        title={t('申请退款')}
        visible={showRefundModal}
        onCancel={() => {
          setShowRefundModal(false);
          setRefundPlan(null);
          setRefundReason('');
        }}
        onOk={handleRequestRefund}
        okText={t('提交申请')}
        cancelText={t('取消')}
        confirmLoading={refundLoading}
      >
        {refundPlan && (
          <div className='space-y-4'>
            <Banner
              type='warning'
              description={t('退款申请提交后需等待管理员审核。审核通过后，退款将原路返还。')}
            />
            <div className='p-4 bg-gray-50 dark:bg-gray-800 rounded-xl'>
              <Text type='secondary'>{t('套餐名称')}:</Text>
              <Text strong className='ml-2'>{refundPlan.name}</Text>
              <br />
              <Text type='secondary'>{t('剩余额度')}:</Text>
              <Text strong className='ml-2'>{renderQuota(refundPlan.quota)}</Text>
              <br />
              <Text type='secondary'>{t('队列位置')}:</Text>
              <Text strong className='ml-2'>#{refundPlan.queue_position}</Text>
            </div>
            <div>
              <Text className='block mb-2'>{t('退款原因')} ({t('可选')})</Text>
              <textarea
                className='w-full p-3 border border-gray-200 dark:border-gray-700 rounded-xl bg-white dark:bg-gray-800 resize-none'
                rows={3}
                placeholder={t('请输入退款原因...')}
                value={refundReason}
                onChange={(e) => setRefundReason(e.target.value)}
                maxLength={500}
              />
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
};

export default MyPlans;
