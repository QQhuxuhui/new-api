import React, { useState, useEffect, useMemo } from 'react';
import {
  Modal,
  Card,
  Table,
  Tag,
  Button,
  Space,
  DatePicker,
  Spin,
  Empty,
  Typography,
  RadioGroup,
  Radio,
} from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { useTokenAnalytics } from '../../../../hooks/tokens/useTokenAnalytics';
import { renderQuota, renderNumber } from '../../../../helpers';

const CHART_CONFIG = { mode: 'desktop-browser' };

const TokenAnalyticsModal = ({ visible, onClose, t }) => {
  const {
    data,
    loading,
    activeRange,
    customRange,
    selectPresetRange,
    selectCustomRange,
    initLoad,
  } = useTokenAnalytics();

  const [barMetric, setBarMetric] = useState('quota');
  const [pieMetric, setPieMetric] = useState('quota');
  const [trendMetric, setTrendMetric] = useState('quota');

  useEffect(() => {
    if (visible) {
      initLoad();
    }
  }, [visible, initLoad]);

  const presetButtons = [
    { key: 'today', label: t('今天') },
    { key: 'thisWeek', label: t('本周') },
    { key: 'thisMonth', label: t('本月') },
    { key: 'last7Days', label: t('最近 7 天') },
    { key: 'last30Days', label: t('最近 30 天') },
  ];

  // Build trend line chart spec
  const trendLineSpec = useMemo(() => {
    if (!data?.trends?.length) return null;

    const values = data.trends.map((item) => ({
      date: item.date,
      token: item.token_name || `Token #${item.token_id}`,
      value: trendMetric === 'quota' ? item.quota : item.request_count,
    }));

    return {
      type: 'line',
      data: [{ id: 'trend', values }],
      xField: 'date',
      yField: 'value',
      seriesField: 'token',
      point: { visible: true },
      title: {
        visible: true,
        text:
          trendMetric === 'quota'
            ? t('令牌消耗趋势（额度）')
            : t('令牌调用趋势（次数）'),
      },
      legends: { visible: true },
      tooltip: {
        mark: {
          content: [
            {
              key: (datum) => datum['token'],
              value: (datum) =>
                trendMetric === 'quota'
                  ? renderQuota(datum['value'], 2)
                  : renderNumber(datum['value']),
            },
          ],
        },
      },
    };
  }, [data, trendMetric, t]);

  // Build bar chart spec
  const barChartSpec = useMemo(() => {
    if (!data?.tokens?.length) return null;
    const sorted = [...data.tokens]
      .sort((a, b) => b[barMetric] - a[barMetric])
      .slice(0, 10);

    const values = sorted.map((tk) => ({
      name: tk.token_name || `Token #${tk.token_id}`,
      value: barMetric === 'quota' ? tk.quota : tk.request_count,
    }));

    return {
      type: 'bar',
      data: [{ id: 'bar', values }],
      xField: 'name',
      yField: 'value',
      title: {
        visible: true,
        text:
          barMetric === 'quota'
            ? t('令牌消耗排名（额度）')
            : t('令牌调用排名（次数）'),
      },
      label: { visible: true },
      tooltip: {
        mark: {
          content: [
            {
              key: (datum) => datum['name'],
              value: (datum) =>
                barMetric === 'quota'
                  ? renderQuota(datum['value'], 2)
                  : renderNumber(datum['value']),
            },
          ],
        },
      },
    };
  }, [data, barMetric, t]);

  // Build pie chart spec
  const pieChartSpec = useMemo(() => {
    if (!data?.tokens?.length) return null;

    const modelTotals = {};
    data.tokens.forEach((tk) => {
      if (tk.models) {
        Object.entries(tk.models).forEach(([modelName, stats]) => {
          if (!modelTotals[modelName]) {
            modelTotals[modelName] = { request_count: 0, quota: 0 };
          }
          modelTotals[modelName].request_count += stats.request_count;
          modelTotals[modelName].quota += stats.quota;
        });
      }
    });

    const values = Object.entries(modelTotals).map(([name, stats]) => ({
      type: name,
      value: pieMetric === 'quota' ? stats.quota : stats.request_count,
    }));

    return {
      type: 'pie',
      data: [{ id: 'pie', values }],
      outerRadius: 0.8,
      innerRadius: 0.5,
      padAngle: 0.6,
      valueField: 'value',
      categoryField: 'type',
      pie: {
        style: { cornerRadius: 10 },
        state: {
          hover: { outerRadius: 0.85, stroke: '#000', lineWidth: 1 },
        },
      },
      title: {
        visible: true,
        text:
          pieMetric === 'quota'
            ? t('模型消耗分布（额度）')
            : t('模型调用分布（次数）'),
      },
      legends: { visible: true, orient: 'left' },
      label: { visible: true },
      tooltip: {
        mark: {
          content: [
            {
              key: (datum) => datum['type'],
              value: (datum) =>
                pieMetric === 'quota'
                  ? renderQuota(datum['value'], 2)
                  : renderNumber(datum['value']),
            },
          ],
        },
      },
    };
  }, [data, pieMetric, t]);

  // Table columns
  const columns = useMemo(
    () => [
      {
        title: t('令牌名称'),
        dataIndex: 'token_name',
        render: (text, record) => text || `Token #${record.token_id}`,
      },
      {
        title: t('调用次数'),
        dataIndex: 'request_count',
        sorter: (a, b) => a.request_count - b.request_count,
        render: (val) => renderNumber(val),
      },
      {
        title: t('消耗额度'),
        dataIndex: 'quota',
        sorter: (a, b) => a.quota - b.quota,
        render: (val) => renderQuota(val, 2),
      },
      {
        title: 'Prompt Tokens',
        dataIndex: 'prompt_tokens',
        sorter: (a, b) => a.prompt_tokens - b.prompt_tokens,
        render: (val) => renderNumber(val),
      },
      {
        title: 'Completion Tokens',
        dataIndex: 'completion_tokens',
        sorter: (a, b) => a.completion_tokens - b.completion_tokens,
        render: (val) => renderNumber(val),
      },
      {
        title: t('最常用模型'),
        dataIndex: 'models',
        render: (models) => {
          if (!models) return '-';
          const sorted = Object.entries(models).sort(
            (a, b) => b[1].quota - a[1].quota
          );
          return sorted.length > 0 ? (
            <Tag size='small'>{sorted[0][0]}</Tag>
          ) : (
            '-'
          );
        },
      },
    ],
    [t]
  );

  // Expandable row for model breakdown
  const expandedRowRender = (record) => {
    if (!record.models || Object.keys(record.models).length === 0) {
      return <Empty description={t('暂无模型数据')} />;
    }
    const modelData = Object.entries(record.models).map(
      ([name, stats]) => ({
        model_name: name,
        request_count: stats.request_count,
        quota: stats.quota,
      })
    );
    const modelColumns = [
      { title: t('模型名称'), dataIndex: 'model_name' },
      {
        title: t('调用次数'),
        dataIndex: 'request_count',
        render: (val) => renderNumber(val),
      },
      {
        title: t('消耗额度'),
        dataIndex: 'quota',
        render: (val) => renderQuota(val, 2),
      },
    ];
    return (
      <Table
        columns={modelColumns}
        dataSource={modelData}
        rowKey='model_name'
        pagination={false}
        size='small'
      />
    );
  };

  const summary = data?.summary;

  return (
    <Modal
      title={t('令牌用量统计')}
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={960}
      style={{ maxWidth: '95vw' }}
      bodyStyle={{ maxHeight: '80vh', overflow: 'auto' }}
    >
      {/* Time Range Selector */}
      <div className='flex flex-wrap items-center gap-2 mb-4'>
        {presetButtons.map((btn) => (
          <Button
            key={btn.key}
            type={activeRange === btn.key ? 'primary' : 'tertiary'}
            size='small'
            onClick={() => selectPresetRange(btn.key)}
          >
            {btn.label}
          </Button>
        ))}
        <DatePicker
          type='dateRange'
          density='compact'
          value={customRange}
          onChange={selectCustomRange}
          style={{ width: 260 }}
        />
      </div>

      <Spin spinning={loading}>
        {!data || !data.tokens?.length ? (
          <Empty
            description={t('暂无数据')}
            style={{ padding: '60px 0' }}
          />
        ) : (
          <div className='flex flex-col gap-4'>
            {/* Summary Cards */}
            <div className='grid grid-cols-2 md:grid-cols-4 gap-3'>
              <Card bodyStyle={{ padding: '12px 16px' }}>
                <Typography.Text type='tertiary' size='small'>
                  {t('总调用次数')}
                </Typography.Text>
                <Typography.Title heading={4} style={{ margin: 0 }}>
                  {renderNumber(summary?.total_requests || 0)}
                </Typography.Title>
              </Card>
              <Card bodyStyle={{ padding: '12px 16px' }}>
                <Typography.Text type='tertiary' size='small'>
                  {t('总消耗额度')}
                </Typography.Text>
                <Typography.Title heading={4} style={{ margin: 0 }}>
                  {renderQuota(summary?.total_quota || 0, 2)}
                </Typography.Title>
              </Card>
              <Card bodyStyle={{ padding: '12px 16px' }}>
                <Typography.Text type='tertiary' size='small'>
                  {t('活跃令牌数')}
                </Typography.Text>
                <Typography.Title heading={4} style={{ margin: 0 }}>
                  {summary?.active_tokens || 0}
                </Typography.Title>
              </Card>
              <Card bodyStyle={{ padding: '12px 16px' }}>
                <Typography.Text type='tertiary' size='small'>
                  {t('总 Token 数')}
                </Typography.Text>
                <Typography.Title heading={4} style={{ margin: 0 }}>
                  {renderNumber(
                    data.tokens.reduce(
                      (sum, tk) =>
                        sum + (tk.prompt_tokens || 0) + (tk.completion_tokens || 0),
                      0
                    )
                  )}
                </Typography.Title>
              </Card>
            </div>

            {/* Time Trend Line Chart */}
            {trendLineSpec && (
              <Card
                title={
                  <div className='flex items-center justify-between w-full'>
                    <span>{t('时间趋势')}</span>
                    <RadioGroup
                      type='button'
                      size='small'
                      value={trendMetric}
                      onChange={(e) => setTrendMetric(e.target.value)}
                    >
                      <Radio value='quota'>{t('消耗额度')}</Radio>
                      <Radio value='request_count'>{t('调用次数')}</Radio>
                    </RadioGroup>
                  </div>
                }
              >
                <div style={{ height: 300 }}>
                  <VChart spec={trendLineSpec} option={CHART_CONFIG} />
                </div>
              </Card>
            )}

            {/* Bar Chart - Token Ranking */}
            {barChartSpec && (
              <Card
                title={
                  <div className='flex items-center justify-between w-full'>
                    <span>{t('令牌对比排名')}</span>
                    <RadioGroup
                      type='button'
                      size='small'
                      value={barMetric}
                      onChange={(e) => setBarMetric(e.target.value)}
                    >
                      <Radio value='quota'>{t('消耗额度')}</Radio>
                      <Radio value='request_count'>{t('调用次数')}</Radio>
                    </RadioGroup>
                  </div>
                }
              >
                <div style={{ height: 300 }}>
                  <VChart spec={barChartSpec} option={CHART_CONFIG} />
                </div>
              </Card>
            )}

            {/* Token Overview Table */}
            <Card title={t('令牌概览')}>
              <Table
                columns={columns}
                dataSource={data.tokens}
                rowKey='token_id'
                expandedRowRender={expandedRowRender}
                pagination={
                  data.tokens.length > 10
                    ? { pageSize: 10, showSizeChanger: true }
                    : false
                }
                size='small'
              />
            </Card>

            {/* Pie Chart - Model Distribution */}
            {pieChartSpec && (
              <Card
                title={
                  <div className='flex items-center justify-between w-full'>
                    <span>{t('模型分布')}</span>
                    <RadioGroup
                      type='button'
                      size='small'
                      value={pieMetric}
                      onChange={(e) => setPieMetric(e.target.value)}
                    >
                      <Radio value='quota'>{t('消耗额度')}</Radio>
                      <Radio value='request_count'>{t('调用次数')}</Radio>
                    </RadioGroup>
                  </div>
                }
              >
                <div style={{ height: 350 }}>
                  <VChart spec={pieChartSpec} option={CHART_CONFIG} />
                </div>
              </Card>
            )}
          </div>
        )}
      </Spin>
    </Modal>
  );
};

export default TokenAnalyticsModal;
