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
  const [channelDailyData, setChannelDailyData] = useState([]);

  useEffect(() => {
    fetchChannelQuotaData();
  }, [timeRange, refreshVersion]);

  const fetchChannelQuotaData = async () => {
    setLoading(true);
    setError(null);

    try {
      // Fetch channel quota analysis data (using quota instead of model_price)
      const result = await AnalyticsAPI.fetchChannelQuotaAnalysis(timeRange);
      setData(result);
    } catch (err) {
      setError(err.message || 'Failed to load channel quota analysis');
      setData(null);
    } finally {
      setLoading(false);
    }

    // Fetch trend data separately
    try {
      const trendResult = await AnalyticsAPI.fetchQuotaTrend(timeRange);
      if (trendResult && trendResult.trends) {
        const chartData = [];
        trendResult.trends.forEach(item => {
          chartData.push(
            { date: item.date, type: t('总消费'), value: item.total_quota_usd || 0 },
            { date: item.date, type: t('平均消费'), value: item.avg_quota_usd || 0 },
            { date: item.date, type: t('请求数'), value: item.request_count || 0 }
          );
        });
        setTrendData(chartData);
      }
    } catch (trendErr) {
      console.error('Failed to load trend data:', trendErr);
      setTrendData([]);
    }

    // Fetch channel daily quota trend data
    try {
      const channelDailyResult = await AnalyticsAPI.fetchChannelDailyQuotaTrend(timeRange);
      if (channelDailyResult && channelDailyResult.trends) {
        setChannelDailyData(channelDailyResult.trends);
      }
    } catch (channelDailyErr) {
      console.error('Failed to load channel daily data:', channelDailyErr);
      setChannelDailyData([]);
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

  const { channels, summary, data_quality } = data;

  // Channel quota table columns
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
      title: t('总消费额度'),
      dataIndex: 'total_quota_usd',
      key: 'total_quota_usd',
      sorter: (a, b) => a.total_quota_usd - b.total_quota_usd,
      render: (quota) => (
        <Text strong style={{ color: '#1890ff' }}>
          {formatUSDAmount(quota)}
        </Text>
      ),
    },
    {
      title: t('平均消费'),
      dataIndex: 'avg_quota_usd',
      key: 'avg_quota_usd',
      sorter: (a, b) => a.avg_quota_usd - b.avg_quota_usd,
      render: (avgQuota) => (
        <Text style={{ color: '#52c41a' }}>
          {formatUSDAmount(avgQuota)}
        </Text>
      ),
    },
    {
      title: t('总Tokens'),
      dataIndex: 'total_tokens',
      key: 'total_tokens',
      sorter: (a, b) => a.total_tokens - b.total_tokens,
      render: (tokens) => (
        <Text>{(tokens / 1000).toFixed(1)}K</Text>
      ),
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

      {/* Summary Cards */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title={t('总请求数')}
              value={summary.total_requests.toLocaleString()}
              valueColor='#1890ff'
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title={t('总消费额度')}
              value={formatUSDAmount(summary.total_quota_usd)}
              valueColor='#52c41a'
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title={t('平均消费')}
              value={formatUSDAmount(summary.avg_quota_usd)}
              valueColor='#faad14'
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card>
            <Statistic
              title={t('数据完整度')}
              value={`${data_quality.coverage_percent.toFixed(1)}%`}
              valueColor='#52c41a'
              prefix={<IconTick />}
            />
          </Card>
        </Col>
      </Row>

      {/* Quota Trend Chart */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col span={24}>
          <Card title={t('消费趋势分析')}>
            {trendData.length === 0 ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={t('趋势数据暂时无法加载')}
                style={{ padding: '40px 0' }}
              />
            ) : (
              <VChart
                spec={{
                  type: 'common',
                  data: [
                    {
                      id: 'trendData',
                      values: trendData.filter(d => d.type !== t('请求数'))
                    },
                    {
                      id: 'requestData',
                      values: trendData.filter(d => d.type === t('请求数'))
                    }
                  ],
                  series: [
                    {
                      type: 'line',
                      id: 'quotaSeries',
                      dataId: 'trendData',
                      xField: 'date',
                      yField: 'value',
                      seriesField: 'type',
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
                    },
                    {
                      type: 'line',
                      id: 'requestSeries',
                      dataId: 'requestData',
                      xField: 'date',
                      yField: 'value',
                      seriesField: 'type',
                      line: {
                        style: {
                          lineWidth: 2,
                          lineDash: [4, 4],
                        },
                      },
                      point: {
                        style: {
                          size: 3,
                        },
                      },
                    }
                  ],
                  color: {
                    type: 'ordinal',
                    domain: [t('总消费'), t('平均消费'), t('请求数')],
                    range: ['#1890ff', '#52c41a', '#faad14'],
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
                      seriesId: ['quotaSeries'],
                      label: {
                        formatMethod: (v) => `$${Number(v).toFixed(2)}`,
                      },
                      title: {
                        visible: true,
                        text: t('消费金额 (USD)'),
                        style: {
                          fontSize: 12,
                        },
                      },
                    },
                    {
                      orient: 'right',
                      seriesId: ['requestSeries'],
                      label: {
                        formatMethod: (v) => Number(v).toLocaleString(),
                      },
                      title: {
                        visible: true,
                        text: t('请求数'),
                        style: {
                          fontSize: 12,
                        },
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
                          value: (datum) => {
                            if (datum.type === t('请求数')) {
                              return Number(datum.value).toLocaleString();
                            }
                            return `$${Number(datum.value).toFixed(4)}`;
                          },
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

      {/* Channel Quota Table */}
      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card title={t('渠道消费详情')}>
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

      {/* Channel Daily Quota Bar Chart */}
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col span={24}>
          <Card title={t('渠道每日消费额度')}>
            {channelDailyData.length === 0 ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={t('渠道每日数据暂时无法加载')}
                style={{ padding: '40px 0' }}
              />
            ) : (
              <VChart
                spec={{
                  type: 'bar',
                  data: [
                    {
                      id: 'channelDailyData',
                      values: channelDailyData.map(d => ({
                        date: d.date,
                        channel: d.channel_name,
                        value: d.total_quota_usd,
                        requests: d.request_count,
                      }))
                    }
                  ],
                  xField: 'date',
                  yField: 'value',
                  seriesField: 'channel',
                  stack: false,
                  bar: {
                    style: {
                      cornerRadius: 4,
                    },
                  },
                  legends: [
                    {
                      visible: true,
                      orient: 'top',
                      position: 'start',
                    },
                  ],
                  axes: [
                    {
                      orient: 'left',
                      label: {
                        formatMethod: (v) => `$${Number(v).toFixed(2)}`,
                      },
                      title: {
                        visible: true,
                        text: t('消费额度 (USD)'),
                        style: {
                          fontSize: 12,
                        },
                      },
                    },
                    {
                      orient: 'bottom',
                      label: {
                        autoRotate: true,
                        autoRotateAngle: [0, 45],
                      },
                      title: {
                        visible: true,
                        text: t('日期'),
                        style: {
                          fontSize: 12,
                        },
                      },
                    },
                  ],
                  tooltip: {
                    visible: true,
                    mark: {
                      title: {
                        key: t('日期'),
                        value: (datum) => datum.date,
                      },
                      content: [
                        {
                          key: t('渠道'),
                          value: (datum) => datum.channel,
                        },
                        {
                          key: t('消费额度'),
                          value: (datum) => `$${Number(datum.value).toFixed(4)}`,
                        },
                        {
                          key: t('请求数'),
                          value: (datum) => Number(datum.requests).toLocaleString(),
                        },
                      ],
                    },
                  },
                  padding: {
                    top: 20,
                    right: 20,
                    bottom: 40,
                    left: 60,
                  },
                }}
                style={{ width: '100%', height: '500px' }}
              />
            )}
          </Card>
        </Col>
      </Row>

      {/* Data Quality Info */}
      {data_quality && (
        <Card style={{ marginTop: 16 }}>
          <Text type='tertiary' style={{ fontSize: '12px' }}>
            {t('数据质量')}: {data_quality.logs_with_pricing} / {data_quality.total_logs} {t('条日志包含额度信息')}
            ({data_quality.coverage_percent.toFixed(1)}%)
          </Text>
        </Card>
      )}
    </div>
  );
};

export default ChannelCostTab;
