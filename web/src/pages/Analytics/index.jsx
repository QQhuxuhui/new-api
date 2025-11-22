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

import React, { useState, lazy, Suspense } from 'react';
import {
  Card,
  Tabs,
  TabPane,
  Select,
  Button,
  Spin,
  Typography,
  Row,
  Col,
  Statistic,
  Table,
  Tag,
  Dropdown,
  Space,
  Empty,
} from '@douyinfe/semi-ui';
import {
  IconRefresh,
  IconDownload,
  IconUser,
  IconPulse,
  IconPriceTag,
  IconServer,
  IconAlertTriangle,
} from '@douyinfe/semi-icons';
import { useAnalyticsData } from '../../hooks/analytics/useAnalyticsData';

const { Title, Text } = Typography;

const Analytics = () => {
  const {
    timeRange,
    loading,
    error,
    userOverview,
    activeUsers,
    consumptionTrend,
    topSpenders,
    modelUsage,
    behaviorPatterns,
    riskIndicators,
    setTimeRange,
    refreshData,
    exportData,
  } = useAnalyticsData('7d');

  const [activeTab, setActiveTab] = useState('overview');

  const timeRangeOptions = [
    { value: '1d', label: '最近24小时' },
    { value: '7d', label: '最近7天' },
    { value: '30d', label: '最近30天' },
    { value: '90d', label: '最近90天' },
  ];

  const exportMenuItems = [
    { node: 'item', name: '导出活跃用户 (JSON)', onClick: () => exportData('active_users', 'json') },
    { node: 'item', name: '导出活跃用户 (CSV)', onClick: () => exportData('active_users', 'csv') },
    { node: 'item', name: '导出消费趋势 (JSON)', onClick: () => exportData('consumption_trend', 'json') },
    { node: 'item', name: '导出消费趋势 (CSV)', onClick: () => exportData('consumption_trend', 'csv') },
    { node: 'item', name: '导出模型使用 (JSON)', onClick: () => exportData('models', 'json') },
    { node: 'item', name: '导出模型使用 (CSV)', onClick: () => exportData('models', 'csv') },
  ];

  // Overview cards
  const renderOverviewCards = () => {
    if (!userOverview) {
      return <Empty description="暂无数据" />;
    }

    const growthColor = userOverview.growth_rate >= 0 ? '#52c41a' : '#ff4d4f';
    const growthIcon = userOverview.growth_rate >= 0 ? '↑' : '↓';

    return (
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="总用户数"
              value={userOverview.total_users}
              prefix={<IconUser />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="今日活跃"
              value={userOverview.active_users_today}
              prefix={<IconPulse />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="7日活跃"
              value={userOverview.active_users_7d}
              suffix={
                <Text style={{ fontSize: 14, color: growthColor }}>
                  {growthIcon} {Math.abs(userOverview.growth_rate || 0).toFixed(1)}%
                </Text>
              }
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title="30日活跃"
              value={userOverview.active_users_30d}
            />
          </Card>
        </Col>
      </Row>
    );
  };

  // Active users table
  const renderActiveUsers = () => {
    const columns = [
      {
        title: '排名',
        dataIndex: 'rank',
        width: 80,
        render: (_, __, index) => (
          <Tag color={index < 3 ? 'orange' : 'grey'}>{index + 1}</Tag>
        ),
      },
      {
        title: '用户名',
        dataIndex: 'username',
        render: (text, record) => text || `用户 ${record.user_id}`,
      },
      {
        title: '请求数',
        dataIndex: 'request_count',
        sorter: (a, b) => a.request_count - b.request_count,
      },
      {
        title: '最后活跃',
        dataIndex: 'last_active_at',
        render: (timestamp) => {
          const date = new Date(timestamp * 1000);
          return date.toLocaleString('zh-CN');
        },
      },
    ];

    return (
      <Card title="活跃用户排行">
        <Table
          columns={columns}
          dataSource={activeUsers}
          pagination={{ pageSize: 10 }}
          loading={loading}
          rowKey="user_id"
        />
      </Card>
    );
  };

  // Top spenders table
  const renderTopSpenders = () => {
    const columns = [
      {
        title: '排名',
        dataIndex: 'rank',
        width: 80,
        render: (_, __, index) => (
          <Tag color={index < 3 ? 'green' : 'grey'}>{index + 1}</Tag>
        ),
      },
      {
        title: '用户名',
        dataIndex: 'username',
        render: (text, record) => text || `用户 ${record.user_id}`,
      },
      {
        title: '消费额度',
        dataIndex: 'total_quota',
        sorter: (a, b) => a.total_quota - b.total_quota,
        render: (value) => value.toLocaleString(),
      },
      {
        title: '请求数',
        dataIndex: 'request_count',
      },
    ];

    return (
      <Card title="消费排行">
        <Table
          columns={columns}
          dataSource={topSpenders}
          pagination={{ pageSize: 10 }}
          loading={loading}
          rowKey="user_id"
        />
      </Card>
    );
  };

  // Model usage table
  const renderModelUsage = () => {
    const columns = [
      {
        title: '模型名称',
        dataIndex: 'model_name',
        width: 200,
      },
      {
        title: '请求数',
        dataIndex: 'request_count',
        sorter: (a, b) => a.request_count - b.request_count,
      },
      {
        title: '独立用户',
        dataIndex: 'unique_users',
      },
      {
        title: '平均Token',
        dataIndex: 'avg_tokens',
      },
      {
        title: '成功率',
        dataIndex: 'success_rate',
        render: (value) => (
          <Tag color={value >= 95 ? 'green' : value >= 80 ? 'orange' : 'red'}>
            {value.toFixed(1)}%
          </Tag>
        ),
      },
    ];

    return (
      <Card title="模型使用统计">
        <Table
          columns={columns}
          dataSource={modelUsage}
          pagination={{ pageSize: 10 }}
          loading={loading}
          rowKey="model_name"
        />
      </Card>
    );
  };

  // Risk indicators
  const renderRiskIndicators = () => {
    const columns = [
      {
        title: '类型',
        dataIndex: 'type',
        width: 120,
        render: (type) => {
          const typeMap = {
            high_frequency: { text: '高频访问', color: 'orange' },
            high_error: { text: '高错误率', color: 'red' },
            low_balance: { text: '低余额', color: 'yellow' },
            spike: { text: '异常波动', color: 'red' },
          };
          const item = typeMap[type] || { text: type, color: 'grey' };
          return <Tag color={item.color}>{item.text}</Tag>;
        },
      },
      {
        title: '严重程度',
        dataIndex: 'severity',
        width: 100,
        render: (severity) => {
          const colorMap = { low: 'blue', medium: 'orange', high: 'red' };
          return <Tag color={colorMap[severity]}>{severity}</Tag>;
        },
      },
      {
        title: '用户',
        dataIndex: 'username',
        render: (text, record) => text || (record.user_id ? `用户 ${record.user_id}` : '-'),
      },
      {
        title: '描述',
        dataIndex: 'description',
      },
    ];

    return (
      <Card title="风险监控">
        {riskIndicators.length === 0 ? (
          <Empty description="暂无风险提示" />
        ) : (
          <Table
            columns={columns}
            dataSource={riskIndicators}
            pagination={{ pageSize: 10 }}
            loading={loading}
            rowKey={(record) => `${record.type}_${record.user_id}`}
          />
        )}
      </Card>
    );
  };

  // Consumption trend simple display
  const renderConsumptionTrend = () => {
    if (!consumptionTrend || consumptionTrend.length === 0) {
      return (
        <Card title="消费趋势">
          <Empty description="暂无数据" />
        </Card>
      );
    }

    const columns = [
      {
        title: '日期',
        dataIndex: 'date',
      },
      {
        title: '总额度',
        dataIndex: 'total_quota',
        render: (value) => value.toLocaleString(),
      },
      {
        title: '请求数',
        dataIndex: 'request_count',
        render: (value) => value.toLocaleString(),
      },
      {
        title: '活跃用户',
        dataIndex: 'user_count',
      },
      {
        title: 'ARPU',
        dataIndex: 'arpu',
        render: (value) => value.toFixed(2),
      },
    ];

    return (
      <Card title="消费趋势">
        <Table
          columns={columns}
          dataSource={consumptionTrend}
          pagination={{ pageSize: 10 }}
          loading={loading}
          rowKey="date"
        />
      </Card>
    );
  };

  return (
    <div className="mt-[60px] px-4">
      <div className="mb-4 flex justify-between items-center">
        <Title heading={3}>
          <IconPulse className="mr-2" />
          用户行为分析
        </Title>
        <Space>
          <Select
            value={timeRange}
            onChange={setTimeRange}
            optionList={timeRangeOptions}
            style={{ width: 150 }}
          />
          <Button
            icon={<IconRefresh />}
            onClick={refreshData}
            loading={loading}
          >
            刷新
          </Button>
          <Dropdown menu={exportMenuItems} trigger="click">
            <Button icon={<IconDownload />}>导出</Button>
          </Dropdown>
        </Space>
      </div>

      {error && (
        <Card style={{ marginBottom: 16 }}>
          <Text type="danger">{error}</Text>
        </Card>
      )}

      <Spin spinning={loading}>
        <Tabs activeKey={activeTab} onChange={setActiveTab}>
          <TabPane
            tab={<span><IconUser className="mr-1" />概览</span>}
            itemKey="overview"
          >
            <div className="space-y-4">
              {renderOverviewCards()}
              <Row gutter={16}>
                <Col xs={24} lg={12}>
                  {renderActiveUsers()}
                </Col>
                <Col xs={24} lg={12}>
                  {renderTopSpenders()}
                </Col>
              </Row>
            </div>
          </TabPane>

          <TabPane
            tab={<span><IconPriceTag className="mr-1" />消费分析</span>}
            itemKey="consumption"
          >
            {renderConsumptionTrend()}
          </TabPane>

          <TabPane
            tab={<span><IconServer className="mr-1" />模型使用</span>}
            itemKey="models"
          >
            {renderModelUsage()}
          </TabPane>

          <TabPane
            tab={<span><IconAlertTriangle className="mr-1" />风险监控</span>}
            itemKey="risks"
          >
            {renderRiskIndicators()}
          </TabPane>
        </Tabs>
      </Spin>
    </div>
  );
};

export default Analytics;
