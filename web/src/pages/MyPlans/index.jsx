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
} from '@douyinfe/semi-ui';
import {
  IconTick,
  IconRefresh,
  IconClock,
  IconLock,
  IconAlertTriangle,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess, renderQuota } from '../../helpers';

const { Title, Text, Paragraph } = Typography;

const MyPlans = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [userPlans, setUserPlans] = useState([]);
  const [quotaStatus, setQuotaStatus] = useState(null);

  // Load user's plans
  const loadMyPlans = useCallback(async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/my_plans/');
      const { success, message, data } = res.data;
      if (success) {
        setUserPlans(data || []);
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

  useEffect(() => {
    loadMyPlans();
    loadQuotaStatus();
  }, [loadMyPlans, loadQuotaStatus]);

  // Switch to a different plan
  const handleSwitchPlan = async (planId) => {
    setLoading(true);
    try {
      const res = await API.post('/api/my_plans/switch', { plan_id: planId });
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

  // Render plan type tag
  const renderPlanType = (type) => {
    const typeConfig = {
      subscription: { color: 'blue', label: t('订阅套餐') },
      consumption: { color: 'green', label: t('按量付费') },
      trial: { color: 'orange', label: t('试用套餐') },
      enterprise: { color: 'purple', label: t('企业套餐') },
    };
    const config = typeConfig[type] || { color: 'grey', label: type };
    return <Tag color={config.color}>{config.label}</Tag>;
  };

  // Render quota progress
  const renderQuotaProgress = (userPlan) => {
    const used = parseInt(userPlan.used_quota) || 0;
    const total = parseInt(userPlan.quota) || 0;
    const remain = total - used;
    const percent = total > 0 ? (remain / total) * 100 : 0;

    return (
      <div className="w-full">
        <div className="flex justify-between mb-1">
          <Text type="secondary" size="small">
            {t('总额度剩余')}
          </Text>
          <Text strong>
            {renderQuota(remain)} / {renderQuota(total)}
          </Text>
        </div>
        <Progress
          percent={percent}
          showInfo
          format={() => `${percent.toFixed(1)}%`}
          stroke={percent > 20 ? 'var(--semi-color-success)' : 'var(--semi-color-danger)'}
        />
        <div className="flex justify-between mt-1">
          <Text type="tertiary" size="small">
            {t('已使用')}: {renderQuota(used)}
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
    const remain = quotaStatus.daily_quota_remain || 0;
    const percent = total > 0 ? ((total - remain) / total) * 100 : 0;

    // Format reset time
    const resetTime = quotaStatus.daily_reset_time
      ? new Date(quotaStatus.daily_reset_time * 1000).toLocaleTimeString()
      : t('明日 00:00');

    return (
      <div className="mt-4 p-4 bg-blue-50 rounded-lg">
        <div className="flex justify-between items-center mb-2">
          <Text strong>{t('每日限额使用情况')}</Text>
          <Tag color="blue" size="small">
            <IconClock className="mr-1" />
            {t('重置时间')}: {resetTime}
          </Tag>
        </div>
        <div className="flex justify-between mb-1">
          <Text type="secondary" size="small">
            {t('今日剩余')}
          </Text>
          <Text strong>
            {renderQuota(remain)} / {renderQuota(total)}
          </Text>
        </div>
        <Progress
          percent={percent}
          showInfo
          format={() => `${percent.toFixed(1)}%`}
          stroke={percent > 80 ? 'var(--semi-color-danger)' : 'var(--semi-color-primary)'}
        />
        <div className="flex justify-between mt-1">
          <Text type="tertiary" size="small">
            {t('今日已用')}: {renderQuota(used)}
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
      <div className="mt-4">
        <Banner
          type="warning"
          icon={<IconAlertTriangle />}
          description={
            <div>
              <Text strong>{message}</Text>
              <br />
              <Text type="secondary">
                {t('预计等待时间')}: {waitMin} {t('分钟')} ({waitSec} {t('秒')})
              </Text>
            </div>
          }
        />
      </div>
    );
  };

  // Render expiration info
  const renderExpiration = (userPlan) => {
    if (!userPlan.expires_at) {
      return (
        <Tag color="green" size="small">
          {t('永久有效')}
        </Tag>
      );
    }

    const expiresAt = new Date(userPlan.expires_at);
    const now = new Date();
    const daysLeft = Math.ceil((expiresAt - now) / (1000 * 60 * 60 * 24));

    if (daysLeft <= 0) {
      return (
        <Tag color="red" size="small">
          {t('已过期')}
        </Tag>
      );
    } else if (daysLeft <= 7) {
      return (
        <Tag color="orange" size="small">
          {t('剩余')} {daysLeft} {t('天')}
        </Tag>
      );
    } else {
      return (
        <Tag color="blue" size="small">
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
    const plan = userPlan.plan || {};

    return (
      <Card
        key={userPlan.id}
        className={`mb-4 ${isCurrent ? 'ring-2 ring-blue-500' : ''}`}
        style={{
          borderRadius: '12px',
          opacity: isLocked ? 0.7 : 1,
        }}
      >
        {/* Header */}
        <div className="flex justify-between items-start mb-4">
          <div>
            <Space>
              <Title heading={5} className="m-0">
                {plan.display_name || plan.name || t('未知套餐')}
              </Title>
              {isCurrent && (
                <Tag color="blue" size="small">
                  <IconTick size="small" className="mr-1" />
                  {t('当前使用')}
                </Tag>
              )}
              {isLocked && (
                <Tag color="red" size="small">
                  <IconLock size="small" className="mr-1" />
                  {t('已锁定')}
                </Tag>
              )}
            </Space>
            <div className="mt-1">
              <Space>
                {renderPlanType(plan.type)}
                {renderExpiration(userPlan)}
                <Tag color="grey" size="small">
                  {t('优先级')}: {plan.priority || 0}
                </Tag>
              </Space>
            </div>
          </div>
        </div>

        {/* Total Quota Progress */}
        <div className="mb-2">
          {renderQuotaProgress(userPlan)}
        </div>

        {/* Daily Quota Progress (current plan only) */}
        {isCurrent && renderDailyQuotaProgress()}

        {/* Rate Limit Status (current plan only) */}
        {isCurrent && renderRateLimitStatus()}

        {/* Description */}
        {plan.description && (
          <div className="mt-4">
            <Paragraph type="tertiary" className="mb-0">
              {plan.description}
            </Paragraph>
          </div>
        )}

        {/* Actions */}
        <div className="flex justify-between items-center pt-4 mt-4 border-t border-gray-100">
          <div className="flex items-center gap-4">
            {/* Auto-switch toggle */}
            {canToggleAuto && !isLocked && (
              <div className="flex items-center gap-2">
                <Text type="secondary" size="small">
                  {t('自动切换')}
                </Text>
                <Switch
                  checked={autoSwitchEnabled}
                  onChange={(checked) => handleToggleAutoSwitch(userPlan.id, checked)}
                  size="small"
                />
              </div>
            )}
            {!canToggleAuto && (
              <Text type="tertiary" size="small">
                {t('自动切换由管理员控制')}
              </Text>
            )}
          </div>

          <Space>
            {!isCurrent && canSwitch && !isLocked && (
              <Popconfirm
                title={t('确认切换到此套餐？')}
                content={t('切换后将使用此套餐的额度和渠道配置')}
                onConfirm={() => handleSwitchPlan(userPlan.plan_id)}
              >
                <Button theme="solid" type="primary" size="small">
                  {t('切换到此套餐')}
                </Button>
              </Popconfirm>
            )}
            {!isCurrent && !canSwitch && !isLocked && (
              <Text type="tertiary" size="small">
                {t('不允许手动切换')}
              </Text>
            )}
            {isLocked && (
              <Text type="danger" size="small">
                {t('套餐已被管理员锁定')}
              </Text>
            )}
          </Space>
        </div>
      </Card>
    );
  };

  // Find current plan
  const currentPlan = userPlans.find((p) => p.is_current === 1);

  return (
    <div className="p-4 max-w-4xl mx-auto">
      {/* Header */}
      <div className="flex justify-between items-center mb-6">
        <div>
          <Title heading={3} className="m-0">
            {t('我的套餐')}
          </Title>
          <Text type="secondary">
            {t('管理您的套餐订阅和使用情况')}
          </Text>
        </div>
        <Button
          icon={<IconRefresh />}
          onClick={() => {
            loadMyPlans();
            loadQuotaStatus();
          }}
          loading={loading}
        >
          {t('刷新')}
        </Button>
      </div>

      {/* Current Plan Summary */}
      {currentPlan && (
        <Banner
          type="info"
          className="mb-6"
          description={
            <span>
              {t('当前使用套餐')}: <strong>{currentPlan.plan?.display_name || currentPlan.plan?.name}</strong>
              {' - '}
              {t('剩余额度')}: <strong>{renderQuota((currentPlan.quota || 0) - (currentPlan.used_quota || 0))}</strong>
            </span>
          }
        />
      )}

      {/* Plans List */}
      <Spin spinning={loading}>
        {userPlans.length > 0 ? (
          <div>
            {/* Current plan first */}
            {currentPlan && renderPlanCard(currentPlan)}

            {/* Other plans */}
            {userPlans
              .filter((p) => p.is_current !== 1)
              .map((userPlan) => renderPlanCard(userPlan))}
          </div>
        ) : (
          <Card>
            <Empty
              description={
                <div>
                  <Text>{t('您还没有任何套餐')}</Text>
                  <br />
                  <Text type="tertiary">
                    {t('请联系管理员为您分配套餐')}
                  </Text>
                </div>
              }
            />
          </Card>
        )}
      </Spin>
    </div>
  );
};

export default MyPlans;
