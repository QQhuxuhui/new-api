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

import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { API, isAdmin, showError, timestamp2string } from '../../helpers';
import { getDefaultTime, getInitialTimestamp } from '../../helpers/dashboard';
import { TIME_OPTIONS, QUICK_DATE_PRESETS } from '../../constants/dashboard.constants';
import { useIsMobile } from '../common/useIsMobile';
import { useMinimumLoadingTime } from '../common/useMinimumLoadingTime';
import { useDebouncedCallback } from 'use-debounce';

export const useDashboardData = (userState, userDispatch, statusState) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const isMobile = useIsMobile();
  const initialized = useRef(false);

  // ========== 基础状态 ==========
  const [loading, setLoading] = useState(false);
  const [greetingVisible, setGreetingVisible] = useState(false);
  const [searchModalVisible, setSearchModalVisible] = useState(false);
  const [activePreset, setActivePreset] = useState(null);
  const showLoading = useMinimumLoadingTime(loading);

  // ========== 输入状态 ==========
  const [inputs, setInputs] = useState({
    username: '',
    token_name: '',
    model_name: '',
    start_timestamp: getInitialTimestamp(),
    end_timestamp: timestamp2string(new Date().getTime() / 1000 + 3600),
    channel: '',
    data_export_default_time: '',
  });
  const [usernameOptions, setUsernameOptions] = useState([]);
  const [usernameSearchLoading, setUsernameSearchLoading] = useState(false);

  const [dataExportDefaultTime, setDataExportDefaultTime] =
    useState(getDefaultTime());

  // ========== 数据状态 ==========
  const [quotaData, setQuotaData] = useState([]);
  const [subscriptionData, setSubscriptionData] = useState(null);
  const [subscriptionLoading, setSubscriptionLoading] = useState(true);
  const [subscriptionError, setSubscriptionError] = useState('');
  const [quotaStatus, setQuotaStatus] = useState(null);
  const [consumeQuota, setConsumeQuota] = useState(0);
  const [consumeTokens, setConsumeTokens] = useState(0);
  const [times, setTimes] = useState(0);
  const [pieData, setPieData] = useState([{ type: 'null', value: '0' }]);
  const [lineData, setLineData] = useState([]);
  const [modelColors, setModelColors] = useState({});

  // ========== 图表状态 ==========
  const [activeChartTab, setActiveChartTab] = useState('1');

  // ========== 趋势数据 ==========
  const [trendData, setTrendData] = useState({
    balance: [],
    usedQuota: [],
    requestCount: [],
    times: [],
    consumeQuota: [],
    tokens: [],
    rpm: [],
    tpm: [],
  });

  // ========== 今日数据 ==========
  const [todayConsumeQuota, setTodayConsumeQuota] = useState(0);
  const [todayConsumeTokens, setTodayConsumeTokens] = useState(0);
  const [todayTimes, setTodayTimes] = useState(0);
  const [todayLoading, setTodayLoading] = useState(false);

  // ========== Uptime 数据 ==========
  const [uptimeData, setUptimeData] = useState([]);
  const [uptimeLoading, setUptimeLoading] = useState(false);
  const [activeUptimeTab, setActiveUptimeTab] = useState('');

  // ========== 常量 ==========
  const now = new Date();
  const isAdminUser = isAdmin();

  // ========== Panel enable flags ==========
  const apiInfoEnabled = statusState?.status?.api_info_enabled ?? true;
  const announcementsEnabled =
    statusState?.status?.announcements_enabled ?? true;
  const faqEnabled = statusState?.status?.faq_enabled ?? true;
  const uptimeEnabled = statusState?.status?.uptime_kuma_enabled ?? true;

  const hasApiInfoPanel = apiInfoEnabled;
  const hasInfoPanels = announcementsEnabled || apiInfoEnabled || uptimeEnabled;

  // ========== Memoized Values ==========
  const timeOptions = useMemo(
    () =>
      TIME_OPTIONS.map((option) => ({
        ...option,
        label: t(option.label),
      })),
    [t],
  );

  const performanceMetrics = useMemo(() => {
    const { start_timestamp, end_timestamp } = inputs;
    const timeDiff =
      (Date.parse(end_timestamp) - Date.parse(start_timestamp)) / 60000;
    const avgRPM = isNaN(times / timeDiff)
      ? '0'
      : (times / timeDiff).toFixed(3);
    const avgTPM = isNaN(consumeTokens / timeDiff)
      ? '0'
      : (consumeTokens / timeDiff).toFixed(3);

    return { avgRPM, avgTPM, timeDiff };
  }, [times, consumeTokens, inputs.start_timestamp, inputs.end_timestamp]);

  const getGreeting = useMemo(() => {
    const hours = new Date().getHours();
    let greeting = '';

    if (hours >= 5 && hours < 12) {
      greeting = t('早上好');
    } else if (hours >= 12 && hours < 14) {
      greeting = t('中午好');
    } else if (hours >= 14 && hours < 18) {
      greeting = t('下午好');
    } else {
      greeting = t('晚上好');
    }

    const username = userState?.user?.username || '';
    return `👋${greeting}，${username}`;
  }, [t, userState?.user?.username]);

  // ========== 回调函数 ==========
  const handleInputChange = useCallback((value, name) => {
    if (name === 'data_export_default_time') {
      setDataExportDefaultTime(value);
      localStorage.setItem('data_export_default_time', value);
      return;
    }
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  }, []);

  const showSearchModal = useCallback(() => {
    setSearchModalVisible(true);
  }, []);

  const handleCloseModal = useCallback(() => {
    setSearchModalVisible(false);
  }, []);

  // ========== 快捷筛选处理函数 ==========
  const handlePresetChange = useCallback((presetKey) => {
    const preset = QUICK_DATE_PRESETS.find((p) => p.key === presetKey);
    if (!preset) return;

    const now = new Date();
    let startDate, endDate;

    if (preset.type === 'month') {
      // 本月: 从本月1号到现在
      startDate = new Date(now.getFullYear(), now.getMonth(), 1);
      endDate = new Date(now.getTime());
    } else if (preset.days === 0) {
      // 今天: 从今天0点到现在
      startDate = new Date(now.getFullYear(), now.getMonth(), now.getDate());
      endDate = new Date(now.getTime() + 3600 * 1000);
    } else if (preset.days === -1) {
      // 昨天: 从昨天0点到昨天23:59:59
      startDate = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1);
      endDate = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 0, 0, 0);
    } else {
      // 近N天: 从N天前0点到现在
      startDate = new Date(now.getFullYear(), now.getMonth(), now.getDate() + preset.days);
      endDate = new Date(now.getTime() + 3600 * 1000);
    }

    setActivePreset(presetKey);
    setInputs((prev) => ({
      ...prev,
      start_timestamp: timestamp2string(startDate.getTime() / 1000),
      end_timestamp: timestamp2string(endDate.getTime() / 1000),
    }));
    setDataExportDefaultTime(preset.defaultGranularity);
    localStorage.setItem('data_export_default_time', preset.defaultGranularity);
  }, []);

  const handleDateRangeChange = useCallback((startValue, endValue) => {
    setActivePreset(null); // 清除预设选中状态
    setInputs((prev) => ({
      ...prev,
      start_timestamp: startValue,
      end_timestamp: endValue,
    }));

    // 自动调整时间粒度
    if (startValue && endValue) {
      const diffMs = Date.parse(endValue) - Date.parse(startValue);
      const diffDays = diffMs / (1000 * 60 * 60 * 24);
      const newGranularity = diffDays <= 2 ? 'hour' : 'day';
      setDataExportDefaultTime(newGranularity);
      localStorage.setItem('data_export_default_time', newGranularity);
    }
  }, []);

  const handleGranularityChange = useCallback((value) => {
    setDataExportDefaultTime(value);
    localStorage.setItem('data_export_default_time', value);
  }, []);

  const handleUsernameChange = useCallback((value) => {
    setInputs((prev) => ({ ...prev, username: value }));
  }, []);

  const fetchUsernameOptions = useDebouncedCallback(
    async (keyword) => {
      if (!isAdminUser) return;
      const trimmedKeyword = (keyword ?? '').toString().trim();
      if (!trimmedKeyword) {
        setUsernameOptions([]);
        setUsernameSearchLoading(false);
        return;
      }
      setUsernameSearchLoading(true);
      try {
        const res = await API.get(
          `/api/user/search?keyword=${encodeURIComponent(trimmedKeyword)}&p=1&page_size=20`,
        );
        const { success, message, data } = res.data;
        if (success) {
          const options = (data?.items || []).map((u) => ({
            label: u?.username || '',
            value: u?.username || '',
          }));
          setUsernameOptions(options.filter((o) => o.value));
        } else {
          showError(message);
        }
      } catch (err) {
        showError(err?.message || t('操作失败，请重试'));
      } finally {
        setUsernameSearchLoading(false);
      }
    },
    300,
    { leading: false, trailing: true },
  );

  const handleUsernameSearch = useCallback(
    (value) => {
      fetchUsernameOptions(value);
    },
    [fetchUsernameOptions],
  );

  // ========== API 调用函数 ==========
  const loadQuotaData = useCallback(async () => {
    setLoading(true);
    try {
      let url = '';
      const { start_timestamp, end_timestamp, username } = inputs;
      let localStartTimestamp = Date.parse(start_timestamp) / 1000;
      let localEndTimestamp = Date.parse(end_timestamp) / 1000;

      if (isAdminUser) {
        url = `/api/data/?username=${username}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&default_time=${dataExportDefaultTime}`;
      } else {
        url = `/api/data/self/?start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&default_time=${dataExportDefaultTime}`;
      }

      const res = await API.get(url);
      const { success, message, data } = res.data;
      if (success) {
        setQuotaData(data);
        if (data.length === 0) {
          data.push({
            count: 0,
            model_name: '无数据',
            quota: 0,
            created_at: now.getTime() / 1000,
          });
        }
        data.sort((a, b) => a.created_at - b.created_at);
        return data;
      } else {
        showError(message);
        return [];
      }
    } finally {
      setLoading(false);
    }
  }, [inputs, dataExportDefaultTime, isAdminUser, now]);

  const loadUptimeData = useCallback(async () => {
    setUptimeLoading(true);
    try {
      const res = await API.get('/api/uptime/status');
      const { success, message, data } = res.data;
      if (success) {
        setUptimeData(data || []);
        if (data && data.length > 0 && !activeUptimeTab) {
          setActiveUptimeTab(data[0].categoryName);
        }
      } else {
        showError(message);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setUptimeLoading(false);
    }
  }, [activeUptimeTab]);

  const getUserData = useCallback(async () => {
    let res = await API.get(`/api/user/self`);
    const { success, message, data } = res.data;
    if (success) {
      userDispatch({ type: 'login', payload: data });
    } else {
      showError(message);
    }
  }, [userDispatch]);

  const loadSubscriptionData = useCallback(async () => {
    setSubscriptionLoading(true);
    setSubscriptionError('');
    try {
      const res = await API.get('/api/my_plans/');
      const { success, message, data } = res.data;
      if (success) {
        setSubscriptionData(data ?? null);
      } else {
        setSubscriptionError(message || t('订阅信息加载失败'));
      }
    } catch (err) {
      setSubscriptionError(err?.message || t('订阅信息加载失败'));
    } finally {
      setSubscriptionLoading(false);
    }
  }, [t]);

  const loadQuotaStatus = useCallback(async () => {
    try {
      const res = await API.get('/api/my_plans/quota-status');
      const { success, data } = res.data;
      if (success && data) {
        setQuotaStatus(data);
      }
    } catch (err) {
      console.error('Error loading quota status:', err);
    }
  }, []);

  const loadTodayData = useCallback(async () => {
    setTodayLoading(true);
    try {
      const todayStart = new Date();
      todayStart.setHours(0, 0, 0, 0);
      const todayEnd = new Date();
      todayEnd.setHours(23, 59, 59, 999);
      const startTs = Math.floor(todayStart.getTime() / 1000);
      const endTs = Math.floor(todayEnd.getTime() / 1000);

      let url = '';
      if (isAdminUser) {
        url = `/api/data/?username=${inputs.username}&start_timestamp=${startTs}&end_timestamp=${endTs}&default_time=hour`;
      } else {
        url = `/api/data/self/?start_timestamp=${startTs}&end_timestamp=${endTs}&default_time=hour`;
      }

      const res = await API.get(url);
      const { success, data } = res.data;
      if (success && data && data.length > 0) {
        let totalQuota = 0;
        let totalTokens = 0;
        let totalTimes = 0;
        for (const item of data) {
          totalQuota += item.quota || 0;
          totalTokens += item.token_used || 0;
          totalTimes += item.count || 0;
        }
        setTodayConsumeQuota(totalQuota);
        setTodayConsumeTokens(totalTokens);
        setTodayTimes(totalTimes);
      } else {
        setTodayConsumeQuota(0);
        setTodayConsumeTokens(0);
        setTodayTimes(0);
      }
    } catch (err) {
      console.error('Error loading today data:', err);
    } finally {
      setTodayLoading(false);
    }
  }, [isAdminUser, inputs.username]);

  const refresh = useCallback(async () => {
    const data = await loadQuotaData();
    await loadUptimeData();
    await loadSubscriptionData();
    await loadQuotaStatus();
    await loadTodayData();
    return data;
  }, [loadQuotaData, loadUptimeData, loadSubscriptionData, loadQuotaStatus, loadTodayData]);

  const handleSearchConfirm = useCallback(
    async (updateChartDataCallback) => {
      const data = await refresh();
      if (data && data.length > 0 && updateChartDataCallback) {
        updateChartDataCallback(data);
      }
      setSearchModalVisible(false);
    },
    [refresh],
  );

  // ========== Effects ==========
  useEffect(() => {
    const timer = setTimeout(() => {
      setGreetingVisible(true);
    }, 100);
    return () => clearTimeout(timer);
  }, []);

  useEffect(() => {
    if (!initialized.current) {
      getUserData();
      loadSubscriptionData();
      loadQuotaStatus();
      loadTodayData();
      initialized.current = true;
    }
  }, [getUserData, loadSubscriptionData, loadQuotaStatus, loadTodayData]);

  return {
    // 基础状态
    loading: showLoading,
    greetingVisible,
    searchModalVisible,
    activePreset,

    // 输入状态
    inputs,
    dataExportDefaultTime,

    // 数据状态
    quotaData,
    subscriptionData,
    subscriptionLoading,
    subscriptionError,
    quotaStatus,
    consumeQuota,
    setConsumeQuota,
    consumeTokens,
    setConsumeTokens,
    times,
    setTimes,
    pieData,
    setPieData,
    lineData,
    setLineData,
    modelColors,
    setModelColors,

    // 图表状态
    activeChartTab,
    setActiveChartTab,

    // 趋势数据
    trendData,
    setTrendData,

    // Uptime 数据
    uptimeData,
    uptimeLoading,
    activeUptimeTab,
    setActiveUptimeTab,

    // 今日数据
    todayConsumeQuota,
    todayConsumeTokens,
    todayTimes,
    todayLoading,

    // 计算值
    timeOptions,
    performanceMetrics,
    getGreeting,
    isAdminUser,
    hasApiInfoPanel,
    hasInfoPanels,
    apiInfoEnabled,
    announcementsEnabled,
    faqEnabled,
    uptimeEnabled,

    // 函数
    handleInputChange,
    showSearchModal,
    handleCloseModal,
    handlePresetChange,
    handleDateRangeChange,
    handleGranularityChange,
    handleUsernameChange,
    handleUsernameSearch,
    loadQuotaData,
    loadUptimeData,
    getUserData,
    refresh,
    handleSearchConfirm,
    usernameOptions,
    usernameSearchLoading,

    // 导航和翻译
    navigate,
    t,
    isMobile,
  };
};
