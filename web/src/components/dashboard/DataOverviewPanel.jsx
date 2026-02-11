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
import { Card, Skeleton } from '@douyinfe/semi-ui';
import {
  IconCoinMoneyStroked,
  IconTextStroked,
  IconSend,
  IconPulse,
} from '@douyinfe/semi-icons';
import { Wallet } from 'lucide-react';
import { renderQuota, renderNumber } from '../../helpers';
import ScrollableContainer from '../common/ui/ScrollableContainer';

const DataOverviewPanel = ({
  consumeQuota,
  consumeTokens,
  times,
  todayConsumeQuota,
  todayConsumeTokens,
  todayTimes,
  loading,
  todayLoading,
  CARD_PROPS,
  FLEX_CENTER_GAP2,
  t,
}) => {
  const sections = [
    {
      label: t('统计范围消耗'),
      color: 'purple',
      icon: <IconCoinMoneyStroked style={{ fontSize: 14 }} />,
      iconBg: 'bg-purple-100',
      valueColor: 'text-purple-600',
      metrics: [
        {
          label: t('总 Tokens'),
          value: isNaN(consumeTokens) ? 0 : renderNumber(consumeTokens),
          sub: consumeTokens >= 10000
            ? `≈ ${(consumeTokens / 10000).toFixed(1)}${t('万')}`
            : null,
          loading,
        },
        {
          label: t('消耗金额'),
          value: renderQuota(consumeQuota),
          sub: t('范围内累计'),
          loading,
        },
      ],
    },
    {
      label: t('今日消耗'),
      color: 'green',
      icon: <IconTextStroked style={{ fontSize: 14 }} />,
      iconBg: 'bg-green-100',
      valueColor: 'text-green-600',
      metrics: [
        {
          label: t('今日 Tokens'),
          value: isNaN(todayConsumeTokens) ? 0 : renderNumber(todayConsumeTokens),
          sub: todayConsumeTokens >= 10000
            ? `≈ ${(todayConsumeTokens / 10000).toFixed(1)}${t('万')}`
            : null,
          loading: todayLoading,
        },
        {
          label: t('今日金额'),
          value: renderQuota(todayConsumeQuota),
          sub: t('截至当前'),
          loading: todayLoading,
        },
      ],
    },
    {
      label: t('请求次数'),
      color: 'amber',
      icon: <IconSend style={{ fontSize: 14 }} />,
      iconBg: 'bg-amber-100',
      valueColor: 'text-amber-600',
      metrics: [
        {
          label: t('今日请求'),
          value: renderNumber(todayTimes),
          sub: t('截至当前'),
          loading: todayLoading,
        },
        {
          label: t('范围请求'),
          value: renderNumber(times),
          sub: t('选定范围内'),
          loading,
        },
      ],
    },
  ];

  const colorMap = {
    purple: { stroke: '#a855f7', iconBg: 'bg-purple-100', text: 'text-purple-600' },
    green: { stroke: '#22c55e', iconBg: 'bg-green-100', text: 'text-green-600' },
    amber: { stroke: '#f59e0b', iconBg: 'bg-amber-100', text: 'text-amber-600' },
  };

  return (
    <Card
      {...CARD_PROPS}
      className="!rounded-2xl"
      title={
        <div className={FLEX_CENTER_GAP2}>
          <Wallet size={16} />
          {t('数据概览')}
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <ScrollableContainer maxHeight="24rem">
        <div className="divide-y divide-gray-100">
          {sections.map((section, idx) => {
            const colors = colorMap[section.color];
            return (
              <div key={idx} className="px-4 py-3">
                {/* Section header */}
                <div className="flex items-center justify-between mb-2.5">
                  <div className="flex items-center gap-1.5">
                    <svg width="12" height="12" viewBox="0 0 12 12">
                      <circle cx="6" cy="6" r="4" fill={colors.stroke} opacity="0.2" />
                      <circle cx="6" cy="6" r="2.5" fill={colors.stroke} />
                    </svg>
                    <span className="text-xs font-semibold text-gray-500">
                      {section.label}
                    </span>
                  </div>
                  <div
                    className={`w-7 h-7 rounded-lg ${colors.iconBg} flex items-center justify-center`}
                  >
                    {React.cloneElement(section.icon, {
                      style: { fontSize: 14, color: colors.stroke },
                    })}
                  </div>
                </div>
                {/* Metrics grid */}
                <div className="grid grid-cols-2 gap-1.5">
                  {section.metrics.map((metric, mIdx) => (
                    <div
                      key={mIdx}
                      className="px-2 py-1.5 rounded-lg hover:bg-gray-50 transition-colors"
                    >
                      <div className="text-[10px] text-gray-400 mb-0.5">
                        {metric.label}
                      </div>
                      <Skeleton
                        loading={metric.loading}
                        active
                        placeholder={
                          <Skeleton.Paragraph
                            active
                            rows={1}
                            style={{ width: '60px', height: '20px' }}
                          />
                        }
                      >
                        <div
                          className={`text-base font-bold leading-tight tracking-tight ${colors.text}`}
                        >
                          {metric.value}
                        </div>
                      </Skeleton>
                      {metric.sub && (
                        <div className="text-[10px] text-gray-400 mt-0.5">
                          {metric.sub}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      </ScrollableContainer>
    </Card>
  );
};

export default DataOverviewPanel;
