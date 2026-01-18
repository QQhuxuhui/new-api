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
import { Card, Avatar, Skeleton, Tag, Progress, Button } from '@douyinfe/semi-ui';
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

  // Render subscription card
  const renderSubscriptionCard = () => {
    if (!subscriptionData || !subscriptionData.current_plan) return null;

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
        className="bg-gradient-to-br from-blue-50 to-indigo-50 border-0 !rounded-2xl w-full col-span-full lg:col-span-2"
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
              onClick={() => navigate('/console/plans')}
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

  return (
    <div className="mb-4">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Subscription Card - Full width on mobile, 2 cols on large screens */}
        {renderSubscriptionCard()}

        {/* Other Stats Cards */}
        {groupedStatsData.map((group, idx) => (
          <Card
            key={idx}
            {...CARD_PROPS}
            className={`${group.color} border-0 !rounded-2xl w-full hover:shadow-lg transition-all duration-200 cursor-pointer`}
            title={group.title}
          >
            <div className="space-y-4">
              {group.items.map((item, itemIdx) => (
                <div
                  key={itemIdx}
                  className="flex items-center justify-between"
                  onClick={item.onClick}
                >
                  <div className="flex items-center">
                    <Avatar
                      className="mr-3"
                      size="small"
                      color={item.avatarColor}
                    >
                      {item.icon}
                    </Avatar>
                    <div>
                      <div className="text-xs text-gray-500">{item.title}</div>
                      <div className="text-lg font-semibold">
                        <Skeleton
                          loading={loading}
                          active
                          placeholder={
                            <Skeleton.Paragraph
                              active
                              rows={1}
                              style={{
                                width: '65px',
                                height: '24px',
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
                  {item.title === t('当前余额') ? (
                    <Tag
                      color="white"
                      shape="circle"
                      size="large"
                      onClick={(e) => {
                        e.stopPropagation();
                        navigate('/console/topup');
                      }}
                    >
                      {t('充值')}
                    </Tag>
                  ) : (
                    (loading ||
                      (item.trendData && item.trendData.length > 0)) && (
                      <div className="w-24 h-10">
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
        ))}
      </div>
    </div>
  );
};

export default StatsCards;
