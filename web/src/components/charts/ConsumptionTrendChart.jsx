import React, { useMemo } from 'react';
import { VChart } from '@visactor/react-vchart';
import { Spin, Empty } from '@douyinfe/semi-ui';
import { formatUSDAmount } from '../../utils/currency';

/**
 * ConsumptionTrendChart - Line chart showing consumption trends over time in USD
 *
 * @param {Object} props
 * @param {Array} props.data - Array of consumption trend data points
 *   [{date: "2025-01-01", total_usd: 123.45, request_count: 100, user_count: 10}, ...]
 * @param {boolean} props.loading - Loading state
 * @param {string} props.timeRange - Time range for context (e.g., "7d", "30d")
 * @param {number} props.height - Chart height in pixels (default: 400)
 */
const ConsumptionTrendChart = ({
  data = [],
  loading = false,
  timeRange = '7d',
  height = 400,
}) => {
  const chartSpec = useMemo(() => {
    if (!data || data.length === 0) {
      return null;
    }

    // Transform data for VChart
    const chartData = data.map((item) => ({
      date: item.date,
      value: item.total_usd || 0,
      requestCount: item.request_count || 0,
      userCount: item.user_count || 0,
    }));

    return {
      type: 'line',
      data: [
        {
          id: 'consumption',
          values: chartData,
        },
      ],
      xField: 'date',
      yField: 'value',
      point: {
        visible: true,
        style: {
          size: 4,
          fill: 'white',
          stroke: '#6366f1',
          lineWidth: 2,
        },
      },
      line: {
        style: {
          stroke: '#6366f1',
          lineWidth: 3,
        },
      },
      axes: [
        {
          orient: 'bottom',
          type: 'band',
          label: {
            style: {
              fontSize: 12,
            },
          },
        },
        {
          orient: 'left',
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
      ],
      tooltip: {
        mark: {
          title: {
            value: (datum) => datum.date,
          },
          content: [
            {
              key: 'Consumption',
              value: (datum) => formatUSDAmount(datum.value),
            },
            {
              key: 'Requests',
              value: (datum) => datum.requestCount?.toLocaleString() || '0',
            },
            {
              key: 'Users',
              value: (datum) => datum.userCount?.toLocaleString() || '0',
            },
          ],
        },
      },
      crosshair: {
        xField: {
          visible: true,
          line: {
            type: 'line',
            style: {
              lineDash: [4, 4],
              stroke: '#94a3b8',
            },
          },
        },
      },
      legends: {
        visible: false,
      },
      padding: {
        top: 20,
        right: 20,
        bottom: 40,
        left: 60,
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
          description='No consumption data available for the selected period'
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

export default ConsumptionTrendChart;
