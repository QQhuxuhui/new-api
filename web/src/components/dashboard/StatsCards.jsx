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
import { Card, Avatar, Skeleton, Tag, Progress, Button, Banner } from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Calendar, TrendingUp } from 'lucide-react';

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
          <div className="flex flex-col items-center justify-center py-8 px-4">
            <div className="w-16 h-16 rounded-full bg-blue-100 flex items-center justify-center mb-4">
              <TrendingUp size={32} className="text-blue-600" />
            </div>
            <h3 className="text-lg font-semibold text-gray-900 mb-2">
              {t('暂无订阅套餐')}
            </h3>
            <p className="text-sm text-gray-500 mb-4 text-center">
              {t('选择适合您的套餐方案')}
            </p>
            <Button
              theme="solid"
              type="primary"
              size="large"
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
          <div className="flex items-center gap-2">
            <TrendingUp size={16} className="text-blue-600" />
            <span>{t('订阅套餐')}</span>
          </div>
        }
      >
        <div className="space-y-4">
          {/* Plan Name and Status */}
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-gray-500">{t('当前套餐')}</div>
              <div className="text-2xl font-bold text-gray-900 mt-1">
                {currentPlan.plan_display_name || currentPlan.plan_name}
              </div>
            </div>
            <Button
              theme="solid"
              type="primary"
              onClick={() => navigate('/plans')}
            >
              {t('续费')}
            </Button>
          </div>

          {/* Quota Progress */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm text-gray-600">{t('套餐额度使用')}</span>
              <span className="text-sm font-semibold text-gray-900">
                {quotaPercent}%
              </span>
            </div>
            <Progress
              percent={quotaPercent}
              stroke={quotaPercent > 80 ? '#f5222d' : '#1890ff'}
              showInfo={false}
              size="large"
            />
            <div className="flex items-center justify-between mt-1 text-xs text-gray-500">
              <span>{t('已使用')}: {usedQuota.toLocaleString()}</span>
              <span>{t('剩余')}: {remainingQuota.toLocaleString()}</span>
            </div>
          </div>

          {/* Daily Quota Progress (if exists) */}
          {dailyLimit > 0 && (
            <div>
              <div className="flex items-center justify-between mb-2">
                <span className="text-sm text-gray-600">{t('今日额度使用')}</span>
                <span className="text-sm font-semibold text-gray-900">
                  {dailyUsed.toLocaleString()} / {dailyLimit.toLocaleString()}
                </span>
              </div>
              <Progress
                percent={dailyPercent}
                stroke="#52c41a"
                showInfo={false}
              />
            </div>
          )}

          {/* Expiry Date */}
          <div className="flex items-center justify-between pt-2 border-t border-gray-200">
            <div className="flex items-center gap-2 text-sm text-gray-600">
              <Calendar size={14} />
              <span>{t('到期时间')}</span>
            </div>
            <div className="flex items-center gap-2">
              <span className={`text-sm font-medium ${isExpiringSoon ? 'text-red-600' : 'text-gray-900'}`}>
                {formatDate(currentPlan.expires_at)}
              </span>
              {daysRemaining !== null && (
                <Tag color={isExpiringSoon ? 'red' : 'blue'} size="small">
                  {daysRemaining > 0 ? `${daysRemaining}${t('天')}` : t('已过期')}
                </Tag>
              )}
            </div>
          </div>
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

  return (
    <div className="mb-4 space-y-4">
      {/* First Row: Subscription + Account Data */}
      <div className="grid gap-4 grid-cols-1 lg:grid-cols-2">
        {/* Subscription Card */}
        {renderSubscriptionCard()}

        {/* Account Data Card */}
        {accountData && (
          <Card
            {...CARD_PROPS}
            className={`${accountData.color} border-0 !rounded-2xl w-full hover:shadow-lg transition-all duration-200`}
            title={accountData.title}
          >
            <div className="grid grid-cols-1 gap-6 py-2">
              {accountData.items.map((item, itemIdx) => (
                <div
                  key={itemIdx}
                  className="flex flex-col items-center text-center cursor-pointer hover:bg-white/50 rounded-xl p-4 transition-all duration-200"
                  onClick={item.onClick}
                >
                  <Avatar
                    className="mb-3"
                    size="large"
                    color={item.avatarColor}
                  >
                    {item.icon}
                  </Avatar>
                  <div className="text-sm text-gray-500 mb-1">{item.title}</div>
                  <div className="text-2xl font-bold mb-3">
                    <Skeleton
                      loading={loading}
                      active
                      placeholder={
                        <Skeleton.Paragraph
                          active
                          rows={1}
                          style={{
                            width: '80px',
                            height: '32px',
                            marginTop: '4px',
                          }}
                        />
                      }
                    >
                      {item.value}
                    </Skeleton>
                  </div>
                  {item.title === t('当前余额') ? (
                    <Button
                      theme="solid"
                      type="primary"
                      size="large"
                      onClick={(e) => {
                        e.stopPropagation();
                        navigate('/console/topup');
                      }}
                    >
                      {t('充值')}
                    </Button>
                  ) : (
                    (loading ||
                      (item.trendData && item.trendData.length > 0)) && (
                      <div className="w-32 h-12 mt-2">
                        <VChart
                          spec={getTrendSpec(item.trendData, item.trendColor)}
                          option={CHART_CONFIG}
                        />
                      </div>
                    )
                  )}
                </div>
              ))}
            </div>
          </Card>
        )}
      </div>
    </div>
  );
};

export default StatsCards;
