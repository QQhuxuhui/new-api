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

import React, { useEffect, useState } from 'react';
import { Card, Avatar, Skeleton, Tag, Progress, Button, Banner } from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { TrendingUp } from 'lucide-react';
import { API } from '../../helpers';
import AffiliateRewardCard from './AffiliateRewardCard';

const StatsCards = ({
  groupedStatsData,
  loading,
  getTrendSpec,
  CARD_PROPS,
  CHART_CONFIG,
  subscriptionData,
  subscriptionLoading,
  subscriptionError,
  quotaStatus,
}) => {
  const navigate = useNavigate();
  const { t } = useTranslation();

  // Affiliate reward summary (for the third dashboard slot)
  // NOTE: only fetch the read-only summary here. The aff code endpoint
  // /api/user/aff has a write side-effect (auto-generates a code on first call),
  // so it is lazily fetched inside AffiliateRewardCard when the user clicks copy.
  const [affSummary, setAffSummary] = useState(null);
  const [affLoading, setAffLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    API.get('/api/user/aff/summary')
      .then((res) => {
        if (cancelled) return;
        if (res?.data?.success) {
          setAffSummary(res.data.data);
        }
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setAffLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const showAffSlot =
    affLoading || (affSummary && (affSummary.reward_percent ?? 0) > 0);

  // Helper function to format date
  const formatDate = (timestamp) => {
    if (!timestamp || timestamp === 0) return t('永久有效');
    const date = new Date(timestamp);
    return date.toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit'
    });
  };

  // Helper function to calculate days remaining
  const getDaysRemaining = (expiresAt) => {
    if (!expiresAt || expiresAt === 0) return null;
    const now = Date.now();
    const diff = expiresAt - now;
    const days = Math.ceil(diff / (1000 * 60 * 60 * 24));
    return days;
  };

  // Helper function to convert quota to USD (using same logic as renderQuota)
  const formatQuotaAsUSD = (quota) => {
    if (!quota || quota === 0) return '$0.00';

    let quotaPerUnit = localStorage.getItem('quota_per_unit');
    const quotaDisplayType = localStorage.getItem('quota_display_type') || 'USD';
    quotaPerUnit = parseFloat(quotaPerUnit);

    // If quotaPerUnit is invalid, show loading indicator
    if (!Number.isFinite(quotaPerUnit) || quotaPerUnit <= 0) {
      return '$ ...';
    }

    // Calculate USD amount using the same formula as renderQuota
    const resultUSD = quota / quotaPerUnit;

    // Apply currency conversion if needed
    let symbol = '$';
    let value = resultUSD;

    if (quotaDisplayType === 'CNY') {
      const statusStr = localStorage.getItem('status');
      let usdRate = 1;
      try {
        if (statusStr) {
          const s = JSON.parse(statusStr);
          usdRate = s?.usd_exchange_rate || 1;
        }
      } catch (e) {}
      value = resultUSD * usdRate;
      symbol = '¥';
    } else if (quotaDisplayType === 'CUSTOM') {
      const statusStr = localStorage.getItem('status');
      let symbolCustom = '¤';
      let rate = 1;
      try {
        if (statusStr) {
          const s = JSON.parse(statusStr);
          symbolCustom = s?.custom_currency_symbol || symbolCustom;
          rate = s?.custom_currency_exchange_rate || rate;
        }
      } catch (e) {}
      value = resultUSD * rate;
      symbol = symbolCustom;
    }

    const fixedResult = value.toFixed(2);
    if (parseFloat(fixedResult) === 0 && quota > 0 && value > 0) {
      const minValue = Math.pow(10, -2);
      return symbol + minValue.toFixed(2);
    }

    return symbol + fixedResult;
  };

  // Render subscription card or empty state
  const renderSubscriptionCard = () => {
    if (subscriptionLoading) {
      return (
        <Card
          {...CARD_PROPS}
          className="bg-gradient-to-br from-blue-50 to-indigo-50 border-0 !rounded-2xl w-full hover:shadow-lg transition-all duration-200"
        >
          <div className="space-y-4">
            <Skeleton.Title style={{ width: 120, height: 16 }} />
            <Skeleton.Title style={{ width: '60%', height: 24 }} />
            <Skeleton.Paragraph rows={2} />
            <Skeleton.Paragraph rows={2} />
          </div>
        </Card>
      );
    }

    if (subscriptionError) {
      return (
        <Card
          {...CARD_PROPS}
          className="bg-gradient-to-br from-blue-50 to-indigo-50 border-0 !rounded-2xl w-full hover:shadow-lg transition-all duration-200"
        >
          <Banner
            type="danger"
            closeIcon={null}
            title={t('订阅信息加载失败')}
            description={subscriptionError || t('请稍后重试')}
          />
        </Card>
      );
    }

    // Empty state when no subscription
    if (!subscriptionData || !subscriptionData.current_plan) {
      return (
        <Card
          {...CARD_PROPS}
          className="bg-gradient-to-br from-blue-50 to-indigo-50 border-0 !rounded-2xl w-full hover:shadow-lg transition-all duration-200"
        >
          <div className="flex flex-col items-center justify-center py-6 px-4">
            <div className="w-12 h-12 rounded-full bg-blue-100 flex items-center justify-center mb-3">
              <TrendingUp size={24} className="text-blue-600" />
            </div>
            <h3 className="text-base font-semibold text-gray-900 mb-1">
              {t('暂无订阅套餐')}
            </h3>
            <p className="text-xs text-gray-500 mb-3 text-center">
              {t('选择适合您的套餐方案')}
            </p>
            <Button
              theme="solid"
              type="primary"
              size="default"
              onClick={() => navigate('/plans')}
            >
              {t('获取订阅')}
            </Button>
          </div>
        </Card>
      );
    }

    // Existing subscription card
    const currentPlan = subscriptionData.current_plan;
    const usedQuota = currentPlan.used_quota || 0;
    const remainingQuota = currentPlan.quota || 0;
    const totalQuota = usedQuota + remainingQuota;
    const quotaPercent = totalQuota > 0
      ? Math.round((usedQuota / totalQuota) * 100)
      : 0;

    const dailyLimit = quotaStatus?.daily_quota_limit ?? currentPlan.effective_daily_limit ?? 0;
    const dailyUsed = quotaStatus?.daily_quota_used ?? currentPlan.daily_used ?? 0;
    const dailyPercent = dailyLimit > 0
      ? Math.min(100, Math.round((dailyUsed / dailyLimit) * 100))
      : 0;

    const daysRemaining = getDaysRemaining(currentPlan.expires_at);
    const isExpiringSoon = daysRemaining !== null && daysRemaining <= 7;

    return (
      <Card
        {...CARD_PROPS}
        className="bg-gradient-to-br from-blue-50 to-indigo-50 border-0 !rounded-2xl w-full hover:shadow-lg transition-all duration-200"
        title={
          <div className="flex items-center justify-between w-full">
            <div className="flex items-center gap-2">
              <TrendingUp size={16} className="text-blue-600" />
              <span>{t('订阅套餐')}</span>
            </div>
            <Button
              theme="solid"
              type="primary"
              onClick={() => navigate('/plans')}
            >
              {t('获取订阅')}
            </Button>
          </div>
        }
      >
        <div className="space-y-3">
          {/* Plan Name & Expiry - Compact Header */}
          <div className="flex items-center justify-between">
            <div>
              <div className="text-xs text-gray-500">{t('当前套餐')}</div>
              <div className="text-lg font-bold text-gray-900 mt-0.5">
                {currentPlan.plan_display_name || currentPlan.plan_name}
              </div>
            </div>
            <div className="flex items-center gap-2">
              <div className="text-right">
                <div className="text-xs text-gray-500">{t('到期')}</div>
                <div className={`text-xs font-medium mt-0.5 ${isExpiringSoon ? 'text-red-600' : 'text-gray-900'}`}>
                  {formatDate(currentPlan.expires_at)}
                </div>
              </div>
              {daysRemaining !== null && (
                <Tag color={isExpiringSoon ? 'red' : 'blue'} size="small">
                  {daysRemaining > 0 ? `${daysRemaining}${t('天')}` : t('已过期')}
                </Tag>
              )}
            </div>
          </div>

          {/* Quota Progress - Inline Label */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <span className="text-xs text-gray-600">{t('套餐额度')}</span>
              <span className="text-xs font-medium text-gray-900">
                {t('已用')} {formatQuotaAsUSD(usedQuota)} / {t('剩余')} {formatQuotaAsUSD(remainingQuota)}
              </span>
            </div>
            <Progress
              percent={quotaPercent}
              stroke={quotaPercent > 80 ? '#f5222d' : '#1890ff'}
              showInfo={false}
            />
          </div>

          {/* Daily Quota Progress - Inline Label (if exists) */}
          {dailyLimit > 0 && (
            <div>
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-xs text-gray-600">{t('今日额度')}</span>
                <span className="text-xs font-medium text-gray-900">
                  {formatQuotaAsUSD(dailyUsed)} / {formatQuotaAsUSD(dailyLimit)}
                </span>
              </div>
              <Progress
                percent={dailyPercent}
                stroke="#52c41a"
                showInfo={false}
              />
            </div>
          )}
        </div>
      </Card>
    );
  };

  // Separate account data from other stats (defensive guards)
  const safeGroupedStatsData = Array.isArray(groupedStatsData) ? groupedStatsData : [];
  const accountData = safeGroupedStatsData[0]; // 账户数据
  const otherStats = safeGroupedStatsData.slice(1); // 使用统计、资源消耗、性能指标

  // Flatten all metrics into a single array for horizontal display
  const allMetrics = otherStats.flatMap(group =>
    group.items.map(item => ({
      ...item,
      groupTitle: group.title
    }))
  );

  const mainSpanClass = showAffSlot ? 'lg:col-span-2' : '';
  const gridColsClass = showAffSlot ? 'lg:grid-cols-5' : 'lg:grid-cols-2';

  return (
    <div className="mb-4 space-y-4">
      {/* First Row: Subscription + Account Data (+ Affiliate when enabled) */}
      <div className={`grid gap-4 grid-cols-1 ${gridColsClass}`}>
        {/* Subscription Card */}
        <div className={mainSpanClass}>{renderSubscriptionCard()}</div>

        {/* Account Data Card */}
        {accountData && (
          <div className={mainSpanClass}>
            <Card
              {...CARD_PROPS}
              className={`${accountData.color} border-0 !rounded-2xl w-full hover:shadow-lg transition-all duration-200`}
              title={
                <div className="flex items-center justify-between w-full">
                  {accountData.title}
                  <Button
                    theme="solid"
                    type="primary"
                    onClick={() => navigate('/console/topup')}
                  >
                    {t('充值')}
                  </Button>
                </div>
              }
            >
              <div className="grid grid-cols-2 gap-4">
                {accountData.items.map((item, itemIdx) => (
                  <div
                    key={itemIdx}
                    className="flex items-center gap-3 cursor-pointer hover:bg-white/50 rounded-xl p-3 transition-all duration-200"
                    onClick={item.onClick}
                  >
                    <Avatar
                      size="default"
                      color={item.avatarColor}
                    >
                      {item.icon}
                    </Avatar>
                    <div className="flex-1">
                      <div className="text-xs text-gray-500">{item.title}</div>
                      <div className="text-xl font-bold">
                        <Skeleton
                          loading={loading}
                          active
                          placeholder={
                            <Skeleton.Paragraph
                              active
                              rows={1}
                              style={{
                                width: '80px',
                                height: '28px',
                                marginTop: '4px',
                              }}
                            />
                          }
                        >
                          {item.value}
                        </Skeleton>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </Card>
          </div>
        )}

        {/* Affiliate Reward Card (3rd slot) */}
        {showAffSlot && (
          <div className="lg:col-span-1">
            <AffiliateRewardCard
              summary={affSummary}
              loading={affLoading}
            />
          </div>
        )}
      </div>
    </div>
  );
};

export default StatsCards;
