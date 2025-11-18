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
import { IconCheckCircle, IconClock, IconAlertTriangle } from '@douyinfe/semi-icons';

/**
 * 通道健康状态指示器组件
 * @param {Object} health - 健康状态数据
 * @param {Function} onClick - 点击回调函数
 */
const ChannelHealthStatus = ({ health, onClick }) => {
  if (!health) {
    return (
      <Tag color="grey" size="small">
        未知
      </Tag>
    );
  }

  const { is_suspended, consecutive_failures } = health;

  // 已暂停状态
  if (is_suspended) {
    return (
      <Tooltip content="点击查看详情">
        <Tag
          color="orange"
          size="small"
          prefixIcon={<IconClock />}
          onClick={onClick}
          style={{ cursor: 'pointer' }}
        >
          已暂停
        </Tag>
      </Tooltip>
    );
  }

  // 警告状态（有连续失败但未暂停）
  if (consecutive_failures > 0) {
    return (
      <Tooltip content={`连续高失败率周期: ${consecutive_failures} 次`}>
        <Tag
          color="yellow"
          size="small"
          prefixIcon={<IconAlertTriangle />}
          onClick={onClick}
          style={{ cursor: 'pointer' }}
        >
          警告
        </Tag>
      </Tooltip>
    );
  }

  // 正常状态
  return (
    <Tooltip content="点击查看详情">
      <Tag
        color="green"
        size="small"
        prefixIcon={<IconCheckCircle />}
        onClick={onClick}
        style={{ cursor: 'pointer' }}
      >
        正常
      </Tag>
    </Tooltip>
  );
};

export default ChannelHealthStatus;
