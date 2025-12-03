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
import { PlanUsageAPI } from '../../services/planUsageApi';

/**
 * Custom hook for fetching and managing plan usage data
 */
export const usePlanUsageData = () => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  // Data states
  const [overview, setOverview] = useState(null);
  const [planList, setPlanList] = useState({ items: [], total: 0, page: 1, page_size: 25, total_pages: 0 });
  const [distribution, setDistribution] = useState([]);
  const [ranking, setRanking] = useState([]);

  // Filters state
  const [filters, setFilters] = useState({});
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);

  // Time range state
  const [timeRange, setTimeRange] = useState('30d');

  // Fetch overview data
  const fetchOverview = useCallback(async (currentTimeRange = timeRange) => {
    try {
      const data = await PlanUsageAPI.fetchPlanUsageOverview(currentTimeRange);
      setOverview(data);
      return data;
    } catch (err) {
      setError(err.message);
      return null;
    }
  }, [timeRange]);

  // Fetch plan usage list
  const fetchPlanList = useCallback(async (currentFilters = filters, currentPage = page, currentPageSize = pageSize, currentTimeRange = timeRange) => {
    setLoading(true);
    try {
      // Add time_range to filters
      const filtersWithTimeRange = {
        ...currentFilters,
        time_range: currentTimeRange,
      };
      const data = await PlanUsageAPI.fetchPlanUsageList(filtersWithTimeRange, currentPage, currentPageSize);
      setPlanList(data);
      return data;
    } catch (err) {
      setError(err.message);
      return null;
    } finally {
      setLoading(false);
    }
  }, [filters, page, pageSize, timeRange]);

  // Fetch distribution
  const fetchDistribution = useCallback(async (currentTimeRange = timeRange) => {
    try {
      const data = await PlanUsageAPI.fetchPlanTypeDistribution(currentTimeRange);
      setDistribution(data);
      return data;
    } catch (err) {
      setError(err.message);
      return [];
    }
  }, [timeRange]);

  // Fetch ranking
  const fetchRanking = useCallback(async (limit = 10, currentTimeRange = timeRange) => {
    try {
      const data = await PlanUsageAPI.fetchPlanConsumptionRanking(limit, currentTimeRange);
      setRanking(data);
      return data;
    } catch (err) {
      setError(err.message);
      return [];
    }
  }, [timeRange]);

  // Fetch all data
  const fetchAllData = useCallback(async (currentTimeRange = timeRange) => {
    setLoading(true);
    setError(null);

    try {
      await Promise.all([
        fetchOverview(currentTimeRange),
        fetchPlanList(filters, page, pageSize, currentTimeRange),
        fetchDistribution(currentTimeRange),
        fetchRanking(10, currentTimeRange),
      ]);
    } catch (err) {
      setError(err.message || 'Failed to fetch plan usage data');
    } finally {
      setLoading(false);
    }
  }, [timeRange, filters, page, pageSize, fetchOverview, fetchPlanList, fetchDistribution, fetchRanking]);

  // Update filters and refetch
  const updateFilters = useCallback((newFilters) => {
    setFilters(newFilters);
    setPage(1); // Reset to first page when filters change
    // Immediately fetch with new filters
    fetchPlanList(newFilters, 1, pageSize);
  }, [pageSize, fetchPlanList]);

  // Update page and refetch
  const updatePage = useCallback((newPage) => {
    setPage(newPage);
    // Immediately fetch with new page
    fetchPlanList(filters, newPage, pageSize);
  }, [filters, pageSize, fetchPlanList]);

  // Refresh all data
  const refreshData = useCallback(() => {
    fetchAllData();
  }, [fetchAllData]);

  // Update time range and refetch
  const updateTimeRange = useCallback((newTimeRange) => {
    setTimeRange(newTimeRange);
    // Pass new time range directly to avoid stale closure
    fetchAllData(newTimeRange);
  }, [fetchAllData]);

  return {
    // Data
    overview,
    planList,
    distribution,
    ranking,

    // State
    loading,
    error,
    filters,
    page,
    pageSize,
    timeRange,

    // Actions
    fetchOverview,
    fetchPlanList,
    fetchDistribution,
    fetchRanking,
    fetchAllData,
    updateFilters,
    updatePage,
    updateTimeRange,
    refreshData,
  };
};

export default usePlanUsageData;
