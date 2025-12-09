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
  Input,
  Empty,
  Spin,
  Space,
  Typography,
} from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import { VChart } from '@visactor/react-vchart';
import { AnalyticsAPI } from '../../../services/analyticsApi';
import { formatUSDAmount } from '../../../utils/currency';

const { Text } = Typography;

const UserConsumptionTab = ({ timeRange }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [userDailyData, setUserDailyData] = useState([]);
  const [username, setUsername] = useState('');
  const [searchUsername, setSearchUsername] = useState('');

  useEffect(() => {
    fetchUserDailyData();
  }, [timeRange, searchUsername]);

  const fetchUserDailyData = async () => {
    setLoading(true);
    try {
      const result = await AnalyticsAPI.fetchUserDailyConsumptionTrend(
        timeRange,
        [],
        searchUsername
      );
      if (result && result.trends) {
        setUserDailyData(result.trends);
      } else {
        setUserDailyData([]);
      }
    } catch (err) {
      console.error('Failed to load user daily consumption data:', err);
      setUserDailyData([]);
    } finally {
      setLoading(false);
    }
  };

  const handleSearch = () => {
    setSearchUsername(username);
  };

  const handleKeyPress = (e) => {
    if (e.key === 'Enter') {
      handleSearch();
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

  return (
    <div style={{ marginTop: 16 }}>
      {/* Search Filter */}
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col span={24}>
          <Card>
            <Space>
              <Text strong>{t('用户名筛选')}:</Text>
              <Input
                prefix={<IconSearch />}
                placeholder={t('输入用户名搜索')}
                value={username}
                onChange={setUsername}
                onEnterPress={handleKeyPress}
                style={{ width: 300 }}
              />
            </Space>
          </Card>
        </Col>
      </Row>

      {/* User Daily Consumption Bar Chart */}
      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card title={t('用户每日消费金额')}>
            {userDailyData.length === 0 ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={t('暂无数据，请调整筛选条件或时间范围')}
                style={{ padding: '40px 0' }}
              />
            ) : (
              <VChart
                spec={{
                  type: 'bar',
                  data: [
                    {
                      id: 'userDailyData',
                      values: userDailyData.map(d => ({
                        date: d.date,
                        user: d.display_name || d.username,
                        value: d.total_quota_usd,
                        requests: d.request_count,
                      }))
                    }
                  ],
                  xField: 'date',
                  yField: 'value',
                  seriesField: 'user',
                  stack: true,
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
                      maxRow: 3,
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
                        text: t('消费金额 (USD)'),
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
                    dimension: {
                      title: {
                        value: (datum) => {
                          const data = Array.isArray(datum) ? datum : (datum ? [datum] : []);
                          return data.length > 0 ? `${t('日期')}: ${data[0]?.date || '-'}` : t('日期') + ': -';
                        },
                      },
                      content: [
                        {
                          key: (datum) => datum['user'],
                          value: (datum) => ({
                            value: datum['value'] || 0,
                            requests: datum['requests'] || 0,
                          }),
                        },
                      ],
                      updateContent: (array) => {
                        // Sort by value in descending order
                        array.sort((a, b) => {
                          const aVal = typeof a.value === 'object' ? a.value.value : a.value;
                          const bVal = typeof b.value === 'object' ? b.value.value : b.value;
                          return bVal - aVal;
                        });

                        let sum = 0;
                        // Process each item and calculate sum
                        for (let i = 0; i < array.length; i++) {
                          const itemValue = typeof array[i].value === 'object' ? array[i].value : { value: 0, requests: 0 };
                          const value = itemValue.value || 0;
                          const requests = itemValue.requests || 0;
                          sum += value;
                          array[i].value = `$${Number(value).toFixed(4)} (${t('请求数')}: ${Number(requests).toLocaleString()})`;
                        }

                        // Add total row at the beginning
                        array.unshift({
                          key: t('总额'),
                          value: `$${Number(sum).toFixed(4)}`,
                        });

                        return array;
                      },
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

      {/* Data Info */}
      {userDailyData.length > 0 && (
        <Card style={{ marginTop: 16 }}>
          <Text type='tertiary' style={{ fontSize: '12px' }}>
            {t('共显示')} {new Set(userDailyData.map(d => d.user_id)).size} {t('个用户的消费数据')}
          </Text>
        </Card>
      )}
    </div>
  );
};

export default UserConsumptionTab;
