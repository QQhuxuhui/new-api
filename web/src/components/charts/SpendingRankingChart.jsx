import React, { useMemo } from 'react';
import { VChart } from '@visactor/react-vchart';
import { Spin, Empty } from '@douyinfe/semi-ui';
import { formatUSDAmount } from '../../utils/currency';

/**
 * SpendingRankingChart - Horizontal bar chart showing top spenders
 *
 * @param {Object} props
 * @param {Array} props.data - Array of top spender data
 *   [{user_id: 1, username: "user1", total_usd: 456.78, request_count: 100}, ...]
 * @param {boolean} props.loading - Loading state
 * @param {number} props.limit - Number of top spenders to show
 * @param {number} props.height - Chart height in pixels (default: 500)
 */
const SpendingRankingChart = ({
  data = [],
  loading = false,
  limit = 20,
  height = 500,
}) => {
  const chartSpec = useMemo(() => {
    if (!data || data.length === 0) {
      return null;
    }

    // Transform data for VChart - reverse for top-to-bottom display
    const chartData = [...data].reverse().map((item, index) => {
      const rank = data.length - index;
      const isTopThree = rank <= 3;

      return {
        username: item.username || `User ${item.user_id}`,
        value: item.total_usd || 0,
        requestCount: item.request_count || 0,
        rank: rank,
        color: isTopThree
          ? rank === 1
            ? '#fbbf24'
            : rank === 2
              ? '#94a3b8'
              : '#f59e0b'
          : '#6366f1',
      };
    });

    return {
      type: 'bar',
      data: [
        {
          id: 'spending',
          values: chartData,
        },
      ],
      direction: 'horizontal',
      xField: 'value',
      yField: 'username',
      seriesField: 'username',
      bar: {
        style: {
          fill: (datum) => datum.color,
          cornerRadius: [0, 4, 4, 0],
        },
      },
      axes: [
        {
          orient: 'bottom',
          type: 'linear',
          label: {
            formatMethod: (value) => formatUSDAmount(value),
            style: {
              fontSize: 12,
            },
          },
          grid: {
            visible: true,
            style: {
              lineDash: [4, 4],
              stroke: '#e5e7eb',
            },
          },
        },
        {
          orient: 'left',
          type: 'band',
          label: {
            style: {
              fontSize: 11,
              maxLineWidth: 100,
            },
          },
        },
      ],
      tooltip: {
        mark: {
          title: {
            value: (datum) => `#${datum.rank} - ${datum.username}`,
          },
          content: [
            {
              key: 'Total Spent',
              value: (datum) => formatUSDAmount(datum.value),
            },
            {
              key: 'Requests',
              value: (datum) => datum.requestCount?.toLocaleString() || '0',
            },
          ],
        },
      },
      legends: {
        visible: false,
      },
      padding: {
        top: 20,
        right: 20,
        bottom: 40,
        left: 120,
      },
    };
  }, [data]);

  if (loading) {
    return (
      <div
        style={{
          height: height,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Spin size='large' />
      </div>
    );
  }

  if (!data || data.length === 0) {
    return (
      <div
        style={{
          height: height,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description='No spending data available'
        />
      </div>
    );
  }

  return (
    <div style={{ width: '100%', height: height }}>
      <VChart spec={chartSpec} option={{ mode: 'desktop-browser' }} />
    </div>
  );
};

export default SpendingRankingChart;
