import React, { useMemo } from 'react';
import { VChart } from '@visactor/react-vchart';
import { Spin, Empty, Tag } from '@douyinfe/semi-ui';
import { formatUSDAmount } from '../../utils/currency';

/**
 * ModelUsageChart - Grouped bar chart showing model usage statistics
 *
 * @param {Object} props
 * @param {Array} props.data - Array of model usage data
 *   [{model_name: "gpt-4", request_count: 1000, unique_users: 50, total_usd: 100, success_rate: 98.5}, ...]
 * @param {boolean} props.loading - Loading state
 * @param {number} props.height - Chart height in pixels (default: 500)
 * @param {number} props.maxModels - Max number of models to display (default: 10)
 */
const ModelUsageChart = ({
  data = [],
  loading = false,
  height = 500,
  maxModels = 10,
}) => {
  const chartSpec = useMemo(() => {
    if (!data || data.length === 0) {
      return null;
    }

    // Take top N models by request count
    const topModels = [...data]
      .sort((a, b) => b.request_count - a.request_count)
      .slice(0, maxModels);

    // Transform data for grouped bar chart
    const chartData = topModels.flatMap((item) => [
      {
        model: item.model_name || 'Unknown',
        metric: 'Requests (k)',
        value: (item.request_count || 0) / 1000,
        displayValue: item.request_count?.toLocaleString() || '0',
        successRate: item.success_rate || 0,
        totalUsd: item.total_usd || 0,
      },
      {
        model: item.model_name || 'Unknown',
        metric: 'Users',
        value: item.unique_users || 0,
        displayValue: item.unique_users?.toLocaleString() || '0',
        successRate: item.success_rate || 0,
        totalUsd: item.total_usd || 0,
      },
    ]);

    return {
      type: 'bar',
      data: [
        {
          id: 'modelUsage',
          values: chartData,
        },
      ],
      xField: 'model',
      yField: 'value',
      seriesField: 'metric',
      bar: {
        style: {
          cornerRadius: [4, 4, 0, 0],
        },
      },
      color: {
        type: 'ordinal',
        range: ['#6366f1', '#10b981'],
      },
      axes: [
        {
          orient: 'bottom',
          type: 'band',
          label: {
            style: {
              fontSize: 11,
              angle: -45,
              textAlign: 'right',
              textBaseline: 'middle',
            },
          },
        },
        {
          orient: 'left',
          type: 'linear',
          label: {
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
            value: (datum) => datum.model,
          },
          content: [
            {
              key: 'Metric',
              value: (datum) => `${datum.metric}: ${datum.displayValue}`,
            },
            {
              key: 'Total Cost',
              value: (datum) => formatUSDAmount(datum.totalUsd),
            },
            {
              key: 'Success Rate',
              value: (datum) => `${datum.successRate.toFixed(2)}%`,
            },
          ],
        },
      },
      legends: {
        visible: true,
        orient: 'top',
        padding: {
          bottom: 20,
        },
        item: {
          shape: {
            style: {
              symbolType: 'rect',
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
        top: 60,
        right: 20,
        bottom: 80,
        left: 60,
      },
    };
  }, [data, maxModels]);

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
          description='No model usage data available'
        />
      </div>
    );
  }

  return (
    <div style={{ width: '100%' }}>
      <div style={{ width: '100%', height: height }}>
        <VChart spec={chartSpec} option={{ mode: 'desktop-browser' }} />
      </div>

      {/* Success rate badges */}
      <div
        style={{
          marginTop: 16,
          display: 'flex',
          flexWrap: 'wrap',
          gap: 8,
        }}
      >
        {data
          .sort((a, b) => b.request_count - a.request_count)
          .slice(0, maxModels)
          .map((item) => {
            const rate = item.success_rate || 0;
            const color =
              rate >= 95 ? 'green' : rate >= 85 ? 'yellow' : 'red';

            return (
              <Tag
                key={item.model_name}
                color={color}
                size='small'
                style={{ fontSize: 11 }}
              >
                {item.model_name}: {rate.toFixed(1)}% success
              </Tag>
            );
          })}
      </div>
    </div>
  );
};

export default ModelUsageChart;
