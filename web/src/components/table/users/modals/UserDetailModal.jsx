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
import { useTranslation } from 'react-i18next';
import {
  SideSheet,
  Card,
  Spin,
  Typography,
  Tag,
  Space,
  Tabs,
  TabPane,
  Select,
  Empty,
  Row,
  Col,
  Table,
  Progress,
} from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { IconClose, IconUser, IconCalendar } from '@douyinfe/semi-icons';
import { AnalyticsAPI } from '../../../../services/analyticsApi';
import { formatUSDAmount } from '../../../../utils/currency';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import { timestamp2string } from '../../../../helpers';
import { timestamp2string } from '../../../../helpers';

const { Text, Title } = Typography;

/**
 * UserDetailModal - Displays detailed consumption analytics for a user
 * Shows daily consumption trends, plan-wise usage, and model breakdown
 */
const UserDetailModal = ({ visible, user, onClose }) => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();

  const [loading, setLoading] = useState(false);
  const [data, setData] = useState(null);
  const [selectedDays, setSelectedDays] = useState(30);
  const [activeTab, setActiveTab] = useState('daily');
  const [topups, setTopups] = useState([]);
  const [planOrders, setPlanOrders] = useState([]);
  const [topupPagination, setTopupPagination] = useState({
    currentPage: 1,
    pageSize: 10,
    total: 0,
  });
  const [planOrderPagination, setPlanOrderPagination] = useState({
    currentPage: 1,
    pageSize: 10,
    total: 0,
  });
  const [topupLoading, setTopupLoading] = useState(false);
  const [planOrderLoading, setPlanOrderLoading] = useState(false);

  // Fetch consumption data
  useEffect(() => {
    if (visible && user?.id) {
      fetchData();
    }
  }, [visible, user?.id, selectedDays]);

  // reset pagination when user changes
  useEffect(() => {
    if (visible) {
      setTopupPagination((prev) => ({ ...prev, currentPage: 1, pageSize: 10, total: 0 }));
      setPlanOrderPagination((prev) => ({ ...prev, currentPage: 1, pageSize: 10, total: 0 }));
      setTopups([]);
      setPlanOrders([]);
    }
  }, [user?.id, visible]);

  // fetch records when records tab opens
  useEffect(() => {
    if (visible && user?.id && activeTab === 'records') {
      fetchTopups(1, topupPagination.pageSize);
      fetchPlanOrders(1, planOrderPagination.pageSize);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab, visible, user?.id]);

  // Fetch top-up records
  const fetchTopups = async (page = 1, pageSize = 10) => {
    if (!user?.id) return;
    setTopupLoading(true);
    try {
      const res = await AnalyticsAPI.fetchUserTopUps(user.id, page, pageSize);
      setTopups(res.items || []);
      setTopupPagination({
        currentPage: res.page || page,
        pageSize: res.page_size || pageSize,
        total: res.total || 0,
      });
    } catch (error) {
      console.error('Failed to fetch user topups:', error);
    } finally {
      setTopupLoading(false);
    }
  };

  // Fetch plan order records
  const fetchPlanOrders = async (page = 1, pageSize = 10) => {
    if (!user?.id) return;
    setPlanOrderLoading(true);
    try {
      const res = await AnalyticsAPI.fetchUserPlanOrders(user.id, page, pageSize);
      setPlanOrders(res.items || []);
      setPlanOrderPagination({
        currentPage: res.page || page,
        pageSize: res.page_size || pageSize,
        total: res.total || 0,
      });
    } catch (error) {
      console.error('Failed to fetch user plan orders:', error);
    } finally {
      setPlanOrderLoading(false);
    }
  };

  const fetchData = async () => {
    setLoading(true);
    try {
      const result = await AnalyticsAPI.fetchUserConsumptionDetail(
        user.id,
        selectedDays
      );
      setData(result);
    } catch (error) {
      console.error('Failed to fetch user consumption detail:', error);
    } finally {
      setLoading(false);
    }
  };

  // Daily consumption stacked bar chart (models stacked by day)
  const dailyChartSpec = useMemo(() => {
    if (!data?.daily_consumption || !Array.isArray(data.daily_consumption)) return null;

    // Transform data for stacked chart
    const chartData = [];
    data.daily_consumption.forEach((day) => {
      // Safely handle null or undefined models array
      if (day.models && Array.isArray(day.models)) {
        day.models.forEach((model) => {
          chartData.push({
            date: day.date,
            model: model.model_name,
            usd: model.usd,
            requests: model.request_count,
            percentage: model.percentage,
          });
        });
      }
    });

    // Return null if no data to display
    if (chartData.length === 0) return null;

    return {
      type: 'bar',
      data: [{ id: 'dailyData', values: chartData }],
      xField: 'date',
      yField: 'usd',
      seriesField: 'model',
      stack: true,
      bar: {
        style: {
          cornerRadius: [4, 4, 0, 0],
        },
      },
      axes: [
        {
          orient: 'bottom',
          type: 'band',
          label: {
            style: {
              fontSize: 10,
              angle: -45,
              textAlign: 'right',
            },
          },
        },
        {
          orient: 'left',
          type: 'linear',
          title: { visible: true, text: t('消耗金额 (USD)') },
          label: { formatMethod: (v) => `$${v.toFixed(2)}` },
          grid: {
            visible: true,
            style: { lineDash: [4, 4], stroke: '#e5e7eb' },
          },
        },
      ],
      tooltip: {
        dimension: {
          title: {
            value: (datum) => {
              const data = Array.isArray(datum) ? datum : (datum ? [datum] : []);
              return data.length > 0 ? `${t('日期')}: ${data[0]?.date || '-'}` : t('日期') + ': -';
            },
          },
          content: [
            {
              key: (datum) => datum['model'],
              value: (datum) => ({
                usd: datum['usd'] || 0,
                requests: datum['requests'] || 0,
              }),
            },
          ],
          updateContent: (array) => {
            // Sort by value in descending order
            array.sort((a, b) => {
              const aVal = typeof a.value === 'object' ? a.value.usd : a.value;
              const bVal = typeof b.value === 'object' ? b.value.usd : b.value;
              return bVal - aVal;
            });

            // Calculate total sum first and store original values
            let sum = 0;
            const processedData = [];

            for (let i = 0; i < array.length; i++) {
              const itemValue = typeof array[i].value === 'object' ? array[i].value : { usd: 0, requests: 0 };
              const usd = itemValue.usd || 0;
              const requests = itemValue.requests || 0;
              sum += usd;

              // Store original values for later formatting
              processedData.push({
                key: array[i].key,
                usd: usd,
                requests: requests,
              });
            }

            // Now format with correct percentages using original values
            const result = processedData.map(item => {
              const percentage = sum > 0 ? ((item.usd / sum) * 100).toFixed(1) : '0.0';
              return {
                key: item.key,
                value: `${formatUSDAmount(item.usd)} (${t('请求')}: ${item.requests.toLocaleString()}, ${t('占比')}: ${percentage}%)`,
              };
            });

            // Add total row at the beginning
            result.unshift({
              key: t('当日总额'),
              value: formatUSDAmount(sum),
            });

            return result;
          },
        },
      },
      legends: {
        visible: true,
        orient: 'top',
        maxRow: 2,
        item: { label: { style: { fontSize: 11 } } },
      },
      padding: { top: 60, right: 20, bottom: 80, left: 60 },
    };
  }, [data, t]);

  // Plan comparison chart
  const planChartSpec = useMemo(() => {
    if (!data?.plan_daily_consumption || !Array.isArray(data.plan_daily_consumption)) return null;

    const chartData = [];
    data.plan_daily_consumption.forEach((plan) => {
      // Safely handle null or undefined daily_data array
      if (plan.daily_data && Array.isArray(plan.daily_data)) {
        plan.daily_data.forEach((day) => {
          chartData.push({
            date: day.date,
            plan: plan.plan_name,
            usd: day.used_usd,
            limit: day.daily_limit_usd,
            percent: day.usage_percent,
          });
        });
      }
    });

    // Return null if no data to display
    if (chartData.length === 0) return null;

    return {
      type: 'bar',
      data: [{ id: 'planData', values: chartData }],
      xField: 'date',
      yField: 'usd',
      seriesField: 'plan',
      bar: {
        style: {
          cornerRadius: [4, 4, 0, 0],
        },
      },
      axes: [
        {
          orient: 'bottom',
          type: 'band',
          label: {
            style: {
              fontSize: 10,
              angle: -45,
              textAlign: 'right',
            },
          },
        },
        {
          orient: 'left',
          type: 'linear',
          title: { visible: true, text: t('消耗金额 (USD)') },
          label: { formatMethod: (v) => `$${v.toFixed(2)}` },
          grid: {
            visible: true,
            style: { lineDash: [4, 4], stroke: '#e5e7eb' },
          },
        },
      ],
      tooltip: {
        dimension: {
          title: {
            value: (datum) => {
              const data = Array.isArray(datum) ? datum : (datum ? [datum] : []);
              return data.length > 0 ? `${t('日期')}: ${data[0]?.date || '-'}` : t('日期') + ': -';
            },
          },
          content: [
            {
              key: (datum) => datum['plan'],
              value: (datum) => ({
                usd: datum['usd'] || 0,
                limit: datum['limit'] || 0,
                percent: datum['percent'] || 0,
              }),
            },
          ],
          updateContent: (array) => {
            // Sort by value in descending order
            array.sort((a, b) => {
              const aVal = typeof a.value === 'object' ? a.value.usd : a.value;
              const bVal = typeof b.value === 'object' ? b.value.usd : b.value;
              return bVal - aVal;
            });

            let sum = 0;
            // Process each item and calculate sum
            for (let i = 0; i < array.length; i++) {
              const itemValue = typeof array[i].value === 'object' ? array[i].value : { usd: 0, limit: 0, percent: 0 };
              const usd = itemValue.usd || 0;
              const limit = itemValue.limit || 0;
              const percent = itemValue.percent || 0;
              sum += usd;
              array[i].value = `${formatUSDAmount(usd)} (${t('限额')}: ${limit > 0 ? formatUSDAmount(limit) : t('无限制')}, ${t('使用率')}: ${percent > 0 ? percent.toFixed(1) + '%' : '-'})`;
            }

            // Add total row at the beginning
            array.unshift({
              key: t('当日总额'),
              value: formatUSDAmount(sum),
            });

            return array;
          },
        },
      },
      legends: {
        visible: true,
        orient: 'top',
        item: { label: { style: { fontSize: 11 } } },
      },
      padding: { top: 60, right: 20, bottom: 80, left: 60 },
    };
  }, [data, t]);

  // Model usage pie chart
  const modelPieSpec = useMemo(() => {
    if (!data?.model_summary || !Array.isArray(data.model_summary) || data.model_summary.length === 0) return null;

    return {
      type: 'pie',
      data: [
        {
          id: 'modelSummary',
          values: data.model_summary.map((m) => ({
            type: m.model_name,
            value: m.total_usd,
            percentage: m.percentage,
            requests: m.request_count,
          })),
        },
      ],
      categoryField: 'type',
      valueField: 'value',
      radius: 0.8,
      innerRadius: 0.5,
      label: {
        visible: true,
        style: { fontSize: 11 },
        formatMethod: (text, datum) => `${datum.type}\n${datum.percentage.toFixed(1)}%`,
      },
      tooltip: {
        mark: {
          title: { value: (d) => d.type },
          content: [
            { key: t('消耗金额'), value: (d) => formatUSDAmount(d.value) },
            { key: t('请求次数'), value: (d) => d.requests.toLocaleString() },
            { key: t('占比'), value: (d) => `${d.percentage.toFixed(2)}%` },
          ],
        },
      },
      legends: {
        visible: true,
        orient: 'right',
        item: { label: { style: { fontSize: 11 } } },
      },
      padding: { top: 20, right: 120, bottom: 20, left: 20 },
    };
  }, [data, t]);

  // Render statistics cards
  const renderStatsCards = () => {
    if (!data) return null;

    const stats = [
      {
        title: t('总消耗'),
        value: formatUSDAmount(data.stats.total_usd),
        color: '#1890ff',
      },
      {
        title: t('日均消耗'),
        value: formatUSDAmount(data.stats.avg_daily_usd),
        color: '#52c41a',
      },
      {
        title: t('峰值消耗'),
        value: formatUSDAmount(data.stats.peak_daily_usd),
        color: '#faad14',
      },
      {
        title: t('总请求数'),
        value: data.stats.total_requests.toLocaleString(),
        color: '#722ed1',
      },
    ];

    return (
      <Row gutter={16} style={{ marginBottom: 20 }}>
        {stats.map((stat, index) => (
          <Col key={index} span={isMobile ? 12 : 6}>
            <Card
              bodyStyle={{
                padding: '16px',
                textAlign: 'center',
              }}
            >
              <Text type="tertiary" style={{ fontSize: 12 }}>
                {stat.title}
              </Text>
              <div style={{ marginTop: 8 }}>
                <Text
                  strong
                  style={{ fontSize: 20, color: stat.color }}
                >
                  {stat.value}
                </Text>
              </div>
            </Card>
          </Col>
        ))}
      </Row>
    );
  };

  // Render model summary table
  const modelTableColumns = [
    {
      title: t('模型名称'),
      dataIndex: 'model_name',
      width: 200,
    },
    {
      title: t('消耗金额'),
      dataIndex: 'total_usd',
      render: (val) => formatUSDAmount(val),
      sorter: (a, b) => a.total_usd - b.total_usd,
    },
    {
      title: t('请求次数'),
      dataIndex: 'request_count',
      render: (val) => val.toLocaleString(),
      sorter: (a, b) => a.request_count - b.request_count,
    },
    {
      title: t('占比'),
      dataIndex: 'percentage',
      render: (val) => `${val.toFixed(2)}%`,
      sorter: (a, b) => a.percentage - b.percentage,
    },
  ];

  // Plan balance columns
  const balanceColumns = [
    {
      title: t('套餐名称'),
      dataIndex: 'plan_display_name',
      render: (text, record) => text || record.plan_name || '-',
    },
    {
      title: t('类型'),
      dataIndex: 'plan_type',
      render: (val) => {
        const map = {
          subscription: t('订阅'),
          consumption: t('消耗'),
          trial: t('试用'),
        };
        return map[val] || val || '-';
      },
    },
    {
      title: t('状态'),
      dataIndex: 'is_current',
      render: (v) => (v === 1 ? <Tag color="green" size="small">{t('当前')}</Tag> : null),
    },
    {
      title: t('总额度'),
      dataIndex: 'total_quota_usd',
      render: (val) => formatUSDAmount(val),
    },
    {
      title: t('已用'),
      dataIndex: 'used_quota_usd',
      render: (val) => formatUSDAmount(val),
    },
    {
      title: t('剩余'),
      dataIndex: 'quota_usd',
      render: (val) => formatUSDAmount(val),
    },
    {
      title: t('使用率'),
      dataIndex: 'usage_percent',
      render: (val) => (
        <div style={{ minWidth: 100 }}>
          <Progress percent={Math.min(Number(val) || 0, 100)} size="small" showInfo />
        </div>
      ),
    },
    {
      title: t('今日限额'),
      dataIndex: 'daily_quota_limit_usd',
      render: (val) => (val && Number(val) > 0 ? formatUSDAmount(val) : '-'),
    },
    {
      title: t('今日已用'),
      dataIndex: 'today_used_usd',
      render: (val) => formatUSDAmount(val),
    },
    {
      title: t('今日剩余'),
      dataIndex: 'today_remaining_usd',
      render: (val) => formatUSDAmount(val),
    },
    {
      title: t('过期时间'),
      dataIndex: 'expires_at',
      render: (val) => {
        const num = Number(val);
        if (num === 0) return t('永久');
        if (!num) return '-';
        const seconds = num > 1e12 ? Math.floor(num / 1000) : Math.floor(num);
        return timestamp2string(seconds);
      },
    },
  ];

  // Topup columns
  const topupColumns = [
    { title: t('订单号'), dataIndex: 'trade_no' },
    { title: t('金额'), dataIndex: 'money', render: (v) => formatUSDAmount(v) },
    { title: t('支付方式'), dataIndex: 'payment_method' },
    { title: t('状态'), dataIndex: 'status' },
    {
      title: t('创建时间'),
      dataIndex: 'create_time',
      render: (v) => {
        const num = Number(v);
        if (!num) return '-';
        const seconds = num > 1e12 ? Math.floor(num / 1000) : Math.floor(num);
        return timestamp2string(seconds);
      },
    },
    {
      title: t('完成时间'),
      dataIndex: 'complete_time',
      render: (v) => {
        const num = Number(v);
        if (!num) return '-';
        const seconds = num > 1e12 ? Math.floor(num / 1000) : Math.floor(num);
        return timestamp2string(seconds);
      },
    },
  ];

  // Plan order columns
  const planOrderColumns = [
    { title: t('订单号'), dataIndex: 'order_no' },
    {
      title: t('套餐名称'),
      dataIndex: 'plan_display_name',
      render: (text, record) => text || record.plan_name || '-',
    },
    { title: t('价格'), dataIndex: 'final_price', render: (v) => formatUSDAmount(v) },
    { title: t('支付方式'), dataIndex: 'payment_method' },
    { title: t('状态'), dataIndex: 'status' },
    {
      title: t('创建时间'),
      dataIndex: 'created_at',
      render: (v) => {
        const num = Number(v);
        if (!num) return '-';
        const seconds = num > 1e12 ? Math.floor(num / 1000) : Math.floor(num);
        return timestamp2string(seconds);
      },
    },
    {
      title: t('支付时间'),
      dataIndex: 'paid_at',
      render: (v) => {
        const num = Number(v);
        if (!num) return '-';
        const seconds = num > 1e12 ? Math.floor(num / 1000) : Math.floor(num);
        return timestamp2string(seconds);
      },
    },
  ];

  const handleTopupPageChange = (page, pageSize) => {
    fetchTopups(page, pageSize || topupPagination.pageSize);
  };

  const handleTopupPageSizeChange = (pageSize) => {
    fetchTopups(1, pageSize);
  };

  const handlePlanOrderPageChange = (page, pageSize) => {
    fetchPlanOrders(page, pageSize || planOrderPagination.pageSize);
  };

  const handlePlanOrderPageSizeChange = (pageSize) => {
    fetchPlanOrders(1, pageSize);
  };

  return (
    <SideSheet
      title={
        <Space>
          <IconUser />
          <Title heading={4} style={{ margin: 0 }}>
            {t('用户消耗详情')} - {user?.username}
          </Title>
        </Space>
      }
      visible={visible}
      onCancel={onClose}
      width={isMobile ? '100%' : 1200}
      placement="right"
      bodyStyle={{ padding: 0 }}
      footer={
        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <Space>
            <Select
              value={selectedDays}
              onChange={setSelectedDays}
              style={{ width: 150 }}
              prefix={<IconCalendar />}
            >
              <Select.Option value={7}>{t('最近7天')}</Select.Option>
              <Select.Option value={15}>{t('最近15天')}</Select.Option>
              <Select.Option value={30}>{t('最近30天')}</Select.Option>
              <Select.Option value={60}>{t('最近60天')}</Select.Option>
              <Select.Option value={90}>{t('最近90天')}</Select.Option>
            </Select>
          </Space>
        </div>
      }
      closeIcon={<IconClose />}
    >
      <Spin spinning={loading}>
        <div style={{ padding: 24 }}>
          {data ? (
            <>
              {/* User basic info */}
              <Card style={{ marginBottom: 20 }}>
                <Space>
                  <Tag color="blue" size="large">
                    ID: {data.user_info.id}
                  </Tag>
                  <Tag color="green" size="large">
                    {t('余额')}: {formatUSDAmount(data.user_info.quota_usd)}
                  </Tag>
                  <Tag color="orange" size="large">
                    {t('已用')}: {formatUSDAmount(data.user_info.used_quota_usd)}
                  </Tag>
                  <Tag color="purple" size="large">
                    {t('请求数')}: {data.user_info.request_count.toLocaleString()}
                  </Tag>
                </Space>
              </Card>

              {/* Plan balance info */}
              <Card style={{ marginBottom: 20 }}>
                <Title heading={5} style={{ marginBottom: 12 }}>
                  {t('套餐余额')}
                </Title>
                <Table
                  columns={balanceColumns}
                  dataSource={data.user_plan_balances || []}
                  rowKey="plan_name"
                  size="small"
                  pagination={false}
                  empty={<Empty description={t('暂无套餐数据')} />}
                  scroll={{ x: 1200 }}
                />
              </Card>

              {/* Statistics cards */}
              {renderStatsCards()}

              {/* Charts tabs */}
              <Card>
                <Tabs
                  activeKey={activeTab}
                  onChange={setActiveTab}
                  type="line"
                  size="small"
                >
                  <TabPane
                    tab={t('每日消耗趋势')}
                    itemKey="daily"
                  >
                    {dailyChartSpec ? (
                      <div style={{ width: '100%', height: 400 }}>
                        <VChart
                          spec={dailyChartSpec}
                          option={{ mode: 'desktop-browser' }}
                        />
                      </div>
                    ) : (
                      <Empty description={t('暂无数据')} />
                    )}
                  </TabPane>

                  <TabPane
                    tab={t('套餐消耗对比')}
                    itemKey="plans"
                  >
                    {planChartSpec ? (
                      <div style={{ width: '100%', height: 400 }}>
                        <VChart
                          spec={planChartSpec}
                          option={{ mode: 'desktop-browser' }}
                        />
                      </div>
                    ) : (
                      <Empty description={t('暂无套餐数据')} />
                    )}
                  </TabPane>

                  <TabPane
                    tab={t('模型使用占比')}
                    itemKey="models"
                  >
                    <Row gutter={16}>
                      <Col span={isMobile ? 24 : 12}>
                        {modelPieSpec ? (
                          <div style={{ width: '100%', height: 400 }}>
                            <VChart
                              spec={modelPieSpec}
                              option={{ mode: 'desktop-browser' }}
                            />
                          </div>
                        ) : (
                          <Empty description={t('暂无数据')} />
                        )}
                      </Col>
                  <Col span={isMobile ? 24 : 12}>
                    <Table
                      columns={modelTableColumns}
                      dataSource={data.model_summary || []}
                      rowKey="model_name"
                      pagination={false}
                      size="small"
                      style={{ marginTop: isMobile ? 20 : 0 }}
                    />
                  </Col>
                </Row>
              </TabPane>

              <TabPane
                tab={t('消费记录')}
                itemKey="records"
              >
                <Space vertical style={{ width: '100%' }} size="large">
                  <Card>
                    <Title heading={5} style={{ marginBottom: 12 }}>
                      {t('钱包充值记录')}
                    </Title>
                    <Table
                      columns={topupColumns}
                      dataSource={topups}
                      loading={topupLoading}
                      rowKey="trade_no"
                      size="small"
                      pagination={{
                        currentPage: topupPagination.currentPage,
                        pageSize: topupPagination.pageSize,
                        total: topupPagination.total,
                        showSizeChanger: true,
                        pageSizeOpts: [10, 20, 50, 100],
                        onPageChange: handleTopupPageChange,
                        onPageSizeChange: handleTopupPageSizeChange,
                      }}
                      empty={<Empty description={t('暂无充值记录')} />}
                      scroll={{ x: 900 }}
                    />
                  </Card>

                  <Card>
                    <Title heading={5} style={{ marginBottom: 12 }}>
                      {t('套餐订单记录')}
                    </Title>
                    <Table
                      columns={planOrderColumns}
                      dataSource={planOrders}
                      loading={planOrderLoading}
                      rowKey="order_no"
                      size="small"
                      pagination={{
                        currentPage: planOrderPagination.currentPage,
                        pageSize: planOrderPagination.pageSize,
                        total: planOrderPagination.total,
                        showSizeChanger: true,
                        pageSizeOpts: [10, 20, 50, 100],
                        onPageChange: handlePlanOrderPageChange,
                        onPageSizeChange: handlePlanOrderPageSizeChange,
                      }}
                      empty={<Empty description={t('暂无套餐订单')} />}
                      scroll={{ x: 1000 }}
                    />
                  </Card>
                </Space>
              </TabPane>
            </Tabs>
          </Card>
        </>
      ) : (
        <Empty description={t('暂无数据')} />
          )}
        </div>
      </Spin>
    </SideSheet>
  );
};

export default UserDetailModal;
