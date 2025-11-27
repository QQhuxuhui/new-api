import React, { useMemo } from 'react';
import { VChart } from '@visactor/react-vchart';
import { Spin, Empty } from '@douyinfe/semi-ui';

/**
 * BalanceDistributionChart - Pie/Donut chart showing balance distribution across ranges
 *
 * @param {Object} props
 * @param {Array} props.data - Array of balance distribution data
 *   [{range_label: "$0-$10", user_count: 45, percentage: 30.5}, ...]
 * @param {boolean} props.loading - Loading state
 * @param {string} props.chartType - "pie" or "donut" (default: "donut")
 * @param {number} props.height - Chart height in pixels (default: 400)
 */
const BalanceDistributionChart = ({
  data = [],
  loading = false,
  chartType = 'donut',
  height = 400,
}) => {
  const chartSpec = useMemo(() => {
    if (!data || data.length === 0) {
      return null;
    }

    // Filter out ranges with 0 users
    const filteredData = data.filter((item) => item.user_count > 0);

    if (filteredData.length === 0) {
      return null;
    }

    // Color palette for different balance ranges
    const colors = [
      '#ef4444',
      '#f97316',
      '#f59e0b',
      '#84cc16',
      '#10b981',
      '#6366f1',
    ];

    const chartData = filteredData.map((item, index) => ({
      range: item.range_label,
      value: item.user_count,
      percentage: item.percentage || 0,
      color: colors[index % colors.length],
    }));

    return {
      type: 'pie',
      data: [
        {
          id: 'balance',
          values: chartData,
        },
      ],
      categoryField: 'range',
      valueField: 'value',
      innerRadius: chartType === 'donut' ? 0.6 : 0,
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
          selected: {
            outerRadius: 0.95,
            stroke: '#000',
            lineWidth: 1,
          },
        },
      },
      label: {
        visible: true,
        position: 'outside',
        style: {
          fontSize: 12,
          fontWeight: 'bold',
        },
        formatMethod: (value, datum) => {
          return `${datum.range}\n${datum.value} users (${datum.percentage.toFixed(1)}%)`;
        },
      },
      tooltip: {
        mark: {
          title: {
            value: (datum) => datum.range,
          },
          content: [
            {
              key: 'Users',
              value: (datum) => datum.value.toLocaleString(),
            },
            {
              key: 'Percentage',
              value: (datum) => `${datum.percentage.toFixed(2)}%`,
            },
          ],
        },
      },
      legends: {
        visible: true,
        orient: 'bottom',
        padding: {
          top: 20,
        },
        item: {
          shape: {
            style: {
              symbolType: 'circle',
            },
          },
          label: {
            style: {
              fontSize: 12,
            },
          },
        },
      },
      padding: {
        top: 20,
        right: 20,
        bottom: 60,
        left: 20,
      },
    };
  }, [data, chartType]);

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

  if (!data || data.length === 0 || !chartSpec) {
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
          description='No balance distribution data available'
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

export default BalanceDistributionChart;
