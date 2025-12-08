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

import { API, showError } from '../helpers';

const BASE_URL = '/api/admin/analytics';

/**
 * Analytics API Service
 */
export const AnalyticsAPI = {
  /**
   * Fetch user overview metrics
   * @param {string} timeRange - Time range: '1d', '7d', '30d', '90d'
   * @returns {Promise<Object>}
   */
  async fetchUserOverview(timeRange = '7d') {
    try {
      const response = await API.get(`${BASE_URL}/user-overview`, {
        params: { time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch user overview');
    } catch (error) {
      showError(error.message || 'Failed to fetch user overview');
      throw error;
    }
  },

  /**
   * Fetch active users ranking
   * @param {string} timeRange - Time range
   * @param {number} limit - Number of results
   * @returns {Promise<Array>}
   */
  async fetchActiveUsers(timeRange = '7d', limit = 20) {
    try {
      const response = await API.get(`${BASE_URL}/active-users`, {
        params: { time_range: timeRange, limit },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch active users');
    } catch (error) {
      showError(error.message || 'Failed to fetch active users');
      throw error;
    }
  },

  /**
   * Fetch consumption trend data
   * @param {string} timeRange - Time range
   * @returns {Promise<Array>}
   */
  async fetchConsumptionTrend(timeRange = '7d') {
    try {
      const response = await API.get(`${BASE_URL}/consumption-trend`, {
        params: { time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch consumption trend');
    } catch (error) {
      showError(error.message || 'Failed to fetch consumption trend');
      throw error;
    }
  },

  /**
   * Fetch top spenders
   * @param {string} timeRange - Time range
   * @param {number} limit - Number of results
   * @returns {Promise<Array>}
   */
  async fetchTopSpenders(timeRange = '7d', limit = 20) {
    try {
      const response = await API.get(`${BASE_URL}/consumption-ranking`, {
        params: { time_range: timeRange, limit },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch top spenders');
    } catch (error) {
      showError(error.message || 'Failed to fetch top spenders');
      throw error;
    }
  },

  /**
   * Fetch model usage statistics
   * @param {string} timeRange - Time range
   * @returns {Promise<Array>}
   */
  async fetchModelUsage(timeRange = '7d') {
    try {
      const response = await API.get(`${BASE_URL}/model-usage`, {
        params: { time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch model usage');
    } catch (error) {
      showError(error.message || 'Failed to fetch model usage');
      throw error;
    }
  },

  /**
   * Fetch behavior patterns
   * @param {string} timeRange - Time range
   * @returns {Promise<Object>}
   */
  async fetchBehaviorPatterns(timeRange = '7d') {
    try {
      const response = await API.get(`${BASE_URL}/behavior-patterns`, {
        params: { time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch behavior patterns');
    } catch (error) {
      showError(error.message || 'Failed to fetch behavior patterns');
      throw error;
    }
  },

  /**
   * Fetch risk indicators
   * @param {string} timeRange - Time range
   * @returns {Promise<Array>}
   */
  async fetchRiskIndicators(timeRange = '24h') {
    try {
      const response = await API.get(`${BASE_URL}/risk-indicators`, {
        params: { time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch risk indicators');
    } catch (error) {
      showError(error.message || 'Failed to fetch risk indicators');
      throw error;
    }
  },

  /**
   * Fetch user balance analysis (overview, distribution, rankings)
   * @param {string} timeRange - Time range
   * @param {number} limit - Number of rankings to return
   * @returns {Promise<Object>}
   */
  async fetchUserBalanceAnalysis(timeRange = '30d', limit = 20) {
    try {
      const response = await API.get(`${BASE_URL}/user-balance-analysis`, {
        params: { time_range: timeRange, limit },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(
        response.data.message || 'Failed to fetch balance analysis'
      );
    } catch (error) {
      showError(error.message || 'Failed to fetch balance analysis');
      throw error;
    }
  },

  /**
   * Export analytics data
   * @param {string} type - Data type to export
   * @param {string} format - Export format: 'csv' or 'json'
   * @param {string} timeRange - Time range
   * @param {number} limit - Number of results
   * @returns {Promise<Blob>}
   */
  async exportData(type, format = 'json', timeRange = '7d', limit = 100) {
    try {
      const response = await API.get(`${BASE_URL}/export`, {
        params: { type, format, time_range: timeRange, limit },
        responseType: 'blob',
      });
      return response.data;
    } catch (error) {
      showError(error.message || 'Failed to export data');
      throw error;
    }
  },

  /**
   * Fetch channel cost analysis
   * @param {string} timeRange - Time range: '1d', '7d', '30d', '90d'
   * @param {number|null} channelId - Optional channel ID filter
   * @returns {Promise<Object>}
   */
  async fetchChannelCostAnalysis(timeRange = '7d', channelId = null) {
    try {
      const params = { time_range: timeRange };
      if (channelId !== null) {
        params.channel_id = channelId;
      }
      const response = await API.get(`${BASE_URL}/channel-cost-analysis`, {
        params,
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(
        response.data.message || 'Failed to fetch channel cost analysis'
      );
    } catch (error) {
      showError(error.message || 'Failed to fetch channel cost analysis');
      throw error;
    }
  },

  /**
   * Fetch cost trend (daily revenue/cost/profit)
   * @param {string} timeRange - Time range
   * @returns {Promise<Object>}
   */
  async fetchCostTrend(timeRange = '7d') {
    try {
      const response = await API.get(`${BASE_URL}/cost-trend`, {
        params: { time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch cost trend');
    } catch (error) {
      showError(error.message || 'Failed to fetch cost trend');
      throw error;
    }
  },

  /**
   * Fetch model profitability analysis
   * @param {string} timeRange - Time range
   * @returns {Promise<Array>}
   */
  async fetchModelProfitability(timeRange = '7d') {
    try {
      const response = await API.get(`${BASE_URL}/model-cost-analysis`, {
        params: { time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(
        response.data.message || 'Failed to fetch model profitability'
      );
    } catch (error) {
      showError(error.message || 'Failed to fetch model profitability');
      throw error;
    }
  },

  /**
   * Fetch user consumption detail with daily trends and model breakdown
   * @param {number} userId - User ID
   * @param {number} days - Number of days to retrieve (default 30, max 90)
   * @returns {Promise<Object>}
   */
  async fetchUserConsumptionDetail(userId, days = 30) {
    try {
      const response = await API.get(`${BASE_URL}/user-consumption-detail/${userId}`, {
        params: { days },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(
        response.data.message || 'Failed to fetch user consumption detail'
      );
    } catch (error) {
      showError(error.message || 'Failed to fetch user consumption detail');
      throw error;
    }
  },

  /**
   * Fetch channel quota analysis (quota-based, doesn't require model_price)
   * @param {string} timeRange - Time range: '1d', '7d', '30d', '90d'
   * @param {number|null} channelId - Optional channel ID filter
   * @returns {Promise<Object>}
   */
  async fetchChannelQuotaAnalysis(timeRange = '7d', channelId = null) {
    try {
      const params = { time_range: timeRange };
      if (channelId !== null) {
        params.channel_id = channelId;
      }
      const response = await API.get(`${BASE_URL}/channel-quota-analysis`, {
        params,
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(
        response.data.message || 'Failed to fetch channel quota analysis'
      );
    } catch (error) {
      showError(error.message || 'Failed to fetch channel quota analysis');
      throw error;
    }
  },

  /**
   * Fetch quota trend (daily quota consumption)
   * @param {string} timeRange - Time range
   * @returns {Promise<Object>}
   */
  async fetchQuotaTrend(timeRange = '7d') {
    try {
      const response = await API.get(`${BASE_URL}/quota-trend`, {
        params: { time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch quota trend');
    } catch (error) {
      showError(error.message || 'Failed to fetch quota trend');
      throw error;
    }
  },

  /**
   * Fetch channel daily quota trend (daily quota consumption by channel)
   * @param {string} timeRange - Time range
   * @returns {Promise<Object>}
   */
  async fetchChannelDailyQuotaTrend(timeRange = '7d') {
    try {
      const response = await API.get(`${BASE_URL}/channel-daily-quota-trend`, {
        params: { time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch channel daily quota trend');
    } catch (error) {
      showError(error.message || 'Failed to fetch channel daily quota trend');
      throw error;
    }
  },
};

export default AnalyticsAPI;
