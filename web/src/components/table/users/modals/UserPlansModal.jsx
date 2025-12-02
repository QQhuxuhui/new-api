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
  SideSheet,
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
} from '@douyinfe/semi-ui';
import {
  IconPlus,
  IconClose,
  IconLock,
  IconUnlock,
  IconDelete,
  IconEdit,
  IconRefresh,
  IconTick,
} from '@douyinfe/semi-icons';

const { Text, Title } = Typography;

const UserPlansModal = ({ visible, user, onClose, refresh }) => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [loading, setLoading] = useState(false);
  const [userPlans, setUserPlans] = useState([]);
  const [allPlans, setAllPlans] = useState([]);
  const [showAssignModal, setShowAssignModal] = useState(false);
  const [showAdjustQuotaModal, setShowAdjustQuotaModal] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState(null);
  const [assignPlanId, setAssignPlanId] = useState(null);
  const [assignQuota, setAssignQuota] = useState(0);
  const [adjustQuotaAmount, setAdjustQuotaAmount] = useState(0);

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

  useEffect(() => {
    if (visible && user?.id) {
      loadUserPlans();
      loadAllPlans();
    }
  }, [visible, user?.id, loadUserPlans, loadAllPlans]);

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
      const res = await API.post('/api/user_plan/remove', {
        user_id: user.id,
        plan_id: userPlan.plan_id,
      });
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

  // Adjust quota
  const handleAdjustQuota = async () => {
    if (!selectedPlan) return;
    setLoading(true);
    try {
      const res = await API.put(`/api/user_plan/${selectedPlan.id}/quota`, {
        amount: adjustQuotaAmount,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('额度调整成功'));
        setShowAdjustQuotaModal(false);
        setSelectedPlan(null);
        setAdjustQuotaAmount(0);
        loadUserPlans();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
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
    const total = parseInt(userPlan.quota) || 0;
    const remain = total - used;
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
      dataIndex: ['plan', 'name'],
      render: (text, record) => (
        <Space>
          {text || t('未知套餐')}
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
      dataIndex: ['plan', 'type'],
      render: (text) => renderPlanType(text),
    },
    {
      title: t('额度'),
      key: 'quota',
      render: (_, record) => renderQuotaProgress(record),
    },
    {
      title: t('优先级'),
      dataIndex: ['plan', 'priority'],
      width: 80,
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
      width: 200,
      render: (_, record) => (
        <Space>
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
            onClick={() => {
              setSelectedPlan(record);
              setAdjustQuotaAmount(0);
              setShowAdjustQuotaModal(true);
            }}
          >
            {t('调整额度')}
          </Button>
          <Button
            size="small"
            icon={record.locked === 1 ? <IconUnlock /> : <IconLock />}
            type={record.locked === 1 ? 'primary' : 'warning'}
            onClick={() => handleToggleLock(record)}
          >
            {record.locked === 1 ? t('解锁') : t('锁定')}
          </Button>
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
      <SideSheet
        placement="right"
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
        bodyStyle={{ padding: 0 }}
        visible={visible}
        width={isMobile ? '100%' : 900}
        footer={
          <div className="flex justify-end bg-white">
            <Space>
              <Button
                theme="solid"
                onClick={() => setShowAssignModal(true)}
                icon={<IconPlus />}
              >
                {t('分配套餐')}
              </Button>
              <Button
                theme="light"
                type="tertiary"
                onClick={() => loadUserPlans()}
                icon={<IconRefresh />}
              >
                {t('刷新')}
              </Button>
              <Button
                theme="light"
                type="primary"
                onClick={onClose}
                icon={<IconClose />}
              >
                {t('关闭')}
              </Button>
            </Space>
          </div>
        }
        closeIcon={null}
        onCancel={onClose}
      >
        <Spin spinning={loading}>
          <div className="p-4">
            <Card className="!rounded-2xl shadow-sm border-0">
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
          </div>
        </Spin>
      </SideSheet>

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

      {/* Adjust Quota Modal */}
      <Modal
        title={t('调整额度')}
        visible={showAdjustQuotaModal}
        onOk={handleAdjustQuota}
        onCancel={() => {
          setShowAdjustQuotaModal(false);
          setSelectedPlan(null);
          setAdjustQuotaAmount(0);
        }}
        confirmLoading={loading}
      >
        {selectedPlan && (
          <div className="space-y-4">
            <div>
              <Text type="secondary">{t('当前套餐')}: </Text>
              <Text strong>{selectedPlan.plan?.name}</Text>
            </div>
            <div>
              <Text type="secondary">{t('当前额度')}: </Text>
              <Text>{renderQuota(selectedPlan.quota)}</Text>
            </div>
            <div>
              <Text className="block mb-2">{t('调整数量（正数增加，负数减少）')}</Text>
              <InputNumber
                placeholder={t('输入调整数量')}
                value={adjustQuotaAmount}
                onChange={setAdjustQuotaAmount}
                style={{ width: '100%' }}
                step={500000}
              />
              {adjustQuotaAmount !== 0 && (() => {
                const prompt = renderQuotaWithPrompt(Math.abs(adjustQuotaAmount));
                return (
                  <Text type="secondary" className="text-xs mt-1 block">
                    {prompt && `${prompt} | `}
                    {t('调整后')}: {renderQuota((selectedPlan.quota || 0) + (adjustQuotaAmount || 0))}
                  </Text>
                );
              })()}
            </div>
          </div>
        )}
      </Modal>
    </>
  );
};

export default UserPlansModal;
