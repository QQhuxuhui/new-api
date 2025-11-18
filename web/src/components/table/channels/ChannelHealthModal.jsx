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

import React, { useState } from 'react';
import {
  Modal,
  Descriptions,
  Tag,
  Progress,
  Typography,
  Space,
  Button,
  Toast,
} from '@douyinfe/semi-ui';
import { formatDistanceToNow } from 'date-fns';
import { zhCN } from 'date-fns/locale';
import { API } from '../../../helpers';

const { Text } = Typography;

/**
 * 通道健康状态详情弹窗
 * @param {boolean} visible - 是否显示弹窗
 * @param {Object} health - 健康状态数据
 * @param {number} channelId - 通道ID
 * @param {Function} onClose - 关闭回调
 * @param {Function} onHealthReset - 重置成功回调
 */
const ChannelHealthModal = ({
  visible,
  health,
  channelId,
  onClose,
  onHealthReset,
}) => {
  const [isResetting, setIsResetting] = useState(false);

  if (!health) return null;

  const {
    consecutive_failures,
    current_failure_rate,
    is_suspended,
    suspended_until,
    suspension_count,
    last_failure_time,
    last_success_time,
    total_failures,
    total_successes,
    window_total_requests,
    window_failure_count,
  } = health;

  // 计算总请求数和成功率
  const totalRequests = total_failures + total_successes;
  const successRate =
    totalRequests > 0
      ? ((total_successes / totalRequests) * 100).toFixed(2)
      : 0;

  // 计算冷却进度
  let cooldownProgress = 0;
  let totalDurationMinutes = 5; // 默认
  if (is_suspended && suspended_until) {
    // 根据暂停次数计算实际暂停时长（指数退避）
    const baseMins = 5.0;
    const maxMins = 60.0;
    totalDurationMinutes = Math.min(
      baseMins * Math.pow(2, suspension_count - 1),
      maxMins
    );

    const now = new Date();
    const suspendedAt = new Date(suspended_until);
    const totalDuration = totalDurationMinutes * 60 * 1000; // 转换为毫秒
    const elapsed = totalDuration - (suspendedAt - now);
    cooldownProgress = Math.max(
      0,
      Math.min(100, (elapsed / totalDuration) * 100)
    );
  }

  // 手动重置处理函数
  const handleReset = async () => {
    setIsResetting(true);
    try {
      const res = await API.post(`/api/channel/${channelId}/health/reset`);
      const { success, message } = res.data;

      if (success) {
        Toast.success('渠道健康状态已重置');
        onHealthReset(); // 刷新父组件数据
        onClose();
      } else {
        Toast.error(message || '重置失败');
      }
    } catch (err) {
      Toast.error('重置请求失败');
    } finally {
      setIsResetting(false);
    }
  };

  return (
    <Modal
      title="通道健康状态详情"
      visible={visible}
      onCancel={onClose}
      footer={
        <Space>
          <Button onClick={onClose}>关闭</Button>
          {/* 只在暂停或有连续失败时显示重置按钮 */}
          {(is_suspended || consecutive_failures > 0) && (
            <Button
              type="danger"
              theme="solid"
              onClick={handleReset}
              loading={isResetting}
            >
              重置健康状态
            </Button>
          )}
        </Space>
      }
      width={700}
      style={{ maxWidth: '90vw' }}
    >
      <Descriptions row size="medium">
        <Descriptions.Item itemKey="状态">
          {is_suspended ? (
            <Tag color="orange">已暂停</Tag>
          ) : consecutive_failures > 0 ? (
            <Tag color="yellow">警告</Tag>
          ) : (
            <Tag color="green">正常</Tag>
          )}
        </Descriptions.Item>

        <Descriptions.Item itemKey="连续高失败率周期">
          <Text type={consecutive_failures >= 3 ? 'danger' : 'secondary'}>
            {consecutive_failures} / 10
          </Text>
        </Descriptions.Item>

        <Descriptions.Item itemKey="当前窗口失败率">
          <Text
            type={current_failure_rate > 0.3 ? 'danger' : 'secondary'}
            strong
          >
            {(current_failure_rate * 100).toFixed(2)}%
          </Text>
        </Descriptions.Item>

        <Descriptions.Item itemKey="窗口请求数">
          <Text type="secondary">
            {window_total_requests} 请求 ({window_failure_count} 失败)
          </Text>
        </Descriptions.Item>

        {suspension_count > 0 && (
          <Descriptions.Item itemKey="暂停次数">
            <Text type="warning">第 {suspension_count} 次暂停</Text>
          </Descriptions.Item>
        )}

        {is_suspended && suspended_until && (
          <Descriptions.Item itemKey="冷却时间" span={3}>
            <div>
              <Text>
                还剩{' '}
                {formatDistanceToNow(new Date(suspended_until), {
                  locale: zhCN,
                })}{' '}
                <Text type="tertiary">({totalDurationMinutes}分钟)</Text>
              </Text>
              <Progress
                percent={cooldownProgress}
                showInfo={false}
                stroke="var(--semi-color-warning)"
                style={{ marginTop: 8 }}
              />
            </div>
          </Descriptions.Item>
        )}

        <Descriptions.Item itemKey="最后成功时间">
          {last_success_time && last_success_time !== '0001-01-01T00:00:00Z'
            ? formatDistanceToNow(new Date(last_success_time), {
                addSuffix: true,
                locale: zhCN,
              })
            : '无'}
        </Descriptions.Item>

        <Descriptions.Item itemKey="最后失败时间">
          {last_failure_time && last_failure_time !== '0001-01-01T00:00:00Z'
            ? formatDistanceToNow(new Date(last_failure_time), {
                addSuffix: true,
                locale: zhCN,
              })
            : '无'}
        </Descriptions.Item>

        <Descriptions.Item itemKey="总请求数">
          {totalRequests.toLocaleString()}
        </Descriptions.Item>

        <Descriptions.Item itemKey="成功次数">
          {total_successes.toLocaleString()}
        </Descriptions.Item>

        <Descriptions.Item itemKey="失败次数">
          {total_failures.toLocaleString()}
        </Descriptions.Item>

        <Descriptions.Item itemKey="成功率">
          <Text strong>{successRate}%</Text>
        </Descriptions.Item>
      </Descriptions>
    </Modal>
  );
};

export default ChannelHealthModal;
