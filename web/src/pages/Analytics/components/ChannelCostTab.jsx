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
import { useTranslation } from 'react-i18next';
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
} from '@douyinfe/semi-ui';
import {
  IconAlertTriangle,
  IconTick,
  IconServer,
} from '@douyinfe/semi-icons';
import { VChart } from '@visactor/react-vchart';
import { AnalyticsAPI } from '../../../services/analyticsApi';
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

const ChannelCostTab = ({ timeRange, refreshVersion }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);
  const [trendData, setTrendData] = useState([]);

  useEffect(() => {
    fetchChannelCostData();
  }, [timeRange, refreshVersion]);

  const fetchChannelCostData = async () => {
    setLoading(true);
    setError(null);

    try {
      // Fetch channel cost analysis data
      const result = await AnalyticsAPI.fetchChannelCostAnalysis(timeRange);
      setData(result);
    } catch (err) {
      setError(err.message || 'Failed to load channel cost analysis');
      setData(null);
    } finally {
      setLoading(false);
    }

    // Fetch trend data separately, don't fail the whole tab if this fails
    try {
      const trendResult = await AnalyticsAPI.fetchCostTrend(timeRange);
      if (trendResult && trendResult.trends) {
        const chartData = [];
        trendResult.trends.forEach(item => {
          chartData.push(
            { date: item.date, type: t('平台成本'), value: item.cost_usd || 0 },
            { date: item.date, type: t('用户收入'), value: item.revenue_usd || 0 },
            { date: item.date, type: t('利润'), value: item.profit_usd || 0 }
          );
        });
        setTrendData(chartData);
      }
    } catch (trendErr) {
      // Trend data failed, but we still show the main cost analysis
      console.error('Failed to load trend data:', trendErr);
      setTrendData([]);
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
        description={t('暂无渠道成本数据')}
        style={{ marginTop: 40 }}
      />
    );
  }

  const { channels, summary, data_quality, warnings } = data;

  // Channel cost table columns
  const channelColumns = [
    {
      title: t('排名'),
      key: 'rank',
      width: 80,
      render: (text, record, index) => {
        const rank = index + 1;
        if (rank === 1)
          return <Tag color='red' size='large'>🔥 {rank}</Tag>;
        if (rank === 2)
          return <Tag color='orange' size='large'>⚡ {rank}</Tag>;
        if (rank === 3)
          return <Tag color='yellow' size='large'>⭐ {rank}</Tag>;
        return <Text>{rank}</Text>;
      },
    },
    {
      title: t('渠道名称'),
      dataIndex: 'channel_name',
      key: 'channel_name',
      ellipsis: true,
      render: (text, record) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <IconServer />
          <Text strong>{text || `渠道 ${record.channel_id}`}</Text>
        </div>
      ),
    },
    {
      title: t('请求数'),
      dataIndex: 'total_requests',
      key: 'total_requests',
      sorter: (a, b) => a.total_requests - b.total_requests,
      render: (count) => <Text>{count.toLocaleString()}</Text>,
    },
    {
      title: t('平台成本'),
      dataIndex: 'cost_usd',
      key: 'cost_usd',
      sorter: (a, b) => a.cost_usd - b.cost_usd,
      render: (cost) => (
        <Text strong style={{ color: '#ff4d4f' }}>
          {formatUSDAmount(cost)}
        </Text>
      ),
    },
    {
      title: t('用户收入'),
      dataIndex: 'revenue_usd',
      key: 'revenue_usd',
      sorter: (a, b) => a.revenue_usd - b.revenue_usd,
      render: (revenue) => (
        <Text strong style={{ color: '#52c41a' }}>
          {formatUSDAmount(revenue)}
        </Text>
      ),
    },
    {
      title: t('利润'),
      dataIndex: 'profit_usd',
      key: 'profit_usd',
      sorter: (a, b) => a.profit_usd - b.profit_usd,
      render: (profit) => (
        <Text strong style={{ color: profit >= 0 ? '#1890ff' : '#ff4d4f' }}>
          {formatUSDAmount(profit)}
        </Text>
      ),
    },
    {
      title: t('利润率'),
      dataIndex: 'profit_margin',
      key: 'profit_margin',
      sorter: (a, b) => a.profit_margin - b.profit_margin,
      render: (margin) => {
        let color = '#52c41a';
        if (margin < 0) color = '#ff4d4f';
        else if (margin < 10) color = '#faad14';

        return (
          <Tag color={margin < 0 ? 'red' : margin < 10 ? 'orange' : 'green'}>
            {margin.toFixed(2)}%
          </Tag>
        );
      },
    },
  ];

  return (
    <div style={{ marginTop: 16 }}>
      {/* Data Quality Warning */}
      {data_quality && data_quality.has_warning && (
        <Banner
          type="warning"
          icon={<IconAlertTriangle />}
          description={data_quality.warning_message}
          style={{ marginBottom: 16 }}
          closeIcon={null}
        />
      )}

      {/* Cost Warnings */}
      {warnings && warnings.length > 0 && (
        <Card style={{ marginBottom: 16 }}>
          <Text strong style={{ fontSize: '16px', marginBottom: '12px', display: 'block' }}>
            <IconAlertTriangle style={{ marginRight: '8px' }} />
            {t('风险提示')}
          </Text>
          {warnings.map((warning, index) => (
            <Banner
              key={index}
              type={warning.severity === 'high' ? 'danger' : warning.severity === 'medium' ? 'warning' : 'info'}
              description={`${warning.channel_name || '系统'}: ${warning.description}`}
              style={{ marginBottom: '8px' }}
              closeIcon={null}
            />
          ))}
        </Card>
      )}

      {/* Summary Cards */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title={t('总平台成本')}
              value={formatUSDAmount(summary.total_cost_usd)}
              valueColor='#ff4d4f'
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title={t('总用户收入')}
              value={formatUSDAmount(summary.total_revenue_usd)}
              valueColor='#52c41a'
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title={t('总利润')}
              value={formatUSDAmount(summary.total_profit_usd)}
              valueColor={summary.total_profit_usd >= 0 ? '#1890ff' : '#ff4d4f'}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title={t('整体利润率')}
              value={`${summary.overall_margin.toFixed(2)}%`}
              valueColor={summary.overall_margin >= 0 ? '#52c41a' : '#ff4d4f'}
              prefix={summary.overall_margin >= 0 ? <IconTick /> : <IconAlertTriangle />}
            />
          </Card>
        </Col>
      </Row>

      {/* Cost Trend Chart */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col span={24}>
          <Card title={t('成本趋势分析')}>
            {trendData.length === 0 ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={t('趋势数据暂时无法加载')}
                style={{ padding: '40px 0' }}
              />
            ) : (
              <VChart
                spec={{
                  type: 'line',
                  data: [{ id: 'trendData', values: trendData }],
                  xField: 'date',
                  yField: 'value',
                  seriesField: 'type',
                  height: 400,
                  line: {
                    style: {
                      lineWidth: 2,
                    },
                  },
                  point: {
                    style: {
                      size: 3,
                    },
                  },
                  color: {
                    type: 'ordinal',
                    domain: [t('平台成本'), t('用户收入'), t('利润')],
                    range: ['#ff4d4f', '#52c41a', '#1890ff'],
                  },
                  legends: [
                    {
                      visible: true,
                      orient: 'top',
                    },
                  ],
                  axes: [
                    {
                      orient: 'left',
                      label: {
                        formatMethod: (v) => `$${Number(v).toFixed(2)}`,
                      },
                    },
                    {
                      orient: 'bottom',
                      type: 'band',
                    },
                  ],
                  tooltip: {
                    visible: true,
                    mark: {
                      content: [
                        {
                          key: (datum) => datum.type,
                          value: (datum) => `$${Number(datum.value).toFixed(4)}`,
                        },
                      ],
                    },
                  },
                }}
                style={{ width: '100%', height: '400px' }}
              />
            )}
          </Card>
        </Col>
      </Row>

      {/* Channel Cost Table */}
      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card title={t('渠道成本详情')}>
            <Table
              columns={channelColumns}
              dataSource={channels}
              pagination={{ pageSize: 10 }}
              loading={loading}
              rowKey='channel_id'
            />
          </Card>
        </Col>
      </Row>

      {/* Data Quality Info */}
      {data_quality && (
        <Card style={{ marginTop: 16 }}>
          <Text type='tertiary' style={{ fontSize: '12px' }}>
            {t('数据质量')}: {data_quality.logs_with_pricing} / {data_quality.total_logs} {t('条日志包含定价信息')}
            ({data_quality.coverage_percent.toFixed(1)}%)
          </Text>
        </Card>
      )}
    </div>
  );
};

export default ChannelCostTab;
