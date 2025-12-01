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
 * Custom hook for fetching and managing channel cost analytics data
 */
export const useChannelCostData = (initialTimeRange = '7d') => {
  const [timeRange, setTimeRange] = useState(initialTimeRange);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  // Data states
  const [channelCostData, setChannelCostData] = useState(null);
  const [costTrendData, setCostTrendData] = useState(null);
  const [modelProfitabilityData, setModelProfitabilityData] = useState([]);

  // Fetch all cost analytics data
  const fetchAllCostData = useCallback(
    async (range = timeRange, channelId = null) => {
      setLoading(true);
      setError(null);

      try {
        const [channelCost, costTrend, modelProfit] = await Promise.all([
          AnalyticsAPI.fetchChannelCostAnalysis(range, channelId).catch(
            () => null
          ),
          AnalyticsAPI.fetchCostTrend(range).catch(() => null),
          AnalyticsAPI.fetchModelProfitability(range).catch(() => []),
        ]);

        setChannelCostData(channelCost);
        setCostTrendData(costTrend);
        setModelProfitabilityData(modelProfit);
      } catch (err) {
        setError(err.message || 'Failed to fetch cost analytics data');
      } finally {
        setLoading(false);
      }
    },
    [timeRange]
  );

  // Fetch individual data sections
  const fetchChannelCostAnalysis = useCallback(
    async (range = timeRange, channelId = null) => {
      try {
        const data = await AnalyticsAPI.fetchChannelCostAnalysis(
          range,
          channelId
        );
        setChannelCostData(data);
        return data;
      } catch (err) {
        setError(err.message);
        return null;
      }
    },
    [timeRange]
  );

  const fetchCostTrend = useCallback(
    async (range = timeRange) => {
      try {
        const data = await AnalyticsAPI.fetchCostTrend(range);
        setCostTrendData(data);
        return data;
      } catch (err) {
        setError(err.message);
        return null;
      }
    },
    [timeRange]
  );

  const fetchModelProfitability = useCallback(
    async (range = timeRange) => {
      try {
        const data = await AnalyticsAPI.fetchModelProfitability(range);
        setModelProfitabilityData(data);
        return data;
      } catch (err) {
        setError(err.message);
        return [];
      }
    },
    [timeRange]
  );

  // Change time range and refetch
  const handleTimeRangeChange = useCallback(
    (newRange) => {
      setTimeRange(newRange);
      fetchAllCostData(newRange);
    },
    [fetchAllCostData]
  );

  // Sync with parent timeRange prop
  useEffect(() => {
    if (initialTimeRange !== timeRange) {
      setTimeRange(initialTimeRange);
      fetchAllCostData(initialTimeRange);
    }
  }, [initialTimeRange]);

  // Initial fetch
  useEffect(() => {
    fetchAllCostData();
  }, []);

  return {
    // State
    timeRange,
    loading,
    error,
    channelCostData,
    costTrendData,
    modelProfitabilityData,

    // Actions
    setTimeRange: handleTimeRangeChange,
    refreshData: () => fetchAllCostData(timeRange),
    fetchChannelCostAnalysis,
    fetchCostTrend,
    fetchModelProfitability,
  };
};

export default useChannelCostData;
