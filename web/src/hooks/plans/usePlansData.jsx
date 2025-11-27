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

import { useState, useEffect } from 'react';
import { API, showError, showSuccess } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { Modal } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { useTableCompactMode } from '../common/useTableCompactMode';

// Plan types
export const PLAN_TYPES = {
  SUBSCRIPTION: 'subscription',
  CONSUMPTION: 'consumption',
  TRIAL: 'trial',
  ENTERPRISE: 'enterprise',
};

// Plan status
export const PLAN_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
};

export const usePlansData = () => {
  const { t } = useTranslation();

  // Basic state
  const [plans, setPlans] = useState([]);
  const [loading, setLoading] = useState(true);
  const [searching, setSearching] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [totalCount, setTotalCount] = useState(0);
  const [selectedKeys, setSelectedKeys] = useState([]);

  // Edit state
  const [editingPlan, setEditingPlan] = useState({ id: undefined });
  const [showEdit, setShowEdit] = useState(false);

  // Form API
  const [formApi, setFormApi] = useState(null);

  // UI state
  const [compactMode, setCompactMode] = useTableCompactMode('plans');

  // Form state
  const formInitValues = {
    searchKeyword: '',
  };

  // Get form values
  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};
    return {
      searchKeyword: formValues.searchKeyword || '',
    };
  };

  // Load plan list
  const loadPlans = async (page = 1, size = pageSize) => {
    setLoading(true);
    try {
      const res = await API.get(`/api/plan/?p=${page}&page_size=${size}`);
      const { success, message, data } = res.data;
      if (success) {
        const newPageData = data.items || [];
        setActivePage(data.page <= 0 ? 1 : data.page);
        setTotalCount(data.total);
        setPlans(newPageData);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setLoading(false);
  };

  // Search plans
  const searchPlans = async () => {
    const { searchKeyword } = getFormValues();
    if (searchKeyword === '') {
      await loadPlans(1, pageSize);
      return;
    }

    setSearching(true);
    try {
      const res = await API.get(
        `/api/plan/search?keyword=${searchKeyword}&p=1&page_size=${pageSize}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        const newPageData = data.items || [];
        setActivePage(data.page || 1);
        setTotalCount(data.total);
        setPlans(newPageData);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
    setSearching(false);
  };

  // Create plan
  const createPlan = async (planData) => {
    setLoading(true);
    try {
      const res = await API.post('/api/plan/', planData);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('套餐创建成功'));
        await refresh();
        return true;
      } else {
        showError(message);
        return false;
      }
    } catch (error) {
      showError(error.message);
      return false;
    } finally {
      setLoading(false);
    }
  };

  // Update plan
  const updatePlan = async (planData) => {
    setLoading(true);
    try {
      const res = await API.put('/api/plan/', planData);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('套餐更新成功'));
        await refresh();
        return true;
      } else {
        showError(message);
        return false;
      }
    } catch (error) {
      showError(error.message);
      return false;
    } finally {
      setLoading(false);
    }
  };

  // Delete plan
  const deletePlan = async (id) => {
    setLoading(true);
    try {
      const res = await API.delete(`/api/plan/${id}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('套餐删除成功'));
        await refresh();
        return true;
      } else {
        showError(message);
        return false;
      }
    } catch (error) {
      showError(error.message);
      return false;
    } finally {
      setLoading(false);
    }
  };

  // Update plan status
  const updatePlanStatus = async (id, status) => {
    setLoading(true);
    try {
      const res = await API.put(`/api/plan/${id}/status`, { status });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('状态更新成功'));
        // Update local state
        setPlans(plans.map(p => p.id === id ? { ...p, status } : p));
        return true;
      } else {
        showError(message);
        return false;
      }
    } catch (error) {
      showError(error.message);
      return false;
    } finally {
      setLoading(false);
    }
  };

  // Refresh data
  const refresh = async (page = activePage) => {
    const { searchKeyword } = getFormValues();
    if (searchKeyword === '') {
      await loadPlans(page, pageSize);
    } else {
      await searchPlans();
    }
  };

  // Handle page change
  const handlePageChange = (page) => {
    setActivePage(page);
    const { searchKeyword } = getFormValues();
    if (searchKeyword === '') {
      loadPlans(page, pageSize);
    } else {
      searchPlans();
    }
  };

  // Handle page size change
  const handlePageSizeChange = (size) => {
    setPageSize(size);
    setActivePage(1);
    const { searchKeyword } = getFormValues();
    if (searchKeyword === '') {
      loadPlans(1, size);
    } else {
      searchPlans();
    }
  };

  // Row selection configuration
  const rowSelection = {
    onSelect: (record, selected) => {},
    onSelectAll: (selected, selectedRows) => {},
    onChange: (selectedRowKeys, selectedRows) => {
      setSelectedKeys(selectedRows);
    },
  };

  // Row style handling
  const handleRow = (record, index) => {
    if (record.status === PLAN_STATUS.DISABLED) {
      return {
        style: {
          background: 'var(--semi-color-disabled-border)',
        },
      };
    }
    return {};
  };

  // Close edit modal
  const closeEdit = () => {
    setShowEdit(false);
    setTimeout(() => {
      setEditingPlan({ id: undefined });
    }, 500);
  };

  // Initialize data loading
  useEffect(() => {
    loadPlans(1, pageSize).catch((reason) => {
      showError(reason);
    });
  }, []);

  return {
    // Data state
    plans,
    loading,
    searching,
    activePage,
    pageSize,
    totalCount,
    selectedKeys,

    // Edit state
    editingPlan,
    showEdit,

    // Form state
    formApi,
    formInitValues,

    // UI state
    compactMode,
    setCompactMode,

    // Data operations
    loadPlans,
    searchPlans,
    createPlan,
    updatePlan,
    deletePlan,
    updatePlanStatus,
    refresh,

    // State updates
    setActivePage,
    setPageSize,
    setSelectedKeys,
    setEditingPlan,
    setShowEdit,
    setFormApi,
    setLoading,

    // Event handlers
    handlePageChange,
    handlePageSizeChange,
    rowSelection,
    handleRow,
    closeEdit,
    getFormValues,

    // Translation function
    t,
  };
};
