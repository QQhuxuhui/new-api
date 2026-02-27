import { useState, useCallback } from 'react';
import { API, showError } from '../../helpers';

const PRESET_RANGES = {
  today: () => {
    const now = new Date();
    const start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    return [Math.floor(start.getTime() / 1000), Math.floor(now.getTime() / 1000)];
  },
  thisWeek: () => {
    const now = new Date();
    const day = now.getDay() || 7;
    const start = new Date(now.getFullYear(), now.getMonth(), now.getDate() - day + 1);
    return [Math.floor(start.getTime() / 1000), Math.floor(now.getTime() / 1000)];
  },
  thisMonth: () => {
    const now = new Date();
    const start = new Date(now.getFullYear(), now.getMonth(), 1);
    return [Math.floor(start.getTime() / 1000), Math.floor(now.getTime() / 1000)];
  },
  last7Days: () => {
    const now = new Date();
    const start = new Date(now.getTime() - 7 * 24 * 3600 * 1000);
    return [Math.floor(start.getTime() / 1000), Math.floor(now.getTime() / 1000)];
  },
  last30Days: () => {
    const now = new Date();
    const start = new Date(now.getTime() - 30 * 24 * 3600 * 1000);
    return [Math.floor(start.getTime() / 1000), Math.floor(now.getTime() / 1000)];
  },
};

export function useTokenAnalytics() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [activeRange, setActiveRange] = useState('last7Days');
  const [customRange, setCustomRange] = useState(null);

  const fetchStats = useCallback(async (startTimestamp, endTimestamp) => {
    setLoading(true);
    try {
      const res = await API.get(
        `/api/token/stats?start_timestamp=${startTimestamp}&end_timestamp=${endTimestamp}`
      );
      const { success, message, data: resData } = res.data || {};
      if (success) {
        setData(resData);
      } else {
        showError(message || 'Failed to fetch token stats');
      }
    } catch (e) {
      showError(e.message || 'Failed to fetch token stats');
    } finally {
      setLoading(false);
    }
  }, []);

  const selectPresetRange = useCallback(
    (rangeKey) => {
      setActiveRange(rangeKey);
      setCustomRange(null);
      const rangeFn = PRESET_RANGES[rangeKey];
      if (rangeFn) {
        const [start, end] = rangeFn();
        fetchStats(start, end);
      }
    },
    [fetchStats]
  );

  const selectCustomRange = useCallback(
    (dates) => {
      if (!dates || dates.length < 2) return;
      setActiveRange(null);
      const start = Math.floor(new Date(dates[0]).getTime() / 1000);
      const end = Math.floor(new Date(dates[1]).getTime() / 1000) + 86399; // end of day
      setCustomRange(dates);
      fetchStats(start, end);
    },
    [fetchStats]
  );

  const initLoad = useCallback(() => {
    selectPresetRange('last7Days');
  }, [selectPresetRange]);

  return {
    data,
    loading,
    activeRange,
    customRange,
    selectPresetRange,
    selectCustomRange,
    initLoad,
  };
}
