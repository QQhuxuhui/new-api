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
import { Skeleton } from '@douyinfe/semi-ui';
import {
  IconCoinMoneyStroked,
  IconTextStroked,
  IconSend,
} from '@douyinfe/semi-icons';
import { Wallet } from 'lucide-react';
import { renderQuota, renderNumber } from '../../helpers';
import ScrollableContainer from '../common/ui/ScrollableContainer';

const colorMap = {
  green: {
    gradient: 'from-green-400 to-emerald-500',
    iconBg: 'bg-green-50',
    stroke: '#22c55e',
    text: 'text-green-600',
  },
  purple: {
    gradient: 'from-purple-400 to-violet-500',
    iconBg: 'bg-purple-50',
    stroke: '#a855f7',
    text: 'text-purple-600',
  },
  amber: {
    gradient: 'from-amber-400 to-orange-500',
    iconBg: 'bg-amber-50',
    stroke: '#f59e0b',
    text: 'text-amber-600',
  },
};

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
  // 今日消耗放最上面
  const sections = [
    {
      label: t('今日消耗'),
      color: 'green',
      badge: t('截至当前'),
      icon: <IconTextStroked style={{ fontSize: 14 }} />,
      metrics: [
        {
          label: t('今日 Tokens'),
          value: isNaN(todayConsumeTokens) ? 0 : renderNumber(todayConsumeTokens),
          sub: todayConsumeTokens >= 10000
            ? `≈ ${(todayConsumeTokens / 10000).toFixed(1)}${t('万')}`
            : null,
          loading: todayLoading,
          highlight: false,
        },
        {
          label: t('今日金额'),
          value: renderQuota(todayConsumeQuota),
          sub: t('截至当前'),
          loading: todayLoading,
          highlight: true,
        },
      ],
    },
    {
      label: t('统计范围消耗'),
      color: 'purple',
      badge: t('范围内累计'),
      icon: <IconCoinMoneyStroked style={{ fontSize: 14 }} />,
      metrics: [
        {
          label: t('总 Tokens'),
          value: isNaN(consumeTokens) ? 0 : renderNumber(consumeTokens),
          sub: consumeTokens >= 10000
            ? `≈ ${(consumeTokens / 10000).toFixed(1)}${t('万')}`
            : null,
          loading,
          highlight: false,
        },
        {
          label: t('消耗金额'),
          value: renderQuota(consumeQuota),
          sub: t('范围内累计'),
          loading,
          highlight: true,
        },
      ],
    },
    {
      label: t('请求次数'),
      color: 'amber',
      icon: <IconSend style={{ fontSize: 14 }} />,
      metrics: [
        {
          label: t('今日请求'),
          value: renderNumber(todayTimes),
          sub: t('截至当前'),
          loading: todayLoading,
          highlight: false,
        },
        {
          label: t('范围请求'),
          value: renderNumber(times),
          sub: t('选定范围内'),
          loading,
          highlight: true,
        },
      ],
    },
  ];

  return (
    <div className="flex flex-col gap-3">
      {/* 标题 */}
      <div className={FLEX_CENTER_GAP2 + ' px-1'}>
        <Wallet size={16} className="text-gray-500" />
        <span className="text-sm font-semibold text-gray-500">{t('数据概览')}</span>
      </div>

      <ScrollableContainer maxHeight="24rem">
        <div className="flex flex-col gap-3">
          {sections.map((section, idx) => {
            const colors = colorMap[section.color];
            return (
              <div
                key={idx}
                className="bg-white rounded-2xl shadow-sm border border-gray-100 flex overflow-hidden cursor-pointer hover:shadow-md transition-shadow duration-200"
              >
                {/* 左侧渐变条 */}
                <div className={`w-1 shrink-0 bg-gradient-to-b ${colors.gradient}`} />

                <div className="flex-1 p-4 relative overflow-hidden">
                  {/* 右上角光晕 */}
                  <div
                    className="absolute top-0 right-0 w-[120px] h-[120px] rounded-full pointer-events-none"
                    style={{
                      background: colors.stroke,
                      filter: 'blur(40px)',
                      opacity: 0.08,
                    }}
                  />

                  {/* Section header */}
                  <div className="flex items-center justify-between mb-3 relative">
                    <div className="flex items-center gap-2">
                      <div className={`w-7 h-7 rounded-lg ${colors.iconBg} flex items-center justify-center`}>
                        {React.cloneElement(section.icon, {
                          style: { fontSize: 14, color: colors.stroke },
                        })}
                      </div>
                      <span className="text-xs font-semibold text-gray-500 tracking-wide">
                        {section.label}
                      </span>
                    </div>
                    {section.badge && (
                      <span className="text-[10px] text-gray-400 bg-gray-50 px-2 py-0.5 rounded-full">
                        {section.badge}
                      </span>
                    )}
                  </div>

                  {/* Metrics grid */}
                  <div className="grid grid-cols-2 gap-3 relative">
                    {section.metrics.map((metric, mIdx) => (
                      <div
                        key={mIdx}
                        className="px-2 py-1.5 rounded-lg hover:bg-gray-50 transition-colors"
                      >
                        <div className="text-[10px] text-gray-400 mb-1">
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
                            className={`text-lg font-bold leading-tight tracking-tight ${metric.highlight ? colors.text : 'text-gray-800'}`}
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
              </div>
            );
          })}
        </div>
      </ScrollableContainer>
    </div>
  );
};

export default DataOverviewPanel;
