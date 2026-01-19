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
import { Card } from '@douyinfe/semi-ui';
import { PieChart } from 'lucide-react';
import { VChart } from '@visactor/react-vchart';

const ChartsPanel = ({
  activeChartTab,
  setActiveChartTab,
  spec_line,
  spec_model_line,
  spec_pie,
  spec_rank_bar,
  spec_token_line,
  spec_token_bar,
  CARD_PROPS,
  CHART_CONFIG,
  FLEX_CENTER_GAP2,
  hasApiInfoPanel,
  t,
}) => {
  const tabs = [
    { key: '1', label: t('消耗分布') },
    { key: '2', label: t('消耗趋势') },
    { key: '3', label: t('调用次数分布') },
    { key: '4', label: t('调用次数排行') },
    { key: '5', label: t('Token用量趋势') },
    { key: '6', label: t('Token用量分布') },
  ];

  return (
    <Card
      {...CARD_PROPS}
      className={`!rounded-2xl ${hasApiInfoPanel ? 'lg:col-span-3' : ''}`}
      title={
        <div className='flex flex-col gap-4 w-full'>
          <div className={FLEX_CENTER_GAP2}>
            <PieChart size={16} />
            {t('模型数据分析')}
          </div>
          <div className='flex flex-wrap gap-2'>
            {tabs.map((tab) => (
              <button
                key={tab.key}
                onClick={() => setActiveChartTab(tab.key)}
                style={{
                  padding: '8px 16px',
                  borderRadius: '8px',
                  fontSize: '14px',
                  fontWeight: '500',
                  border: 'none',
                  cursor: 'pointer',
                  transition: 'all 0.2s',
                  backgroundColor: activeChartTab === tab.key ? '#3b82f6' : '#f3f4f6',
                  color: activeChartTab === tab.key ? '#ffffff' : '#374151',
                  boxShadow: activeChartTab === tab.key ? '0 1px 2px 0 rgba(0, 0, 0, 0.05)' : 'none',
                }}
                onMouseEnter={(e) => {
                  if (activeChartTab !== tab.key) {
                    e.currentTarget.style.backgroundColor = '#e5e7eb';
                  }
                }}
                onMouseLeave={(e) => {
                  if (activeChartTab !== tab.key) {
                    e.currentTarget.style.backgroundColor = '#f3f4f6';
                  }
                }}
              >
                {tab.label}
              </button>
            ))}
          </div>
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <div className='h-96 p-2'>
        {activeChartTab === '1' && (
          <VChart spec={spec_line} option={CHART_CONFIG} />
        )}
        {activeChartTab === '2' && (
          <VChart spec={spec_model_line} option={CHART_CONFIG} />
        )}
        {activeChartTab === '3' && (
          <VChart spec={spec_pie} option={CHART_CONFIG} />
        )}
        {activeChartTab === '4' && (
          <VChart spec={spec_rank_bar} option={CHART_CONFIG} />
        )}
        {activeChartTab === '5' && (
          <VChart spec={spec_token_line} option={CHART_CONFIG} />
        )}
        {activeChartTab === '6' && (
          <VChart spec={spec_token_bar} option={CHART_CONFIG} />
        )}
      </div>
    </Card>
  );
};

export default ChartsPanel;
