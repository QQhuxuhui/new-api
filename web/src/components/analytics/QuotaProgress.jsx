import React from 'react';
import PropTypes from 'prop-types';
import { Progress, Typography } from '@douyinfe/semi-ui';

const { Text } = Typography;

/**
 * QuotaProgress - Component to display quota status with color-coded progress bar
 *
 * Color coding:
 *   - Green (<50%): Healthy
 *   - Yellow (50-80%): Warning
 *   - Red (>80%): Critical
 *
 * Usage:
 *   <QuotaProgress
 *     usedUSD={150.00}
 *     totalUSD={200.00}
 *     requests={1234}
 *   />
 */
const QuotaProgress = ({ usedUSD, totalUSD, requests }) => {
  const usageRate = totalUSD > 0 ? (usedUSD / totalUSD) * 100 : 0;

  // Determine color based on usage rate
  let progressColor = '#52c41a'; // Green (healthy)
  let textColor = '#52c41a';

  if (usageRate >= 80) {
    progressColor = '#ff4d4f'; // Red (critical)
    textColor = '#ff4d4f';
  } else if (usageRate >= 50) {
    progressColor = '#faad14'; // Yellow (warning)
    textColor = '#faad14';
  }

  const formattedUsed = typeof usedUSD === 'number' ? `$${usedUSD.toFixed(2)}` : '$0.00';
  const formattedTotal = typeof totalUSD === 'number' ? `$${totalUSD.toFixed(2)}` : '$0.00';
  const formattedRequests =
    typeof requests === 'number' ? `${requests.toLocaleString()} requests` : '';

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
      {/* Line 1: USD display */}
      <Text style={{ fontSize: '14px', color: textColor, fontWeight: 500 }}>
        {formattedUsed} / {formattedTotal}
      </Text>

      {/* Line 2: Progress bar */}
      <Progress
        percent={Math.min(usageRate, 100)}
        showInfo={true}
        format={(percent) => `${percent.toFixed(0)}%`}
        stroke={progressColor}
        style={{ width: '200px' }}
      />

      {/* Line 3: Request count (secondary info) */}
      {requests !== undefined && (
        <Text type="tertiary" style={{ fontSize: '12px' }}>
          {formattedRequests}
        </Text>
      )}
    </div>
  );
};

QuotaProgress.propTypes = {
  usedUSD: PropTypes.number.isRequired,
  totalUSD: PropTypes.number.isRequired,
  requests: PropTypes.number,
};

export default QuotaProgress;
