import React from 'react';
import PropTypes from 'prop-types';
import { Typography } from '@douyinfe/semi-ui';

const { Text } = Typography;

/**
 * MoneyWithDetails - Component to display USD amount with supplementary details
 *
 * Usage:
 *   <MoneyWithDetails
 *     usd={125.50}
 *     requests={1234}
 *     tokens={890}
 *     fontSize={16}
 *   />
 */
const MoneyWithDetails = ({
  usd,
  requests,
  tokens,
  fontSize = 16,
  showCurrency = true,
}) => {
  const formattedUSD = typeof usd === 'number'
    ? `$${usd.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
    : '$0.00';
  const formattedRequests = typeof requests === 'number' ? requests.toLocaleString() : '0';
  const formattedTokens = typeof tokens === 'number' ? tokens.toLocaleString() : null;

  // Build secondary info text
  let secondaryText = `${formattedRequests} requests`;
  if (formattedTokens) {
    secondaryText += ` · 平均${formattedTokens} tokens`;
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
      {/* Primary: USD amount */}
      <Text
        strong
        style={{
          fontSize: `${fontSize}px`,
          color: '#52c41a',
          fontWeight: 600,
        }}
      >
        {formattedUSD}
      </Text>

      {/* Secondary: request count and optional token info */}
      {requests !== undefined && (
        <Text
          type="tertiary"
          style={{
            fontSize: `${fontSize * 0.75}px`,
            fontWeight: 400,
          }}
        >
          {secondaryText}
        </Text>
      )}
    </div>
  );
};

MoneyWithDetails.propTypes = {
  usd: PropTypes.number,
  requests: PropTypes.number,
  tokens: PropTypes.number,
  fontSize: PropTypes.number,
  showCurrency: PropTypes.bool,
};

export default MoneyWithDetails;
