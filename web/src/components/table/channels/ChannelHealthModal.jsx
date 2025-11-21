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

import React, { useState, useEffect } from 'react';
import {
  Modal,
  Card,
  Banner,
  Tag,
  Progress,
  Typography,
  Space,
  Button,
  Toast,
  Row,
  Col,
  Collapse,
  Tooltip,
} from '@douyinfe/semi-ui';
import {
  IconTickCircle,
  IconAlertTriangle,
  IconActivity,
  IconClock,
} from '@douyinfe/semi-icons';
import { formatDistanceToNow } from 'date-fns';
import { zhCN } from 'date-fns/locale';
import { API } from '../../../helpers';

const { Text, Title } = Typography;

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
  const [countdown, setCountdown] = useState('');
  const [cooldownComplete, setCooldownComplete] = useState(false);

  // No data, nothing to render — bail out early to skip unnecessary work
  if (!health) {
    return null;
  }

  // Extract health data with defaults
  const {
    consecutive_failures = 0,
    current_failure_rate = 0,
    is_suspended = false,
    suspended_until = null,
    suspension_count = 0,
    last_failure_time = null,
    last_success_time = null,
    total_failures = 0,
    total_successes = 0,
    window_total_requests = 0,
    window_failure_count = 0,
  } = health;

  // 计算总请求数和成功率
  const totalRequests = total_failures + total_successes;
  const successRate =
    totalRequests > 0
      ? ((total_successes / totalRequests) * 100).toFixed(2)
      : 0;

  // Helper function: Get failure rate color
  const getFailureRateColor = (rate) => {
    if (rate < 0.1) return 'var(--semi-color-success)'; // Green
    if (rate <= 0.3) return 'var(--semi-color-warning)'; // Orange
    return 'var(--semi-color-danger)'; // Red
  };

  // Helper function: Get success rate color
  const getSuccessRateColor = (ratePercent) => {
    if (ratePercent >= 90) return 'var(--semi-color-success)'; // Green
    if (ratePercent >= 70) return 'var(--semi-color-warning)'; // Orange
    return 'var(--semi-color-danger)'; // Red
  };

  // Helper function: Get consecutive failures color
  const getConsecutiveFailuresColor = (count) => {
    if (count === 0) return 'var(--semi-color-success)'; // Green
    if (count <= 5) return 'var(--semi-color-warning)'; // Orange
    return 'var(--semi-color-danger)'; // Red
  };

  // Helper function: Get status badge
  const getStatusBadge = () => {
    if (is_suspended) {
      return <Tag color="red" size="large">🔴 已暂停</Tag>;
    }
    if (consecutive_failures > 0 || current_failure_rate > 0.1) {
      return <Tag color="orange" size="large">⚠️ 警告</Tag>;
    }
    return <Tag color="green" size="large">✅ 正常</Tag>;
  };

  // 计算冷却进度和剩余时间
  let cooldownProgress = 0;
  let totalDurationMinutes = 5;
  let remainingMs = 0;

  if (is_suspended && suspended_until) {
    const baseMins = 5.0;
    const maxMins = 60.0;
    totalDurationMinutes = Math.min(
      baseMins * Math.pow(2, suspension_count - 1),
      maxMins
    );

    const now = new Date();
    const suspendedAt = new Date(suspended_until);
    remainingMs = suspendedAt - now;
    const totalDuration = totalDurationMinutes * 60 * 1000;
    const elapsed = totalDuration - remainingMs;
    cooldownProgress = Math.max(
      0,
      Math.min(100, (elapsed / totalDuration) * 100)
    );
  }

  // Real-time countdown timer
  useEffect(() => {
    if (!is_suspended || !suspended_until) {
      setCountdown('');
      setCooldownComplete(false); // Reset when not suspended
      return;
    }

    // Reset cooldownComplete when modal opens or suspension changes
    setCooldownComplete(false);

    const updateCountdown = () => {
      const now = new Date();
      const target = new Date(suspended_until);
      const diff = target - now;

      if (diff <= 0) {
        setCountdown('冷却完成');
        setCooldownComplete(true);
        return;
      }

      const minutes = Math.floor(diff / 60000);
      const seconds = Math.floor((diff % 60000) / 1000);

      if (minutes > 0) {
        setCountdown(`冷却中 - 还剩 ${minutes} 分钟 ${seconds} 秒`);
      } else {
        setCountdown(`冷却中 - 还剩 ${seconds} 秒`);
      }
    };

    updateCountdown();
    const interval = setInterval(updateCountdown, 1000);

    return () => clearInterval(interval);
  }, [is_suspended, suspended_until, visible]);

  // 手动重置处理函数
  const handleReset = async () => {
    setIsResetting(true);
    try {
      const res = await API.post(`/api/channel/${channelId}/health/reset`);
      const { success, message } = res.data;

      if (success) {
        Toast.success('渠道健康状态已重置');
        onHealthReset();
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
      <Space vertical spacing="large" style={{ width: '100%' }}>
        {/* Top Section: Status Summary Card */}
        <Card
          style={{
            background: 'var(--semi-color-fill-0)',
          }}
          bodyStyle={{ padding: '20px' }}
        >
          <Space vertical spacing="medium" style={{ width: '100%' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              {getStatusBadge()}
            </div>

            <div>
              <Text type="secondary" size="small">当前窗口失败率</Text>
              <div>
                <Text
                  style={{
                    fontSize: '28px',
                    fontWeight: 'bold',
                    color: getFailureRateColor(current_failure_rate),
                  }}
                >
                  {(current_failure_rate * 100).toFixed(2)}%
                </Text>
              </div>
            </div>

            <div>
              <Text type="secondary" size="small">连续失败</Text>
              <Tooltip content="达到 10 次将自动暂停渠道">
                <Progress
                  percent={(consecutive_failures / 10) * 100}
                  stroke={getConsecutiveFailuresColor(consecutive_failures)}
                  showInfo={false}
                  style={{ marginTop: 8 }}
                />
              </Tooltip>
              <Text
                type="secondary"
                size="small"
                style={{
                  marginTop: 4,
                  display: 'block',
                  color: getConsecutiveFailuresColor(consecutive_failures),
                }}
              >
                {consecutive_failures} / 10
              </Text>
            </div>
          </Space>
        </Card>

        {/* Middle Section: Suspension Alert Banner (conditional) */}
        {is_suspended && suspended_until && (
          <Banner
            type={cooldownComplete ? 'success' : 'warning'}
            icon={<IconClock />}
            description={
              <div>
                <div style={{ marginBottom: 8 }}>
                  <Text strong>
                    {cooldownComplete
                      ? '✅ 渠道已恢复，可以重新使用'
                      : countdown}
                  </Text>
                  {suspension_count > 0 && !cooldownComplete && (
                    <Text type="tertiary" style={{ marginLeft: 12 }}>
                      第 {suspension_count} 次暂停
                    </Text>
                  )}
                </div>
                {!cooldownComplete && (
                  <>
                    <Progress
                      percent={cooldownProgress}
                      showInfo={false}
                      stroke="var(--semi-color-warning)"
                      style={{ marginBottom: 8 }}
                    />
                    <Text type="tertiary" size="small">
                      预计恢复时间:{' '}
                      {new Date(suspended_until).toLocaleTimeString('zh-CN', {
                        hour: '2-digit',
                        minute: '2-digit',
                      })}
                    </Text>
                  </>
                )}
              </div>
            }
            fullMode
          />
        )}

        {/* Bottom Section: Metrics Grid */}
        <Row gutter={16}>
          <Col xs={24} sm={8}>
            <Card
              style={{ textAlign: 'center' }}
              bodyStyle={{ padding: '16px' }}
            >
              <Space vertical spacing="small" align="center" style={{ width: '100%' }}>
                <IconTickCircle
                  size="extra-large"
                  style={{ color: 'var(--semi-color-success)', fontSize: '32px' }}
                />
                <Text type="secondary" size="small">成功次数</Text>
                <Title
                  heading={3}
                  style={{ margin: 0, color: 'var(--semi-color-success)' }}
                >
                  {total_successes.toLocaleString()}
                </Title>
              </Space>
            </Card>
          </Col>
          <Col xs={24} sm={8}>
            <Card
              style={{ textAlign: 'center' }}
              bodyStyle={{ padding: '16px' }}
            >
              <Space vertical spacing="small" align="center" style={{ width: '100%' }}>
                <IconAlertTriangle
                  size="extra-large"
                  style={{
                    color:
                      current_failure_rate > 0.3
                        ? 'var(--semi-color-danger)'
                        : 'var(--semi-color-tertiary)',
                    fontSize: '32px'
                  }}
                />
                <Text type="secondary" size="small">失败次数</Text>
                <Title
                  heading={3}
                  style={{
                    margin: 0,
                    color:
                      current_failure_rate > 0.3
                        ? 'var(--semi-color-danger)'
                        : 'inherit',
                  }}
                >
                  {total_failures.toLocaleString()}
                </Title>
              </Space>
            </Card>
          </Col>
          <Col xs={24} sm={8}>
            <Card
              style={{ textAlign: 'center' }}
              bodyStyle={{ padding: '16px' }}
            >
              <Space vertical spacing="small" align="center" style={{ width: '100%' }}>
                <IconActivity
                  size="extra-large"
                  style={{ fontSize: '32px' }}
                />
                <Text type="secondary" size="small">成功率</Text>
                <Title
                  heading={3}
                  style={{
                    margin: 0,
                    color: getSuccessRateColor(parseFloat(successRate)),
                  }}
                >
                  {successRate}%
                </Title>
              </Space>
            </Card>
          </Col>
        </Row>

        {/* Collapsible Details */}
        <Collapse defaultActiveKey={[]} accordion={false}>
          <Collapse.Panel header="📈 窗口统计 (最近 100 次请求)" itemKey="window">
            <Space vertical spacing="small" style={{ width: '100%' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text type="secondary">窗口请求数</Text>
                <Text strong>{window_total_requests}</Text>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text type="secondary">窗口失败数</Text>
                <Text strong>{window_failure_count}</Text>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text type="secondary">窗口失败率</Text>
                <Text
                  strong
                  style={{ color: getFailureRateColor(current_failure_rate) }}
                >
                  {(current_failure_rate * 100).toFixed(2)}%
                </Text>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text type="secondary">最后成功时间</Text>
                <Text>
                  {last_success_time && last_success_time !== '0001-01-01T00:00:00Z'
                    ? formatDistanceToNow(new Date(last_success_time), {
                        addSuffix: true,
                        locale: zhCN,
                      })
                    : '无'}
                </Text>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text type="secondary">最后失败时间</Text>
                <Text>
                  {last_failure_time && last_failure_time !== '0001-01-01T00:00:00Z'
                    ? formatDistanceToNow(new Date(last_failure_time), {
                        addSuffix: true,
                        locale: zhCN,
                      })
                    : '无'}
                </Text>
              </div>
            </Space>
          </Collapse.Panel>

          <Collapse.Panel header="📊 历史统计" itemKey="history">
            <Space vertical spacing="small" style={{ width: '100%' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text type="secondary">总请求数</Text>
                <Text strong>{totalRequests.toLocaleString()}</Text>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text type="secondary">总成功次数</Text>
                <Text strong>{total_successes.toLocaleString()}</Text>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text type="secondary">总失败次数</Text>
                <Text strong>{total_failures.toLocaleString()}</Text>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text type="secondary">总成功率</Text>
                <Text strong style={{ color: 'var(--semi-color-success)' }}>
                  {successRate}%
                </Text>
              </div>
              {suspension_count > 0 && (
                <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <Text type="secondary">历史暂停次数</Text>
                  <Text strong type="warning">{suspension_count}</Text>
                </div>
              )}
            </Space>
          </Collapse.Panel>
        </Collapse>
      </Space>
    </Modal>
  );
};

export default ChannelHealthModal;
