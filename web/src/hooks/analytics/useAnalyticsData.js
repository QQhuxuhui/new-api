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

import { useState, useCallback, useEffect } from 'react';
import { AnalyticsAPI } from '../../services/analyticsApi';

/**
 * Custom hook for fetching and managing analytics data
 */
export const useAnalyticsData = (initialTimeRange = '7d') => {
  const [timeRange, setTimeRange] = useState(initialTimeRange);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  // Data states
  const [userOverview, setUserOverview] = useState(null);
  const [activeUsers, setActiveUsers] = useState([]);
  const [consumptionTrend, setConsumptionTrend] = useState([]);
  const [topSpenders, setTopSpenders] = useState([]);
  const [modelUsage, setModelUsage] = useState([]);
  const [behaviorPatterns, setBehaviorPatterns] = useState(null);
  const [riskIndicators, setRiskIndicators] = useState([]);

  // Fetch all analytics data
  const fetchAllData = useCallback(async (range = timeRange) => {
    setLoading(true);
    setError(null);

    try {
      const [
        overview,
        active,
        consumption,
        spenders,
        models,
        behavior,
        risks
      ] = await Promise.all([
        AnalyticsAPI.fetchUserOverview(range).catch(() => null),
        AnalyticsAPI.fetchActiveUsers(range, 20).catch(() => []),
        AnalyticsAPI.fetchConsumptionTrend(range).catch(() => []),
        AnalyticsAPI.fetchTopSpenders(range, 20).catch(() => []),
        AnalyticsAPI.fetchModelUsage(range).catch(() => []),
        AnalyticsAPI.fetchBehaviorPatterns(range).catch(() => null),
        AnalyticsAPI.fetchRiskIndicators(range).catch(() => [])
      ]);

      setUserOverview(overview);
      setActiveUsers(active);
      setConsumptionTrend(consumption);
      setTopSpenders(spenders);
      setModelUsage(models);
      setBehaviorPatterns(behavior);
      setRiskIndicators(risks);
    } catch (err) {
      setError(err.message || 'Failed to fetch analytics data');
    } finally {
      setLoading(false);
    }
  }, [timeRange]);

  // Fetch individual data sections
  const fetchUserOverview = useCallback(async (range = timeRange) => {
    try {
      const data = await AnalyticsAPI.fetchUserOverview(range);
      setUserOverview(data);
      return data;
    } catch (err) {
      setError(err.message);
      return null;
    }
  }, [timeRange]);

  const fetchActiveUsers = useCallback(async (range = timeRange, limit = 20) => {
    try {
      const data = await AnalyticsAPI.fetchActiveUsers(range, limit);
      setActiveUsers(data);
      return data;
    } catch (err) {
      setError(err.message);
      return [];
    }
  }, [timeRange]);

  const fetchModelUsage = useCallback(async (range = timeRange) => {
    try {
      const data = await AnalyticsAPI.fetchModelUsage(range);
      setModelUsage(data);
      return data;
    } catch (err) {
      setError(err.message);
      return [];
    }
  }, [timeRange]);

  // Change time range and refetch
  const handleTimeRangeChange = useCallback((newRange) => {
    setTimeRange(newRange);
    fetchAllData(newRange);
  }, [fetchAllData]);

  // Export data
  const exportData = useCallback(async (type, format = 'json') => {
    try {
      const blob = await AnalyticsAPI.exportData(type, format, timeRange);
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `analytics_${type}_${timeRange}.${format}`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (err) {
      setError(err.message);
    }
  }, [timeRange]);

  // Initial fetch
  useEffect(() => {
    fetchAllData();
  }, []);

  return {
    // State
    timeRange,
    loading,
    error,
    userOverview,
    activeUsers,
    consumptionTrend,
    topSpenders,
    modelUsage,
    behaviorPatterns,
    riskIndicators,

    // Actions
    setTimeRange: handleTimeRangeChange,
    refreshData: () => fetchAllData(timeRange),
    fetchUserOverview,
    fetchActiveUsers,
    fetchModelUsage,
    exportData,
  };
};

export default useAnalyticsData;
