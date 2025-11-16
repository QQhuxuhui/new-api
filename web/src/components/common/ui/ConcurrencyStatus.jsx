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
import { Tag, Tooltip } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

/**
 * Get color based on usage percentage
 * @param {number} usagePercent - Usage percentage (0-100)
 * @returns {string} - Semi Design color
 */
const getUsageColor = (usagePercent) => {
  if (usagePercent < 0) return 'grey'; // Unknown/unavailable
  if (usagePercent >= 80) return 'red';
  if (usagePercent >= 50) return 'orange';
  return 'green';
};

/**
 * ConcurrencyStatus - Display concurrency information
 * @param {Object} props
 * @param {Object} props.concurrencyInfo - Concurrency info from API
 * @param {boolean} props.isMultiKey - Whether this is a multi-key channel
 * @param {boolean} props.compact - Compact display mode (default: true)
 * @param {boolean} props.showTooltip - Show detailed tooltip (default: true)
 */
const ConcurrencyStatus = ({
  concurrencyInfo,
  isMultiKey = false,
  compact = true,
  showTooltip = true,
}) => {
  const { t } = useTranslation();

  // No concurrency info
  if (!concurrencyInfo) {
    return (
      <Tag color='grey' size='small' shape='circle'>
        -
      </Tag>
    );
  }

  // Handle multi-key channels
  if (isMultiKey && concurrencyInfo.keys) {
    const { total_current, total_capacity, usage_percent } = concurrencyInfo;
    const color = getUsageColor(usage_percent);

    // Unknown state
    if (total_current < 0) {
      return (
        <Tag color='grey' size='small' shape='circle'>
          {t('数据不可用')}
        </Tag>
      );
    }

    const mainDisplay = (
      <Tag color={color} size='small' shape='circle'>
        {total_current}/{total_capacity}
      </Tag>
    );

    // Show tooltip with per-key breakdown
    if (showTooltip) {
      const tooltipContent = (
        <div className='flex flex-col gap-2' style={{ maxWidth: '300px' }}>
          <div className='font-semibold'>{t('密钥并发详情')}</div>
          {concurrencyInfo.keys.map((key) => (
            <div key={key.key_index} className='flex justify-between items-center'>
              <span>
                {t('密钥')} #{key.key_index + 1}:
              </span>
              <span>
                <Tag
                  color={getUsageColor(key.usage_percent)}
                  size='small'
                  shape='circle'
                >
                  {key.current >= 0 ? `${key.current}/${key.limit}` : t('数据不可用')}
                </Tag>
                {key.status === 'disabled' && (
                  <Tag color='grey' size='small' style={{ marginLeft: '4px' }}>
                    {t('已禁用')}
                  </Tag>
                )}
                {key.status === 'at_limit' && (
                  <Tag color='red' size='small' style={{ marginLeft: '4px' }}>
                    {t('已达上限')}
                  </Tag>
                )}
              </span>
            </div>
          ))}
          <div className='text-xs text-gray-500 mt-1'>
            {t('总使用率')}: {usage_percent.toFixed(1)}%
          </div>
        </div>
      );

      return <Tooltip content={tooltipContent}>{mainDisplay}</Tooltip>;
    }

    return mainDisplay;
  }

  // Handle single-key channels
  const { current, limit, usage_percent } = concurrencyInfo;
  const color = getUsageColor(usage_percent);

  // Unknown state
  if (current < 0) {
    return (
      <Tag color='grey' size='small' shape='circle'>
        {t('数据不可用')}
      </Tag>
    );
  }

  const mainDisplay = (
    <Tag color={color} size='small' shape='circle'>
      {current}/{limit}
    </Tag>
  );

  // Show tooltip with usage percentage
  if (showTooltip && usage_percent >= 0) {
    const tooltipContent = (
      <div className='flex flex-col gap-1'>
        <div>
          {t('当前并发')}: {current}
        </div>
        <div>
          {t('并发限制')}: {limit}
        </div>
        <div>
          {t('并发使用率')}: {usage_percent.toFixed(1)}%
        </div>
        {usage_percent >= 80 && (
          <div className='text-red-500 font-semibold mt-1'>
            ⚠️ {t('高并发使用')}
          </div>
        )}
      </div>
    );

    return <Tooltip content={tooltipContent}>{mainDisplay}</Tooltip>;
  }

  return mainDisplay;
};

export default ConcurrencyStatus;
