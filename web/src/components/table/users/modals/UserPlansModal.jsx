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

import React, { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  API,
  showError,
  showSuccess,
  renderQuota,
  renderQuotaWithPrompt,
} from '../../../../helpers';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Button,
  Modal,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  Table,
  Switch,
  Select,
  InputNumber,
  Popconfirm,
  Empty,
  Progress,
  Tooltip,
  DatePicker,
  TextArea,
  Checkbox,
  Tabs,
  TabPane,
  Banner,
} from '@douyinfe/semi-ui';
import {
  IconPlus,
  IconLock,
  IconUnlock,
  IconDelete,
  IconEdit,
  IconRefresh,
  IconTick,
  IconBolt,
  IconList,
  IconArrowUp,
  IconArrowDown,
} from '@douyinfe/semi-icons';

const { Text, Title } = Typography;

const UserPlansModal = ({ visible, user, onClose, refresh }) => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [loading, setLoading] = useState(false);
  const [userPlans, setUserPlans] = useState([]);
  const [allPlans, setAllPlans] = useState([]);
  const [showAssignModal, setShowAssignModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState(null);
  const [assignPlanId, setAssignPlanId] = useState(null);
  const [assignQuota, setAssignQuota] = useState(0);
  const [activeTab, setActiveTab] = useState('plans');

  // Daily Pool states
  const [dailyPool, setDailyPool] = useState(null);
  const [showDailyPoolModal, setShowDailyPoolModal] = useState(false);
  const [dailyPoolQuota, setDailyPoolQuota] = useState(0);
  const [dailyPoolMode, setDailyPoolMode] = useState('adjust'); // 'adjust' or 'create'

  // Edit form states
  const [editQuota, setEditQuota] = useState(null);
  const [editExpiresAt, setEditExpiresAt] = useState(null);
  const [editDailyQuotaOverride, setEditDailyQuotaOverride] = useState(null);
  const [editAdminNote, setEditAdminNote] = useState('');
  const [neverExpires, setNeverExpires] = useState(false);
  const [usePlanDefault, setUsePlanDefault] = useState(true);

  // Load user's plans
  const loadUserPlans = useCallback(async () => {
    if (!user?.id) return;
    setLoading(true);
    try {
      const res = await API.get(`/api/user_plan/user/${user.id}`);
      const { success, message, data } = res.data;
      if (success) {
        setUserPlans(data || []);
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  }, [user?.id]);

  // Load all available plans
  const loadAllPlans = useCallback(async () => {
    try {
      const res = await API.get('/api/plan/');
      const { success, data } = res.data;
      if (success) {
        // API returns paginated data, extract items array
        setAllPlans(data?.items || []);
      }
    } catch (e) {
      // Silent fail for plan list
    }
  }, []);

  // Load user's daily pool
  const loadDailyPool = useCallback(async () => {
    if (!user?.id) return;
    try {
      const res = await API.get(`/api/daily_pool/user/${user.id}`);
      const { success, data } = res.data;
      if (success) {
        setDailyPool(data);
      }
    } catch (e) {
      // Silent fail - user may not have daily pool
      setDailyPool(null);
    }
  }, [user?.id]);

  useEffect(() => {
    if (visible && user?.id) {
      loadUserPlans();
      loadAllPlans();
      loadDailyPool();
    }
  }, [visible, user?.id, loadUserPlans, loadAllPlans, loadDailyPool]);

  // Assign plan to user
  const handleAssignPlan = async () => {
    if (!assignPlanId) {
      showError(t('请选择套餐'));
      return;
    }
    setLoading(true);
    try {
      const res = await API.post('/api/user_plan/assign', {
        user_id: user.id,
        plan_id: assignPlanId,
        quota: assignQuota || 0,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('套餐分配成功'));
        setShowAssignModal(false);
        setAssignPlanId(null);
        setAssignQuota(0);
        loadUserPlans();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  };

  // Remove plan from user
  const handleRemovePlan = async (userPlan) => {
    setLoading(true);
    try {
      // Use DELETE API with user_plan_id to support deleted plan templates
      const res = await API.delete(`/api/user_plan/${userPlan.id}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('套餐移除成功'));
        loadUserPlans();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  };

  // Create or adjust daily pool
  const handleDailyPoolSubmit = async () => {
    setLoading(true);
    try {
      const endpoint = `/api/daily_pool/user/${user.id}`;
      const method = dailyPoolMode === 'create' ? 'post' : 'put';

      // POST expects 'total_quota', PUT expects 'amount'
      const payload = dailyPoolMode === 'create'
        ? { total_quota: dailyPoolQuota }
        : { amount: dailyPoolQuota };

      const res = await API[method](endpoint, payload);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t(dailyPoolMode === 'create' ? '日卡池创建成功' : '日卡池调整成功'));
        setShowDailyPoolModal(false);
        setDailyPoolQuota(0);
        loadDailyPool();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  };

  // Revoke a plan (move to queue or remove)
  const handleRevokePlan = async (userPlan) => {
    setLoading(true);
    try {
      const res = await API.post(`/api/user_plan/${userPlan.id}/revoke`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('套餐已撤销'));
        loadUserPlans();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  };

  // Toggle permission
  const handleTogglePermission = async (userPlanId, field, value) => {
    setLoading(true);
    try {
      const res = await API.put(`/api/user_plan/${userPlanId}/permissions`, {
        [field]: value ? 1 : 0,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('权限更新成功'));
        loadUserPlans();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  };

  // Lock/unlock plan
  const handleToggleLock = async (userPlan) => {
    setLoading(true);
    const endpoint = userPlan.locked === 1 ? 'unlock' : 'lock';
    try {
      const res = await API.post(`/api/user_plan/${userPlan.id}/${endpoint}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t(userPlan.locked === 1 ? '套餐已解锁' : '套餐已锁定'));
        loadUserPlans();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  };

  // Force switch current plan
  const handleForceSwitch = async (userPlanId, planId) => {
    setLoading(true);
    try {
      const res = await API.post('/api/user_plan/force_switch', {
        user_id: user.id,
        plan_id: planId,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('已切换当前套餐'));
        loadUserPlans();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  };

  // Edit user plan (comprehensive)
  const handleEditPlan = async () => {
    if (!selectedPlan) return;

    // Validation: if not never-expires, must have a valid expiration date
    if (!neverExpires && !editExpiresAt) {
      showError(t('请选择过期时间或勾选"永不过期"'));
      return;
    }

    setLoading(true);
    try {
      const payload = {};
      let needsClearDailyQuotaOverride = false;

      // Only include changed fields
      if (editQuota !== null && editQuota !== selectedPlan.quota) {
        payload.quota = editQuota;
      }

      // Handle expiration - check if never-expires state changed or date changed
      // Backend stores expires_at in milliseconds
      const currentNeverExpires = selectedPlan.expires_at === 0;
      if (neverExpires !== currentNeverExpires) {
        // Never-expires state changed
        payload.expires_at = neverExpires ? 0 : new Date(editExpiresAt).getTime();
      } else if (!neverExpires && editExpiresAt) {
        // Not never-expires, check if date changed
        const newTimestamp = new Date(editExpiresAt).getTime();
        if (newTimestamp !== selectedPlan.expires_at) {
          payload.expires_at = newTimestamp;
        }
      }

      // Handle daily quota override
      const hadOverride = selectedPlan.daily_quota_limit_override !== null;
      if (usePlanDefault && hadOverride) {
        // User wants to clear override - need to call DELETE API
        needsClearDailyQuotaOverride = true;
      } else if (!usePlanDefault) {
        // User wants to set override
        const newOverride = editDailyQuotaOverride || 0;
        if (newOverride !== selectedPlan.daily_quota_limit_override) {
          payload.daily_quota_limit_override = newOverride;
        }
      }

      // Handle admin note - allow clearing (empty string)
      if (editAdminNote !== (selectedPlan.admin_note || '')) {
        payload.admin_note = editAdminNote;
      }

      if (Object.keys(payload).length === 0 && !needsClearDailyQuotaOverride) {
        showError(t('没有修改任何内容'));
        setLoading(false);
        return;
      }

      // Update main fields if any
      if (Object.keys(payload).length > 0) {
        const res = await API.put(`/api/user_plan/${selectedPlan.id}`, payload);
        const { success, message } = res.data;
        if (!success) {
          showError(message);
          setLoading(false);
          return;
        }
      }

      // Clear daily quota override if needed
      if (needsClearDailyQuotaOverride) {
        const res = await API.delete(`/api/user_plan/${selectedPlan.id}/daily_quota_override`);
        const { success, message } = res.data;
        if (!success) {
          showError(message);
          setLoading(false);
          return;
        }
      }

      showSuccess(t('用户套餐更新成功'));
      setShowEditModal(false);
      resetEditForm();
      loadUserPlans();
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  };

  // Reset edit form
  const resetEditForm = () => {
    setSelectedPlan(null);
    setEditQuota(null);
    setEditExpiresAt(null);
    setEditDailyQuotaOverride(null);
    setEditAdminNote('');
    setNeverExpires(false);
    setUsePlanDefault(true);
  };

  // Open edit modal with current values
  const openEditModal = (record) => {
    setSelectedPlan(record);
    setEditQuota(record.quota);

    // Handle expiration date initialization
    const isNeverExpires = record.expires_at === 0;
    if (isNeverExpires) {
      // If currently never-expires, default date to 30 days from now (for when user unchecks)
      const defaultDate = new Date();
      defaultDate.setDate(defaultDate.getDate() + 30);
      setEditExpiresAt(defaultDate);
    } else {
      // Backend stores expires_at in milliseconds, use it directly
      setEditExpiresAt(new Date(record.expires_at));
    }
    setNeverExpires(isNeverExpires);

    setEditDailyQuotaOverride(record.daily_quota_limit_override || 0);
    setUsePlanDefault(record.daily_quota_limit_override === null);
    setEditAdminNote(record.admin_note || '');
    setShowEditModal(true);
  };

  // Get available plans for assignment (exclude already assigned)
  const getAvailablePlans = () => {
    const assignedPlanIds = userPlans.map((up) => up.plan_id);
    return allPlans
      .filter((p) => !assignedPlanIds.includes(p.id) && p.status === 1)
      .map((p) => ({
        label: `${p.name} (${t(p.type)})`,
        value: p.id,
      }));
  };

  // Render plan type tag
  const renderPlanType = (type) => {
    const typeColors = {
      subscription: 'blue',
      consumption: 'green',
      trial: 'orange',
      enterprise: 'purple',
    };
    return (
      <Tag color={typeColors[type] || 'grey'} size="small">
        {t(type)}
      </Tag>
    );
  };

  // Render quota progress
  const renderQuotaProgress = (userPlan) => {
    const used = parseInt(userPlan.used_quota) || 0;
    const remain = parseInt(userPlan.quota) || 0;
    const total = used + remain;
    const percent = total > 0 ? (remain / total) * 100 : 0;
    return (
      <Tooltip content={`${t('已用')}: ${renderQuota(used)} / ${t('总计')}: ${renderQuota(total)}`}>
        <div style={{ minWidth: 100 }}>
          <div className="text-xs">{renderQuota(remain)} / {renderQuota(total)}</div>
          <Progress percent={percent} size="small" showInfo={false} />
        </div>
      </Tooltip>
    );
  };

  // Table columns
  const columns = [
    {
      title: t('套餐名称'),
      dataIndex: 'plan_display_name',
      render: (text, record) => (
        <Space>
          {/* 优先使用快照数据，fallback 到实时数据 */}
          {text || record.plan_name || record.plan?.display_name || record.plan?.name || t('未知套餐')}
          {record.is_current === 1 && (
            <Tag color="green" size="small">{t('当前')}</Tag>
          )}
          {record.locked === 1 && (
            <Tag color="red" size="small">{t('已锁定')}</Tag>
          )}
        </Space>
      ),
    },
    {
      title: t('类型'),
      dataIndex: 'plan_type',
      render: (text, record) => {
        // 优先使用快照数据，fallback 到实时数据
        const type = text || record.plan?.type;
        return renderPlanType(type);
      },
    },
    {
      title: t('额度'),
      key: 'quota',
      render: (_, record) => renderQuotaProgress(record),
    },
    {
      title: t('优先级'),
      dataIndex: 'plan_priority',
      render: (text, record) => {
        // 优先使用快照数据，fallback 到实时数据
        return text !== undefined ? text : record.plan?.priority;
      },
      width: 80,
    },
    {
      title: t('队列位置'),
      dataIndex: 'queue_position',
      width: 100,
      render: (value, record) => {
        if (record.is_current === 1) {
          return <Tag color='green' size='small'>{t('当前使用中')}</Tag>;
        }
        if (value > 0) {
          return <Tag color='blue' size='small'>#{value}</Tag>;
        }
        return <Tag color='grey' size='small'>{t('未排队')}</Tag>;
      },
    },
    {
      title: t('允许切换'),
      dataIndex: 'can_switch',
      width: 100,
      render: (value, record) => (
        <Switch
          checked={value === 1}
          onChange={(checked) =>
            handleTogglePermission(record.id, 'can_switch', checked)
          }
          size="small"
        />
      ),
    },
    {
      title: t('允许自动切换'),
      dataIndex: 'can_toggle_auto',
      width: 120,
      render: (value, record) => (
        <Switch
          checked={value === 1}
          onChange={(checked) =>
            handleTogglePermission(record.id, 'can_toggle_auto', checked)
          }
          size="small"
        />
      ),
    },
    {
      title: t('自动切换'),
      dataIndex: 'auto_switch',
      width: 100,
      render: (value, record) => (
        <Switch
          checked={value === 1}
          onChange={(checked) =>
            handleTogglePermission(record.id, 'auto_switch', checked)
          }
          size="small"
        />
      ),
    },
    {
      title: t('操作'),
      key: 'actions',
      width: 280,
      render: (_, record) => (
        <Space wrap>
          {record.is_current !== 1 && (
            <Popconfirm
              title={t('确认切换为当前套餐？')}
              onConfirm={() => handleForceSwitch(record.id, record.plan_id)}
            >
              <Button size="small" icon={<IconTick />} type="primary">
                {t('设为当前')}
              </Button>
            </Popconfirm>
          )}
          <Button
            size="small"
            icon={<IconEdit />}
            onClick={() => openEditModal(record)}
          >
            {t('编辑')}
          </Button>
          <Button
            size="small"
            icon={record.locked === 1 ? <IconUnlock /> : <IconLock />}
            type={record.locked === 1 ? 'primary' : 'warning'}
            onClick={() => handleToggleLock(record)}
          >
            {record.locked === 1 ? t('解锁') : t('锁定')}
          </Button>
          {record.is_current === 1 && (
            <Popconfirm
              title={t('确认撤销当前套餐？')}
              content={t('撤销后将自动切换到下一个排队套餐')}
              onConfirm={() => handleRevokePlan(record)}
            >
              <Button size="small" icon={<IconArrowDown />} type="tertiary">
                {t('撤销')}
              </Button>
            </Popconfirm>
          )}
          <Popconfirm
            title={t('确认移除该套餐？')}
            onConfirm={() => handleRemovePlan(record)}
          >
            <Button size="small" icon={<IconDelete />} type="danger">
              {t('移除')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <>
      <Modal
        title={
          <Space>
            <Tag color="blue" shape="circle">
              {t('套餐管理')}
            </Tag>
            <Title heading={4} className="m-0">
              {user?.username || t('用户')} - {t('套餐管理')}
            </Title>
          </Space>
        }
        visible={visible}
        onCancel={onClose}
        footer={
          <Space>
            <Button
              theme="solid"
              onClick={() => setShowAssignModal(true)}
              icon={<IconPlus />}
            >
              {t('分配套餐')}
            </Button>
            <Button
              type="tertiary"
              onClick={() => loadUserPlans()}
              icon={<IconRefresh />}
            >
              {t('刷新')}
            </Button>
            <Button
              type="primary"
              onClick={onClose}
            >
              {t('关闭')}
            </Button>
          </Space>
        }
        style={{ width: isMobile ? '95%' : '90%', maxWidth: 1400 }}
        bodyStyle={{ maxHeight: '70vh', overflowY: 'auto' }}
      >
        <Spin spinning={loading}>
          <div>
            <Tabs activeKey={activeTab} onChange={setActiveTab}>
              <TabPane tab={<span><IconList className="mr-1" />{t('套餐管理')}</span>} itemKey="plans">
                <Card className="!rounded-2xl shadow-sm border-0 mt-4">
                  <div className="flex items-center justify-between mb-4">
                    <Text className="text-lg font-medium">
                      {t('用户套餐列表')}
                    </Text>
                    <Text type="secondary" className="text-sm">
                      {t('共')} {userPlans.length} {t('个套餐')}
                    </Text>
                  </div>

                  {userPlans.length > 0 ? (
                    <Table
                      columns={columns}
                      dataSource={userPlans}
                      rowKey="id"
                      pagination={false}
                      size="small"
                      scroll={{ x: 'max-content' }}
                    />
                  ) : (
                    <Empty description={t('该用户尚未分配任何套餐')} />
                  )}
                </Card>
              </TabPane>

              <TabPane tab={<span><IconBolt className="mr-1" />{t('日卡池')}</span>} itemKey="daily_pool">
                <Card className="!rounded-2xl shadow-sm border-0 mt-4">
                  <div className="flex items-center justify-between mb-4">
                    <Text className="text-lg font-medium">
                      {t('今日日卡池')}
                    </Text>
                    {dailyPool ? (
                      <Button
                        size="small"
                        icon={<IconEdit />}
                        onClick={() => {
                          setDailyPoolMode('adjust');
                          setDailyPoolQuota(0);
                          setShowDailyPoolModal(true);
                        }}
                      >
                        {t('调整额度')}
                      </Button>
                    ) : (
                      <Button
                        size="small"
                        type="primary"
                        icon={<IconPlus />}
                        onClick={() => {
                          setDailyPoolMode('create');
                          setDailyPoolQuota(0);
                          setShowDailyPoolModal(true);
                        }}
                      >
                        {t('创建日卡池')}
                      </Button>
                    )}
                  </div>

                  {dailyPool ? (
                    <div className="space-y-4">
                      <div className="grid grid-cols-3 gap-4">
                        <div className="p-4 bg-blue-50 dark:bg-blue-900/20 rounded-xl text-center">
                          <Text type="secondary" size="small" className="block mb-1">{t('总额度')}</Text>
                          <Text strong className="text-lg">{renderQuota(dailyPool.total_quota)}</Text>
                        </div>
                        <div className="p-4 bg-orange-50 dark:bg-orange-900/20 rounded-xl text-center">
                          <Text type="secondary" size="small" className="block mb-1">{t('已使用')}</Text>
                          <Text strong className="text-lg text-orange-600">{renderQuota(dailyPool.used_quota)}</Text>
                        </div>
                        <div className="p-4 bg-green-50 dark:bg-green-900/20 rounded-xl text-center">
                          <Text type="secondary" size="small" className="block mb-1">{t('剩余')}</Text>
                          <Text strong className="text-lg text-green-600">{renderQuota(dailyPool.total_quota - dailyPool.used_quota)}</Text>
                        </div>
                      </div>
                      <Progress
                        percent={dailyPool.total_quota > 0 ? ((dailyPool.total_quota - dailyPool.used_quota) / dailyPool.total_quota * 100) : 0}
                        showInfo
                        format={(percent) => `${percent.toFixed(1)}%`}
                        stroke={dailyPool.total_quota > 0 && ((dailyPool.total_quota - dailyPool.used_quota) / dailyPool.total_quota) > 0.2 ? 'var(--semi-color-success)' : 'var(--semi-color-danger)'}
                      />
                      <div className="flex justify-between text-sm">
                        <Text type="tertiary">{t('日期')}: {dailyPool.date}</Text>
                        <Text type="tertiary">{t('有效期至')}: 23:59:59</Text>
                      </div>
                    </div>
                  ) : (
                    <Empty
                      description={t('该用户今日没有日卡池')}
                      image={<IconBolt size="extra-large" style={{ color: 'var(--semi-color-text-2)' }} />}
                    />
                  )}
                </Card>
              </TabPane>
            </Tabs>
          </div>
        </Spin>
      </Modal>

      {/* Assign Plan Modal */}
      <Modal
        title={t('分配套餐')}
        visible={showAssignModal}
        onOk={handleAssignPlan}
        onCancel={() => {
          setShowAssignModal(false);
          setAssignPlanId(null);
          setAssignQuota(0);
        }}
        confirmLoading={loading}
      >
        <div className="space-y-4">
          <div>
            <Text className="block mb-2">{t('选择套餐')}</Text>
            <Select
              placeholder={t('请选择要分配的套餐')}
              value={assignPlanId}
              onChange={setAssignPlanId}
              optionList={getAvailablePlans()}
              style={{ width: '100%' }}
            />
          </div>
          <div>
            <Text className="block mb-2">{t('初始额度')}</Text>
            <InputNumber
              placeholder={t('输入初始额度（可选，不填则使用套餐默认额度）')}
              value={assignQuota}
              onChange={setAssignQuota}
              style={{ width: '100%' }}
              step={500000}
              min={0}
            />
            {assignQuota > 0 && (
              <Text type="secondary" className="text-xs mt-1 block">
                {renderQuotaWithPrompt(assignQuota)}
              </Text>
            )}
          </div>
        </div>
      </Modal>

      {/* Edit Plan Modal */}
      <Modal
        title={t('编辑用户套餐')}
        visible={showEditModal}
        onOk={handleEditPlan}
        onCancel={() => {
          setShowEditModal(false);
          resetEditForm();
        }}
        confirmLoading={loading}
        style={{ width: 600 }}
      >
        {selectedPlan && (
          <div className="space-y-4">
            {/* Plan Info */}
            <div className="bg-gray-50 dark:bg-gray-800 p-3 rounded">
              <Text type="secondary">{t('套餐')}: </Text>
              {/* 优先使用快照数据 */}
              <Text strong>{selectedPlan.plan_display_name || selectedPlan.plan_name || selectedPlan.plan?.display_name || selectedPlan.plan?.name}</Text>
              <Text type="secondary" className="ml-4">{t('类型')}: </Text>
              {/* 优先使用快照数据 */}
              {renderPlanType(selectedPlan.plan_type || selectedPlan.plan?.type)}
            </div>

            {/* Quota */}
            <div>
              <Text className="block mb-2">{t('套餐额度')}</Text>
              <InputNumber
                placeholder={t('输入套餐额度')}
                value={editQuota}
                onChange={setEditQuota}
                style={{ width: '100%' }}
                step={500000}
                min={0}
              />
              {editQuota > 0 && (
                <Text type="secondary" className="text-xs mt-1 block">
                  {renderQuotaWithPrompt(editQuota)}
                </Text>
              )}
            </div>

            {/* Expiration */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <Text>{t('过期时间')}</Text>
                <Checkbox
                  checked={neverExpires}
                  onChange={(e) => {
                    setNeverExpires(e.target.checked);
                    if (e.target.checked) {
                      // Keep the date value but hide the picker
                      // Don't set to null to maintain the fallback date
                    } else if (!editExpiresAt) {
                      // If somehow the date is null, set default to 30 days from now
                      const defaultDate = new Date();
                      defaultDate.setDate(defaultDate.getDate() + 30);
                      setEditExpiresAt(defaultDate);
                    }
                  }}
                >
                  {t('永不过期')}
                </Checkbox>
              </div>
              {!neverExpires && (
                <>
                  <DatePicker
                    type="dateTime"
                    value={editExpiresAt}
                    onChange={setEditExpiresAt}
                    style={{ width: '100%' }}
                    placeholder={t('选择过期时间')}
                  />
                  {selectedPlan.expires_at === 0 && (
                    <Text type="warning" className="text-xs mt-1 block">
                      {t('原套餐为永不过期，已自动设置为30天后，可修改')}
                    </Text>
                  )}
                </>
              )}
              {selectedPlan.expires_at > 0 && (
                <Text type="secondary" className="text-xs mt-1 block">
                  {t('当前过期时间')}: {new Date(selectedPlan.expires_at).toLocaleString()}
                </Text>
              )}
            </div>

            {/* Daily Quota Override */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <Text>{t('每日限额覆盖')}</Text>
                <Checkbox
                  checked={usePlanDefault}
                  onChange={(e) => {
                    setUsePlanDefault(e.target.checked);
                    if (!e.target.checked && editDailyQuotaOverride === null) {
                      setEditDailyQuotaOverride(0);
                    }
                  }}
                >
                  {t('使用套餐默认值')}
                </Checkbox>
              </div>
              {!usePlanDefault && (
                <>
                  <InputNumber
                    placeholder={t('输入每日限额（0 = 无限制）')}
                    value={editDailyQuotaOverride}
                    onChange={setEditDailyQuotaOverride}
                    style={{ width: '100%' }}
                    step={500000}
                    min={0}
                  />
                  {editDailyQuotaOverride > 0 && (
                    <Text type="secondary" className="text-xs mt-1 block">
                      {renderQuotaWithPrompt(editDailyQuotaOverride)}
                    </Text>
                  )}
                  {editDailyQuotaOverride === 0 && (
                    <Text type="warning" className="text-xs mt-1 block">
                      {t('设置为 0 表示无每日限额限制')}
                    </Text>
                  )}
                </>
              )}
              {selectedPlan.plan?.daily_quota_limit && (
                <Text type="secondary" className="text-xs mt-1 block">
                  {t('套餐默认每日限额')}: {renderQuota(selectedPlan.plan.daily_quota_limit)}
                </Text>
              )}
            </div>

            {/* Admin Note */}
            <div>
              <Text className="block mb-2">{t('管理员备注')}</Text>
              <TextArea
                placeholder={t('输入管理员备注（可选）')}
                value={editAdminNote}
                onChange={setEditAdminNote}
                rows={3}
                maxLength={500}
                showClear
              />
            </div>
          </div>
        )}
      </Modal>

      {/* Daily Pool Modal */}
      <Modal
        title={dailyPoolMode === 'create' ? t('创建日卡池') : t('调整日卡池额度')}
        visible={showDailyPoolModal}
        onOk={handleDailyPoolSubmit}
        onCancel={() => {
          setShowDailyPoolModal(false);
          setDailyPoolQuota(0);
        }}
        confirmLoading={loading}
      >
        <div className="space-y-4">
          {dailyPoolMode === 'create' ? (
            <Banner
              type="info"
              description={t('为用户创建今日日卡池。日卡池将在今日 23:59:59 过期。')}
            />
          ) : (
            <Banner
              type="info"
              description={t('调整日卡池额度（正数增加，负数减少）')}
            />
          )}

          {dailyPool && dailyPoolMode === 'adjust' && (
            <div className="p-3 bg-gray-50 dark:bg-gray-800 rounded">
              <Text type="secondary">{t('当前总额度')}: </Text>
              <Text strong>{renderQuota(dailyPool.total_quota)}</Text>
              <br />
              <Text type="secondary">{t('当前剩余')}: </Text>
              <Text strong>{renderQuota(dailyPool.total_quota - dailyPool.used_quota)}</Text>
            </div>
          )}

          <div>
            <Text className="block mb-2">
              {dailyPoolMode === 'create' ? t('初始额度') : t('调整额度')}
            </Text>
            <InputNumber
              placeholder={dailyPoolMode === 'create' ? t('输入初始额度') : t('输入调整额度（正负数）')}
              value={dailyPoolQuota}
              onChange={setDailyPoolQuota}
              style={{ width: '100%' }}
              step={500000}
            />
            {dailyPoolQuota !== 0 && (
              <Text type="secondary" className="text-xs mt-1 block">
                {renderQuotaWithPrompt(Math.abs(dailyPoolQuota))}
                {dailyPoolMode === 'adjust' && dailyPoolQuota < 0 && (
                  <Text type="warning" className="ml-2">{t('将减少额度')}</Text>
                )}
              </Text>
            )}
          </div>
        </div>
      </Modal>
    </>
  );
};

export default UserPlansModal;
