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

const BASE_URL = '/api/admin/analytics/plan-usage';

/**
 * Plan Usage Analytics API Service
 */
export const PlanUsageAPI = {
  /**
   * Fetch plan usage overview statistics
   * @param {string} timeRange - Time range: '1d', '7d', '30d', '90d'
   * @returns {Promise<Object>}
   */
  async fetchPlanUsageOverview(timeRange = '30d') {
    try {
      const response = await API.get(`${BASE_URL}/overview`, {
        params: { time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch plan usage overview');
    } catch (error) {
      showError(error.message || 'Failed to fetch plan usage overview');
      throw error;
    }
  },

  /**
   * Fetch paginated list of user plans with usage stats
   * @param {Object} filters - Filter parameters
   * @param {number} filters.user_id - Filter by user ID
   * @param {string} filters.plan_type - Filter by plan type
   * @param {string} filters.status - Filter by status
   * @param {string} filters.time_range - Time range for usage data
   * @param {number} page - Page number (1-based)
   * @param {number} pageSize - Items per page
   * @returns {Promise<Object>}
   */
  async fetchPlanUsageList(filters = {}, page = 1, pageSize = 25) {
    try {
      const params = {
        ...filters,
        page,
        page_size: pageSize,
      };
      const response = await API.get(`${BASE_URL}/list`, { params });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch plan usage list');
    } catch (error) {
      showError(error.message || 'Failed to fetch plan usage list');
      throw error;
    }
  },

  /**
   * Fetch plan type distribution
   * @param {string} timeRange - Time range: '1d', '7d', '30d', '90d'
   * @returns {Promise<Array>}
   */
  async fetchPlanTypeDistribution(timeRange = '30d') {
    try {
      const response = await API.get(`${BASE_URL}/type-distribution`, {
        params: { time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch plan type distribution');
    } catch (error) {
      showError(error.message || 'Failed to fetch plan type distribution');
      throw error;
    }
  },

  /**
   * Fetch top consuming plans ranking
   * @param {number} limit - Number of results (default 10)
   * @param {string} timeRange - Time range: '1d', '7d', '30d', '90d'
   * @returns {Promise<Array>}
   */
  async fetchPlanConsumptionRanking(limit = 10, timeRange = '30d') {
    try {
      const response = await API.get(`${BASE_URL}/consumption-ranking`, {
        params: { limit, time_range: timeRange },
      });
      if (response.data.success) {
        return response.data.data;
      }
      throw new Error(response.data.message || 'Failed to fetch plan consumption ranking');
    } catch (error) {
      showError(error.message || 'Failed to fetch plan consumption ranking');
      throw error;
    }
  },
};

export default PlanUsageAPI;
