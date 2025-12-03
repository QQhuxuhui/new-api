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
import {
  Card,
  Row,
  Col,
  Table,
  Tag,
  Typography,
  Empty,
  Spin,
  Banner,
  Divider,
} from '@douyinfe/semi-ui';
import {
  IconMoneyExchangeStroked,
  IconAlertTriangle,
  IconTickCircle,
} from '@douyinfe/semi-icons';
import { useChannelCostData } from '../../../hooks/analytics/useChannelCostData';

const { Text, Title } = Typography;

// Custom Statistic component
const Statistic = ({ title, value, prefix, suffix, valueColor }) => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
    <Text type="tertiary" style={{ fontSize: '14px' }}>
      {title}
    </Text>
    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
      {prefix && <span style={{ fontSize: '24px' }}>{prefix}</span>}
      <Text strong style={{ fontSize: '24px', color: valueColor }}>
        {value || 0}
      </Text>
      {suffix && <span>{suffix}</span>}
    </div>
  </div>
);

const CostEfficiencyTab = ({ timeRange }) => {
  const {
    loading,
    error,
    channelCostData,
    costTrendData,
    modelProfitabilityData,
  } = useChannelCostData(timeRange);

  if (loading) {
    return (
      <div
        style={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          minHeight: 400,
        }}
      >
        <Spin size="large" />
      </div>
    );
  }

  if (error) {
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description={error}
        style={{ marginTop: 40 }}
      />
    );
  }

  if (!channelCostData) {
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="No cost data available"
        style={{ marginTop: 40 }}
      />
    );
  }

  const { channels, summary, data_quality, warnings } = channelCostData;

  // Format currency
  const formatUSD = (value) => {
    if (value === null || value === undefined) return '$0.00';
    return `$${value.toFixed(2)}`;
  };

  // Get margin color
  const getMarginColor = (margin) => {
    if (margin < 0) return '#ff4d4f'; // Red for negative
    if (margin < 10) return '#faad14'; // Orange for low
    if (margin < 20) return '#1890ff'; // Blue for medium
    return '#52c41a'; // Green for good
  };

  // Get margin tag
  const getMarginTag = (margin) => {
    if (margin < 0) return <Tag color="red">负利润</Tag>;
    if (margin < 10) return <Tag color="orange">低利润</Tag>;
    if (margin < 20) return <Tag color="blue">中等</Tag>;
    return <Tag color="green">良好</Tag>;
  };

  // Channel cost table columns
  const channelColumns = [
    {
      title: '渠道名称',
      dataIndex: 'channel_name',
      key: 'channel_name',
      ellipsis: true,
    },
    {
      title: '业务量',
      dataIndex: 'total_requests',
      key: 'business_volume',
      sorter: (a, b) => a.total_requests - b.total_requests,
      render: (requests, record) => (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
          <Text>{requests?.toLocaleString() || 0} requests</Text>
          <Text type="tertiary" style={{ fontSize: '12px' }}>
            {((record.total_tokens || 0) / 1000000).toFixed(2)}M tokens
          </Text>
        </div>
      ),
    },
    {
      title: '收入',
      dataIndex: 'revenue_usd',
      key: 'revenue_usd',
      sorter: (a, b) => a.revenue_usd - b.revenue_usd,
      render: (revenue) => (
        <Text style={{ color: '#52c41a' }}>{formatUSD(revenue)}</Text>
      ),
    },
    {
      title: '成本',
      dataIndex: 'cost_usd',
      key: 'cost_usd',
      sorter: (a, b) => a.cost_usd - b.cost_usd,
      render: (cost) => (
        <Text style={{ color: '#ff4d4f' }}>{formatUSD(cost)}</Text>
      ),
    },
    {
      title: '利润',
      dataIndex: 'profit_usd',
      key: 'profit_usd',
      sorter: (a, b) => a.profit_usd - b.profit_usd,
      render: (profit) => (
        <Text style={{ color: profit >= 0 ? '#52c41a' : '#ff4d4f' }}>
          {formatUSD(profit)}
        </Text>
      ),
    },
    {
      title: '利润率',
      dataIndex: 'profit_margin',
      key: 'profit_margin',
      sorter: (a, b) => a.profit_margin - b.profit_margin,
      render: (margin) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <Text style={{ color: getMarginColor(margin) }}>
            {margin?.toFixed(2)}%
          </Text>
          {getMarginTag(margin)}
        </div>
      ),
    },
  ];

  // Model profitability table columns
  const modelColumns = [
    {
      title: '模型名称',
      dataIndex: 'model_name',
      key: 'model_name',
      ellipsis: true,
    },
    {
      title: '请求数',
      dataIndex: 'total_requests',
      key: 'total_requests',
      sorter: (a, b) => a.total_requests - b.total_requests,
      render: (count) => count?.toLocaleString() || 0,
    },
    {
      title: '收入',
      dataIndex: 'revenue_usd',
      key: 'revenue_usd',
      sorter: (a, b) => a.revenue_usd - b.revenue_usd,
      render: (revenue) => formatUSD(revenue),
    },
    {
      title: '成本',
      dataIndex: 'cost_usd',
      key: 'cost_usd',
      sorter: (a, b) => a.cost_usd - b.cost_usd,
      render: (cost) => formatUSD(cost),
    },
    {
      title: '利润',
      dataIndex: 'profit_usd',
      key: 'profit_usd',
      sorter: (a, b) => a.profit_usd - b.profit_usd,
      render: (profit) => (
        <Text style={{ color: profit >= 0 ? '#52c41a' : '#ff4d4f' }}>
          {formatUSD(profit)}
        </Text>
      ),
    },
    {
      title: '利润率',
      dataIndex: 'profit_margin',
      key: 'profit_margin',
      sorter: (a, b) => a.profit_margin - b.profit_margin,
      render: (margin) => (
        <Text style={{ color: getMarginColor(margin) }}>
          {margin?.toFixed(2)}%
        </Text>
      ),
    },
  ];

  return (
    <div>
      {/* Data Quality Warning */}
      {data_quality?.has_warning && (
        <Banner
          type="warning"
          title="数据质量警告"
          description={data_quality.warning_message}
          closeIcon
          style={{ marginBottom: 16 }}
        />
      )}

      {/* Warnings */}
      {warnings && warnings.length > 0 && (
        <Banner
          type="danger"
          title={`发现 ${warnings.length} 个警告`}
          description={
            <div>
              {warnings.slice(0, 3).map((warning, idx) => (
                <div key={idx} style={{ marginTop: idx > 0 ? 8 : 0 }}>
                  <IconAlertTriangle /> {warning.description}
                </div>
              ))}
              {warnings.length > 3 && (
                <Text type="tertiary">
                  ... 还有 {warnings.length - 3} 个警告
                </Text>
              )}
            </div>
          }
          closeIcon
          style={{ marginBottom: 16 }}
        />
      )}

      {/* Summary Cards */}
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col span={6}>
          <Card>
            <Statistic
              title="总收入"
              value={formatUSD(summary?.total_revenue_usd)}
              prefix={<IconMoneyExchangeStroked style={{ color: '#52c41a' }} />}
              valueColor="#52c41a"
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="总成本"
              value={formatUSD(summary?.total_cost_usd)}
              prefix={<IconMoneyExchangeStroked style={{ color: '#ff4d4f' }} />}
              valueColor="#ff4d4f"
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="总利润"
              value={formatUSD(summary?.total_profit_usd)}
              prefix={<IconMoneyExchangeStroked />}
              valueColor={
                summary?.total_profit_usd >= 0 ? '#52c41a' : '#ff4d4f'
              }
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="整体利润率"
              value={`${summary?.overall_margin?.toFixed(2)}%`}
              prefix={
                summary?.overall_margin >= 0 ? (
                  <IconTickCircle style={{ color: '#52c41a' }} />
                ) : (
                  <IconAlertTriangle style={{ color: '#ff4d4f' }} />
                )
              }
              valueColor={getMarginColor(summary?.overall_margin)}
            />
          </Card>
        </Col>
      </Row>

      {/* Channel Cost Breakdown */}
      <Card
        title={
          <Title heading={5} style={{ margin: 0 }}>
            渠道成本分析
          </Title>
        }
        style={{ marginBottom: 24 }}
      >
        <Table
          columns={channelColumns}
          dataSource={channels}
          pagination={{ pageSize: 10 }}
          rowKey="channel_id"
        />
      </Card>

      {/* Model Profitability */}
      {modelProfitabilityData && modelProfitabilityData.length > 0 && (
        <Card
          title={
            <Title heading={5} style={{ margin: 0 }}>
              模型盈利分析
            </Title>
          }
        >
          <Table
            columns={modelColumns}
            dataSource={modelProfitabilityData}
            pagination={{ pageSize: 10 }}
            rowKey="model_name"
          />
        </Card>
      )}

      {/* Data Quality Info */}
      {data_quality && (
        <Card title="数据质量" style={{ marginTop: 24 }}>
          <Row gutter={16}>
            <Col span={8}>
              <Text type="tertiary">总日志数:</Text>
              <Text strong style={{ marginLeft: 8 }}>
                {data_quality.total_logs?.toLocaleString()}
              </Text>
            </Col>
            <Col span={8}>
              <Text type="tertiary">包含定价数据:</Text>
              <Text strong style={{ marginLeft: 8 }}>
                {data_quality.logs_with_pricing?.toLocaleString()}
              </Text>
            </Col>
            <Col span={8}>
              <Text type="tertiary">覆盖率:</Text>
              <Text
                strong
                style={{
                  marginLeft: 8,
                  color: data_quality.coverage_percent >= 90 ? '#52c41a' : '#faad14',
                }}
              >
                {data_quality.coverage_percent?.toFixed(1)}%
              </Text>
            </Col>
          </Row>
        </Card>
      )}
    </div>
  );
};

export default CostEfficiencyTab;
