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
  Card,
  Row,
  Col,
  Table,
  Tag,
  Typography,
  Empty,
  Spin,
} from '@douyinfe/semi-ui';
import {
  IconDollarStroked,
  IconUserCircle,
  IconAlertTriangle,
} from '@douyinfe/semi-icons';
import { AnalyticsAPI } from '../../../services/analyticsApi';
import BalanceDistributionChart from '../../../components/charts/BalanceDistributionChart';
import { formatUSDAmount } from '../../../utils/currency';

const { Text } = Typography;

// Custom Statistic component
const Statistic = ({ title, value, prefix, suffix, valueColor }) => (
  <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
    <Text type='tertiary' style={{ fontSize: '14px' }}>
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

const BalanceAnalysisTab = ({ timeRange }) => {
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchBalanceData();
  }, [timeRange]);

  const fetchBalanceData = async () => {
    try {
      setLoading(true);
      setError(null);
      const result = await AnalyticsAPI.fetchUserBalanceAnalysis(
        timeRange,
        20
      );
      setData(result);
    } catch (err) {
      setError(err.message || 'Failed to load balance analysis');
    } finally {
      setLoading(false);
    }
  };

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
        <Spin size='large' />
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

  if (!data) {
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description='No balance data available'
        style={{ marginTop: 40 }}
      />
    );
  }

  const { overview, distribution, rankings } = data;

  // Balance ranking table columns
  const rankingColumns = [
    {
      title: 'Rank',
      dataIndex: 'rank',
      key: 'rank',
      width: 80,
      render: (text, record, index) => {
        const rank = index + 1;
        if (rank === 1)
          return <Tag color='yellow' size='large'>🥇 {rank}</Tag>;
        if (rank === 2)
          return <Tag color='grey' size='large'>🥈 {rank}</Tag>;
        if (rank === 3)
          return <Tag color='orange' size='large'>🥉 {rank}</Tag>;
        return <Text>{rank}</Text>;
      },
    },
    {
      title: 'Username',
      dataIndex: 'username',
      key: 'username',
      ellipsis: true,
    },
    {
      title: 'Balance',
      dataIndex: 'balance_usd',
      key: 'balance_usd',
      sorter: (a, b) => a.balance_usd - b.balance_usd,
      render: (balance) => {
        const color = balance < 5 ? '#ff4d4f' : balance < 20 ? '#faad14' : '#52c41a';
        return (
          <Text strong style={{ color }}>
            {formatUSDAmount(balance)}
          </Text>
        );
      },
    },
    {
      title: 'Status',
      dataIndex: 'balance_usd',
      key: 'status',
      width: 120,
      render: (balance) => {
        if (balance < 5)
          return (
            <Tag color='red' size='small'>
              <IconAlertTriangle /> Low
            </Tag>
          );
        if (balance < 20)
          return (
            <Tag color='orange' size='small'>
              Warning
            </Tag>
          );
        return (
          <Tag color='green' size='small'>
            Good
          </Tag>
        );
      },
    },
    {
      title: 'Last Activity',
      dataIndex: 'last_activity',
      key: 'last_activity',
      render: (timestamp) => {
        if (!timestamp || timestamp === 0) return <Text type='tertiary'>N/A</Text>;
        const date = new Date(timestamp * 1000);
        return <Text type='secondary'>{date.toLocaleString()}</Text>;
      },
    },
  ];

  return (
    <div style={{ marginTop: 16 }}>
      {/* Summary Cards */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title='Total Balance'
              value={formatUSDAmount(overview.total_balance_usd)}
              prefix={<IconDollarStroked />}
              valueColor='#52c41a'
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title='Average Balance'
              value={formatUSDAmount(overview.average_balance_usd)}
              prefix={<IconUserCircle />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title='Median Balance'
              value={formatUSDAmount(overview.median_balance_usd)}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title='Low Balance Users'
              value={overview.low_balance_count}
              prefix={<IconAlertTriangle />}
              valueColor='#ff4d4f'
              suffix={
                <Text type='tertiary' style={{ fontSize: 14 }}>
                  / {overview.user_count}
                </Text>
              }
            />
          </Card>
        </Col>
      </Row>

      {/* Balance Distribution Chart */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col span={24}>
          <Card title='Balance Distribution'>
            <BalanceDistributionChart
              data={distribution}
              loading={loading}
              height={400}
            />
          </Card>
        </Col>
      </Row>

      {/* Balance Rankings Table */}
      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card title='Top Users by Balance'>
            <Table
              columns={rankingColumns}
              dataSource={rankings}
              pagination={false}
              loading={loading}
              rowKey='user_id'
            />
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default BalanceAnalysisTab;
