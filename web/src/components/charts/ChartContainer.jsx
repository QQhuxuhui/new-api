import React from 'react';
import { Card, Spin, Empty, Button, Typography } from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';

const { Text } = Typography;

/**
 * ChartContainer - Reusable wrapper for chart components with loading, error, and empty states
 *
 * @param {Object} props
 * @param {React.ReactNode} props.children - Chart component to wrap
 * @param {string} props.title - Chart title (optional)
 * @param {boolean} props.loading - Loading state
 * @param {boolean} props.error - Error state
 * @param {string} props.errorMessage - Custom error message
 * @param {Function} props.onRetry - Retry function for error state
 * @param {boolean} props.isEmpty - Empty state flag
 * @param {string} props.emptyMessage - Custom empty state message
 * @param {number} props.height - Container height in pixels (default: 400)
 * @param {Object} props.style - Additional custom styles
 */
const ChartContainer = ({
  children,
  title,
  loading = false,
  error = false,
  errorMessage = 'Failed to load chart data',
  onRetry,
  isEmpty = false,
  emptyMessage = 'No data available',
  height = 400,
  style = {},
}) => {
  const renderContent = () => {
    if (loading) {
      return (
        <div
          style={{
            height: height,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 16,
          }}
        >
          <Spin size='large' />
          <Text type='secondary'>Loading chart data...</Text>
        </div>
      );
    }

    if (error) {
      return (
        <div
          style={{
            height: height,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 16,
          }}
        >
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={
              <div>
                <Text type='danger'>{errorMessage}</Text>
                {onRetry && (
                  <div style={{ marginTop: 12 }}>
                    <Button
                      icon={<IconRefresh />}
                      onClick={onRetry}
                      type='primary'
                      theme='solid'
                    >
                      Retry
                    </Button>
                  </div>
                )}
              </div>
            }
          />
        </div>
      );
    }

    if (isEmpty) {
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
            description={emptyMessage}
          />
        </div>
      );
    }

    return children;
  };

  return (
    <Card
      title={title}
      style={{
        borderRadius: 8,
        ...style,
      }}
      bodyStyle={{
        padding: '16px',
      }}
    >
      {renderContent()}
    </Card>
  );
};

export default ChartContainer;
