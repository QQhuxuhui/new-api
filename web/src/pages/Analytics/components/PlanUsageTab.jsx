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

import React, { useState, useEffect, useMemo } from 'react';
import {
  Card,
  Row,
  Col,
  Table,
  Tag,
  Typography,
  Empty,
  Spin,
  Select,
  Input,
  Space,
  Progress,
} from '@douyinfe/semi-ui';
import {
  IconSearch,
  IconBox,
  IconLock,
  IconAlertTriangle,
} from '@douyinfe/semi-icons';
import { VChart } from '@visactor/react-vchart';
import { usePlanUsageData } from '../../../hooks/analytics/usePlanUsageData';
import QuotaProgress from '../../../components/analytics/QuotaProgress';

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
        {typeof value === 'number' ? value.toLocaleString() : value || 0}
      </Text>
      {suffix && <span>{suffix}</span>}
    </div>
  </div>
);

const PlanUsageTab = ({ timeRange }) => {
  const {
    loading,
    overview,
    planList,
    distribution,
    ranking,
    filters,
    page,
    pageSize,
    fetchAllData,
    updateFilters,
    updatePage,
    updateTimeRange,
  } = usePlanUsageData();

  const [searchUserId, setSearchUserId] = useState('');
  const [selectedPlanType, setSelectedPlanType] = useState('');
  const [selectedStatus, setSelectedStatus] = useState('');

  // Fetch data on mount and when timeRange changes
  useEffect(() => {
    if (timeRange) {
      // updateTimeRange will handle both setting state and fetching all data with correct timeRange
      updateTimeRange(timeRange);
    } else {
      // Initial load without timeRange prop
      fetchAllData();
    }
  }, [timeRange, updateTimeRange, fetchAllData]);

  // Handle filter changes - call updateFilters which will trigger refetch
  const handleSearchChange = (value) => {
    setSearchUserId(value);
    const newFilters = {};
    if (value) newFilters.user_id = parseInt(value);
    if (selectedPlanType) newFilters.plan_type = selectedPlanType;
    if (selectedStatus) newFilters.status = selectedStatus;
    updateFilters(newFilters);
  };

  const handlePlanTypeChange = (value) => {
    setSelectedPlanType(value);
    const newFilters = {};
    if (searchUserId) newFilters.user_id = parseInt(searchUserId);
    if (value) newFilters.plan_type = value;
    if (selectedStatus) newFilters.status = selectedStatus;
    updateFilters(newFilters);
  };

  const handleStatusChange = (value) => {
    setSelectedStatus(value);
    const newFilters = {};
    if (searchUserId) newFilters.user_id = parseInt(searchUserId);
    if (selectedPlanType) newFilters.plan_type = selectedPlanType;
    if (value) newFilters.status = value;
    updateFilters(newFilters);
  };

  // Render overview cards
  const renderOverviewCards = () => {
    if (!overview) {
      return <Empty description="暂无数据" />;
    }

    return (
      <>
        <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
          <Col xs={24} sm={12} md={6}>
            <Card>
              <Statistic
                title="总套餐数"
                value={overview.total_plans}
                prefix={<IconBox />}
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card>
              <Statistic
                title="活跃套餐"
                value={overview.active_plans}
                valueColor="#52c41a"
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card>
              <Statistic
                title="即将过期"
                value={overview.expiring_plans}
                prefix={<IconAlertTriangle />}
                valueColor="#faad14"
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={6}>
            <Card>
              <Statistic
                title="已锁定"
                value={overview.locked_plans}
                prefix={<IconLock />}
                valueColor="#ff4d4f"
              />
            </Card>
          </Col>
        </Row>

        <Row gutter={[16, 16]}>
          <Col xs={24} sm={12} md={8}>
            <Card>
              <Statistic
                title="总分配额度 (USD)"
                value={`$${overview.total_allocated_usd?.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) || '0.00'}`}
                valueColor="#1890ff"
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={8}>
            <Card>
              <Statistic
                title="总使用额度 (USD)"
                value={`$${overview.total_used_usd?.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) || '0.00'}`}
                valueColor="#52c41a"
              />
            </Card>
          </Col>
          <Col xs={24} sm={12} md={8}>
            <Card>
              <Statistic
                title="平均使用率"
                value={`${overview.average_usage_rate?.toFixed(1) || '0.0'}%`}
                valueColor="#722ed1"
              />
            </Card>
          </Col>
        </Row>
      </>
    );
  };

  // Render plan type distribution chart
  const renderDistributionChart = () => {
    if (!distribution || distribution.length === 0) {
      return (
        <Card title="套餐类型分布">
          <Empty description="暂无数据" />
        </Card>
      );
    }

    const typeColorMap = {
      subscription: '#1890ff',
      consumption: '#52c41a',
      trial: '#faad14',
      enterprise: '#722ed1',
    };

    const typeNameMap = {
      subscription: '订阅套餐',
      consumption: '按量付费',
      trial: '试用套餐',
      enterprise: '企业套餐',
    };

    const chartData = distribution.map((item) => ({
      type: typeNameMap[item.plan_type] || item.plan_type,
      value: item.user_count,
      percentage: item.percentage,
      usd: item.total_usd,
      color: typeColorMap[item.plan_type] || '#999',
    }));

    const chartSpec = {
      type: 'pie',
      data: [
        {
          id: 'planType',
          values: chartData,
        },
      ],
      categoryField: 'type',
      valueField: 'value',
      innerRadius: 0.6,
      outerRadius: 0.9,
      pie: {
        style: {
          fill: (datum) => datum.color,
        },
        state: {
          hover: {
            outerRadius: 0.95,
            stroke: '#000',
            lineWidth: 1,
          },
        },
      },
      label: {
        visible: true,
        style: {
          fontSize: 12,
          fontWeight: 'bold',
        },
      },
      legends: {
        visible: true,
        orient: 'right',
        item: {
          shape: {
            style: {
              size: 12,
            },
          },
        },
      },
      tooltip: {
        visible: true,
        renderMode: 'canvas',
        mark: {
          title: {
            visible: true,
          },
          content: [
            {
              key: (datum) => '用户数',
              value: (datum) => datum.value,
            },
            {
              key: (datum) => '占比',
              value: (datum) => `${datum.percentage.toFixed(1)}%`,
            },
            {
              key: (datum) => '总额度',
              value: (datum) => `$${datum.usd.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`,
            },
          ],
        },
      },
      width: 400,
      height: 300,
    };

    return (
      <Card title="套餐类型分布">
        <VChart spec={chartSpec} />
      </Card>
    );
  };

  // Render health indicators
  const renderHealthIndicators = () => {
    if (!overview) {
      return null;
    }

    const totalPlans = overview.total_plans || 1;
    const activePercent = ((overview.active_plans || 0) / totalPlans) * 100;
    const expiringPercent = ((overview.expiring_plans || 0) / totalPlans) * 100;
    const lockedPercent = ((overview.locked_plans || 0) / totalPlans) * 100;

    return (
      <Card title="套餐健康状态">
        <div style={{ padding: '16px' }}>
          {/* Active Plans */}
          <div style={{ marginBottom: '24px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '8px' }}>
              <Text strong>活跃套餐</Text>
              <Text type="success">
                {overview.active_plans} ({activePercent.toFixed(1)}%)
              </Text>
            </div>
            <Progress
              percent={activePercent}
              stroke="#52c41a"
              showInfo={false}
              size="large"
            />
          </div>

          {/* Expiring Plans */}
          <div style={{ marginBottom: '24px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '8px' }}>
              <Text strong>即将过期</Text>
              <Text type="warning">
                {overview.expiring_plans} ({expiringPercent.toFixed(1)}%)
              </Text>
            </div>
            <Progress
              percent={expiringPercent}
              stroke="#faad14"
              showInfo={false}
              size="large"
            />
          </div>

          {/* Locked Plans */}
          <div>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '8px' }}>
              <Text strong>已锁定</Text>
              <Text type="danger">
                {overview.locked_plans} ({lockedPercent.toFixed(1)}%)
              </Text>
            </div>
            <Progress
              percent={lockedPercent}
              stroke="#ff4d4f"
              showInfo={false}
              size="large"
            />
          </div>
        </div>
      </Card>
    );
  };

  // Plan type options
  const planTypeOptions = [
    { value: '', label: '全部类型' },
    { value: 'subscription', label: '订阅套餐' },
    { value: 'consumption', label: '按量付费' },
    { value: 'trial', label: '试用套餐' },
    { value: 'enterprise', label: '企业套餐' },
  ];

  // Status options
  const statusOptions = [
    { value: '', label: '全部状态' },
    { value: 'active', label: '活跃' },
    { value: 'expiring', label: '即将过期' },
    { value: 'expired', label: '已过期' },
    { value: 'locked', label: '已锁定' },
  ];

  // Plan type tag map
  const getPlanTypeTag = (type) => {
    const typeMap = {
      subscription: { text: '订阅', color: 'blue' },
      consumption: { text: '按量', color: 'green' },
      trial: { text: '试用', color: 'orange' },
      enterprise: { text: '企业', color: 'purple' },
    };
    const item = typeMap[type] || { text: type, color: 'grey' };
    return <Tag color={item.color}>{item.text}</Tag>;
  };

  // Expiration display
  const renderExpiration = (expiresAt) => {
    if (!expiresAt || expiresAt === 0) {
      return <Tag color="green">永久</Tag>;
    }

    const now = Date.now();
    const expireTime = expiresAt;
    const daysRemaining = Math.ceil((expireTime - now) / (1000 * 60 * 60 * 24));

    if (daysRemaining < 0) {
      return <Tag color="red">已过期</Tag>;
    } else if (daysRemaining <= 3) {
      return (
        <Tag color="orange" icon={<IconAlertTriangle />}>
          {daysRemaining}天后
        </Tag>
      );
    } else {
      return <Text type="tertiary">{daysRemaining}天后</Text>;
    }
  };

  // Plan usage table columns
  const planColumns = [
    {
      title: '用户信息',
      dataIndex: 'username',
      key: 'user',
      render: (username, record) => (
        <div>
          <Text strong>{username || `用户 ${record.user_id}`}</Text>
          <br />
          <Text type="tertiary" style={{ fontSize: '12px' }}>
            ID: {record.user_id}
          </Text>
        </div>
      ),
    },
    {
      title: '套餐类型',
      dataIndex: 'plan_type',
      key: 'plan_type',
      render: (type, record) => (
        <div>
          {getPlanTypeTag(type)}
          <br />
          <Text type="tertiary" style={{ fontSize: '12px' }}>
            {record.plan_display_name || record.plan_name}
          </Text>
        </div>
      ),
    },
    {
      title: '额度状态',
      dataIndex: 'used_usd',
      key: 'quota_status',
      sorter: (a, b) => a.usage_rate - b.usage_rate,
      render: (_, record) => (
        <QuotaProgress
          usedUSD={record.used_usd}
          totalUSD={record.total_usd}
          requests={record.request_count}
        />
      ),
    },
    {
      title: '过期时间',
      dataIndex: 'expires_at',
      key: 'expiration',
      render: (expiresAt) => renderExpiration(expiresAt),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status, record) => {
        if (record.locked === 1) {
          return <Tag color="red" icon={<IconLock />}>已锁定</Tag>;
        }
        if (status === 1) {
          return <Tag color="green">活跃</Tag>;
        }
        if (status === 2) {
          return <Tag color="grey">已过期</Tag>;
        }
        return <Tag color="grey">禁用</Tag>;
      },
    },
  ];

  // Top 10 ranking table
  const renderRanking = () => {
    if (!ranking || ranking.length === 0) {
      return <Empty description="暂无数据" />;
    }

    const rankColumns = [
      {
        title: '排名',
        dataIndex: 'rank',
        key: 'rank',
        width: 80,
        render: (rank) => {
          if (rank === 1) return <span style={{ fontSize: '20px' }}>🥇</span>;
          if (rank === 2) return <span style={{ fontSize: '20px' }}>🥈</span>;
          if (rank === 3) return <span style={{ fontSize: '20px' }}>🥉</span>;
          return <Text strong>{rank}</Text>;
        },
      },
      {
        title: '套餐名称',
        dataIndex: 'plan_display_name',
        key: 'plan_name',
        render: (text, record) => text || record.plan_name,
      },
      {
        title: '总消费 (USD)',
        dataIndex: 'total_consumed_usd',
        key: 'consumed',
        render: (value, record) => (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
            <Text strong style={{ color: '#52c41a', fontSize: '16px' }}>
              ${value.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
            </Text>
            <Text type="tertiary" style={{ fontSize: '12px' }}>
              {record.user_count} 用户 · {record.request_count.toLocaleString()} 请求
            </Text>
          </div>
        ),
      },
    ];

    return (
      <Card title="套餐消费排行 TOP10">
        <Table
          columns={rankColumns}
          dataSource={ranking}
          pagination={false}
          rowKey="plan_id"
        />
      </Card>
    );
  };

  if (loading && !planList.items) {
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

  return (
    <div style={{ padding: '16px 0' }}>
      {/* Overview Cards */}
      {renderOverviewCards()}

      {/* Distribution Chart and Health Indicators */}
      <Row gutter={[16, 16]} style={{ marginTop: 16, marginBottom: 16 }}>
        <Col xs={24} md={12}>
          {renderDistributionChart()}
        </Col>
        <Col xs={24} md={12}>
          {renderHealthIndicators()}
        </Col>
      </Row>

      {/* Filters */}
      <Card title="筛选条件" style={{ marginBottom: 16 }}>
        <Space wrap>
          <Input
            prefix={<IconSearch />}
            placeholder="搜索用户ID"
            value={searchUserId}
            onChange={handleSearchChange}
            style={{ width: 200 }}
          />
          <Select
            placeholder="套餐类型"
            value={selectedPlanType}
            onChange={handlePlanTypeChange}
            optionList={planTypeOptions}
            style={{ width: 150 }}
          />
          <Select
            placeholder="状态"
            value={selectedStatus}
            onChange={handleStatusChange}
            optionList={statusOptions}
            style={{ width: 150 }}
          />
        </Space>
      </Card>

      {/* Plan Usage Table */}
      <Card title="套餐使用详情">
        <Table
          columns={planColumns}
          dataSource={planList.items}
          pagination={{
            currentPage: page,
            pageSize: pageSize,
            total: planList.total,
            onPageChange: updatePage,
            showSizeChanger: false,
          }}
          loading={loading}
          rowKey="user_plan_id"
        />
      </Card>

      {/* Ranking */}
      <div style={{ marginTop: 16 }}>{renderRanking()}</div>
    </div>
  );
};

export default PlanUsageTab;
