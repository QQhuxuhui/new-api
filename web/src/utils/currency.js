/**
 * Currency conversion utilities for converting quota to USD
 *
 * IMPORTANT: QUOTA_PER_UNIT must match the backend constant in common/constants.go
 * Current value: 500000 = $1.00 USD
 */

// Must match backend common.QuotaPerUnit constant
export const QUOTA_PER_UNIT = 500000;

/**
 * Converts quota units to USD amount
 *
 * @param {number} quota - The quota value to convert
 * @returns {number} The USD amount
 *
 * @example
 * quotaToUSD(500000)  // Returns: 1.0
 * quotaToUSD(250000)  // Returns: 0.5
 * quotaToUSD(0)       // Returns: 0.0
 */
export function quotaToUSD(quota) {
  if (typeof quota !== 'number') {
    return 0;
  }
  return quota / QUOTA_PER_UNIT;
}

/**
 * Formats quota value as USD currency string
 *
 * @param {number} quota - The quota value to format
 * @param {Object} options - Formatting options
 * @param {number} options.minimumFractionDigits - Minimum decimal places (default: 2)
 * @param {number} options.maximumFractionDigits - Maximum decimal places (default: 2)
 * @param {string} options.locale - Locale for formatting (default: 'en-US')
 * @returns {string} Formatted USD string (e.g., "$123.45")
 *
 * @example
 * formatUSD(500000)           // Returns: "$1.00"
 * formatUSD(250000)           // Returns: "$0.50"
 * formatUSD(123456789)        // Returns: "$246.91"
 * formatUSD(5000, { maximumFractionDigits: 4 })  // Returns: "$0.0100"
 */
export function formatUSD(quota, options = {}) {
  const usd = quotaToUSD(quota);

  const {
    minimumFractionDigits = 2,
    maximumFractionDigits = 2,
    locale = 'en-US',
  } = options;

  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits,
    maximumFractionDigits,
  }).format(usd);
}

/**
 * Formats a USD amount (not quota) as currency string
 * Use this when you already have a USD value from the API
 *
 * @param {number} usdAmount - The USD amount to format
 * @param {Object} options - Formatting options (same as formatUSD)
 * @returns {string} Formatted USD string (e.g., "$123.45")
 *
 * @example
 * formatUSDAmount(1.5)    // Returns: "$1.50"
 * formatUSDAmount(0.01)   // Returns: "$0.01"
 */
export function formatUSDAmount(usdAmount, options = {}) {
  const {
    minimumFractionDigits = 2,
    maximumFractionDigits = 2,
    locale = 'en-US',
  } = options;

  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits,
    maximumFractionDigits,
  }).format(usdAmount);
}
