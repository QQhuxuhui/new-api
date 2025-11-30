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

import React, { useState, useEffect, useCallback } from 'react';
import {
  Button,
  Card,
  Table,
  Tag,
  Space,
  Popconfirm,
  Form,
  Input,
  Modal,
  Select,
  InputNumber,
  TextArea,
  Switch,
  Empty,
  Tooltip,
  ArrayField,
} from '@douyinfe/semi-ui';
import {
  IconPlus,
  IconSearch,
  IconRefresh,
  IconEdit,
  IconDelete,
  IconPlusCircle,
  IconMinusCircle,
} from '@douyinfe/semi-icons';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { usePlansData, PLAN_TYPES, PLAN_STATUS } from '../../../hooks/plans/usePlansData';
import { API, showError } from '../../../helpers';

const PlansTable = () => {
  const {
    plans,
    loading,
    activePage,
    pageSize,
    totalCount,
    editingPlan,
    showEdit,
    formApi,
    formInitValues,
    compactMode,
    createPlan,
    updatePlan,
    deletePlan,
    updatePlanStatus,
    refresh,
    setEditingPlan,
    setShowEdit,
    setFormApi,
    handlePageChange,
    handlePageSizeChange,
    handleRow,
    closeEdit,
    searchPlans,
    t,
  } = usePlansData();

  const [editFormApi, setEditFormApi] = useState(null);
  const [channelGroups, setChannelGroups] = useState([]);
  const [loadingChannelGroups, setLoadingChannelGroups] = useState(false);

  // Load channel groups
  const loadChannelGroups = useCallback(async () => {
    setLoadingChannelGroups(true);
    try {
      const res = await API.get('/api/channel/groups');
      const { success, message, data } = res.data;
      if (success) {
        const groups = data || [];
        setChannelGroups(groups.map(g => ({ label: g, value: g })));
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoadingChannelGroups(false);
  }, []);

  useEffect(() => {
    loadChannelGroups();
  }, [loadChannelGroups]);

  // Plan type options
  const planTypeOptions = [
    { value: PLAN_TYPES.SUBSCRIPTION, label: t('订阅套餐') },
    { value: PLAN_TYPES.CONSUMPTION, label: t('按量付费') },
    { value: PLAN_TYPES.TRIAL, label: t('试用套餐') },
    { value: PLAN_TYPES.ENTERPRISE, label: t('企业套餐') },
  ];

  // Get plan type label
  const getPlanTypeLabel = (type) => {
    const option = planTypeOptions.find(opt => opt.value === type);
    return option ? option.label : type;
  };

  // Get plan type color
  const getPlanTypeColor = (type) => {
    switch (type) {
      case PLAN_TYPES.SUBSCRIPTION:
        return 'blue';
      case PLAN_TYPES.CONSUMPTION:
        return 'green';
      case PLAN_TYPES.TRIAL:
        return 'orange';
      case PLAN_TYPES.ENTERPRISE:
        return 'purple';
      default:
        return 'grey';
    }
  };

  // Render channel groups
  const renderChannelGroups = (record) => {
    try {
      const groups = record.channel_groups ? JSON.parse(record.channel_groups) : [];
      if (groups.length === 0) {
        return <Tag color="grey" size="small">{t('无')}</Tag>;
      }
      return groups.map(g => (
        <Tag key={g} color="blue" size="small" className="mr-1">
          {g}
        </Tag>
      ));
    } catch (e) {
      return record.channel_group ? (
        <Tag color="blue" size="small">{record.channel_group}</Tag>
      ) : (
        <Tag color="grey" size="small">{t('无')}</Tag>
      );
    }
  };

  // Render rate limit rules
  const renderRateLimitRules = (record) => {
    try {
      const rules = record.rate_limit_rules ? JSON.parse(record.rate_limit_rules) : [];
      if (rules.length === 0) {
        return <Tag color="grey" size="small">{t('无限制')}</Tag>;
      }
      return (
        <Tooltip content={
          <div>
            {rules.map((rule, idx) => (
              <div key={idx}>{rule.window_hours}h: ${rule.max_amount}</div>
            ))}
          </div>
        }>
          <Tag color="orange" size="small">{rules.length} {t('条规则')}</Tag>
        </Tooltip>
      );
    } catch (e) {
      return <Tag color="grey" size="small">{t('无限制')}</Tag>;
    }
  };

  // Table columns
  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 80,
    },
    {
      title: t('套餐名称'),
      dataIndex: 'name',
      width: 120,
    },
    {
      title: t('显示名称'),
      dataIndex: 'display_name',
      width: 150,
    },
    {
      title: t('类型'),
      dataIndex: 'type',
      width: 100,
      render: (type) => (
        <Tag color={getPlanTypeColor(type)}>{getPlanTypeLabel(type)}</Tag>
      ),
    },
    {
      title: t('优先级'),
      dataIndex: 'priority',
      width: 80,
    },
    {
      title: t('渠道分组'),
      dataIndex: 'channel_groups',
      width: 200,
      render: (_, record) => renderChannelGroups(record),
    },
    {
      title: t('默认额度'),
      dataIndex: 'default_quota',
      width: 120,
      render: (quota) => quota?.toLocaleString() || '0',
    },
    {
      title: t('每日限额'),
      dataIndex: 'daily_quota_limit',
      width: 120,
      render: (limit) => limit > 0 ? limit.toLocaleString() : <Tag color="grey" size="small">{t('无限制')}</Tag>,
    },
    {
      title: t('速率限制'),
      dataIndex: 'rate_limit_rules',
      width: 120,
      render: (_, record) => renderRateLimitRules(record),
    },
    {
      title: t('有效天数'),
      dataIndex: 'validity_days',
      width: 100,
      render: (days) => days === 0 ? t('永久') : `${days} ${t('天')}`,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (status, record) => (
        <Switch
          checked={status === PLAN_STATUS.ENABLED}
          onChange={(checked) => {
            updatePlanStatus(record.id, checked ? PLAN_STATUS.ENABLED : PLAN_STATUS.DISABLED);
          }}
        />
      ),
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      width: 150,
      fixed: 'right',
      render: (_, record) => (
        <Space>
          <Button
            theme='light'
            type='tertiary'
            icon={<IconEdit />}
            onClick={() => {
              setEditingPlan(record);
              setShowEdit(true);
              // Update formPlanType to match the plan being edited
              setFormPlanType(record.type || PLAN_TYPES.CONSUMPTION);
            }}
          />
          <Popconfirm
            title={t('确定删除此套餐？')}
            content={t('删除后无法恢复')}
            onConfirm={() => deletePlan(record.id)}
          >
            <Button
              theme='light'
              type='danger'
              icon={<IconDelete />}
            />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  // Prepare form initial values
  const getEditFormInitValues = () => {
    if (editingPlan.id) {
      // Parse channel_groups
      let channel_groups_array = [];
      try {
        channel_groups_array = editingPlan.channel_groups ? JSON.parse(editingPlan.channel_groups) : [];
      } catch (e) {
        if (editingPlan.channel_group) {
          channel_groups_array = [editingPlan.channel_group];
        }
      }

      // Parse rate_limit_rules
      let rate_limit_rules_array = [];
      try {
        rate_limit_rules_array = editingPlan.rate_limit_rules ? JSON.parse(editingPlan.rate_limit_rules) : [];
      } catch (e) {
        rate_limit_rules_array = [];
      }

      return {
        ...editingPlan,
        channel_groups: channel_groups_array,
        rate_limit_rules: rate_limit_rules_array,
      };
    }
    return {
      status: PLAN_STATUS.ENABLED,
      type: PLAN_TYPES.CONSUMPTION,
      priority: 0,
      default_quota: 0,
      daily_quota_limit: 0,
      validity_days: 0,
      default_allow_switch: 1,
      default_allow_toggle: 1,
      channel_groups: [],
      rate_limit_rules: [],
    };
  };

  // Handle form submit
  const handleSubmit = async () => {
    if (!editFormApi) return;

    const values = editFormApi.getValues();

    // Transform channel_groups and rate_limit_rules to JSON strings
    const transformedValues = {
      ...values,
      channel_groups: JSON.stringify(values.channel_groups || []),
      rate_limit_rules: JSON.stringify(
        (values.rate_limit_rules || []).filter(r => r && r.window_hours > 0 && r.max_amount > 0)
      ),
    };

    let success = false;

    if (editingPlan.id) {
      success = await updatePlan({ ...transformedValues, id: editingPlan.id });
    } else {
      success = await createPlan(transformedValues);
    }

    if (success) {
      closeEdit();
    }
  };

  // Watch form type change
  const [formPlanType, setFormPlanType] = useState(PLAN_TYPES.CONSUMPTION);

  // Update formPlanType when editing an existing plan
  useEffect(() => {
    if (showEdit && editingPlan && editingPlan.id) {
      // Editing existing plan - set type from plan data
      setFormPlanType(editingPlan.type || PLAN_TYPES.CONSUMPTION);
    } else if (showEdit && (!editingPlan || !editingPlan.id)) {
      // Creating new plan - reset to default
      setFormPlanType(PLAN_TYPES.CONSUMPTION);
    }
  }, [showEdit, editingPlan]);

  return (
    <Card
      title={t('套餐管理')}
      headerExtraContent={
        <Space>
          <Button
            icon={<IconPlus />}
            theme='solid'
            onClick={() => {
              setEditingPlan({ id: undefined });
              setShowEdit(true);
              setFormPlanType(PLAN_TYPES.CONSUMPTION);
            }}
          >
            {t('新建套餐')}
          </Button>
          <Button
            icon={<IconRefresh />}
            onClick={() => refresh()}
          >
            {t('刷新')}
          </Button>
        </Space>
      }
    >
      {/* Search Bar */}
      <div className='mb-4'>
        <Form
          layout='horizontal'
          getFormApi={setFormApi}
          initValues={formInitValues}
        >
          <Form.Input
            field='searchKeyword'
            placeholder={t('搜索套餐名称')}
            prefix={<IconSearch />}
            showClear
            style={{ width: 300 }}
            onEnterPress={searchPlans}
          />
          <Button
            className='ml-2'
            onClick={searchPlans}
          >
            {t('搜索')}
          </Button>
        </Form>
      </div>

      {/* Table */}
      <Table
        columns={columns}
        dataSource={plans}
        rowKey='id'
        loading={loading}
        onRow={handleRow}
        scroll={{ x: 'max-content' }}
        pagination={{
          currentPage: activePage,
          pageSize: pageSize,
          total: totalCount,
          showSizeChanger: true,
          pageSizeOpts: [10, 20, 50, 100],
          onPageSizeChange: handlePageSizeChange,
          onPageChange: handlePageChange,
        }}
        empty={
          <Empty
            image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
            darkModeImage={<IllustrationNoResultDark style={{ width: 150, height: 150 }} />}
            description={t('暂无数据')}
          />
        }
      />

      {/* Edit Modal */}
      <Modal
        title={editingPlan.id ? t('编辑套餐') : t('新建套餐')}
        visible={showEdit}
        onCancel={closeEdit}
        onOk={handleSubmit}
        okText={t('保存')}
        cancelText={t('取消')}
        width={700}
        bodyStyle={{ maxHeight: '70vh', overflowY: 'auto' }}
      >
        <Form
          getFormApi={setEditFormApi}
          initValues={getEditFormInitValues()}
          labelPosition='left'
          labelWidth={120}
          onValueChange={(values) => {
            if (values.type) {
              setFormPlanType(values.type);
            }
          }}
        >
          <Form.Input
            field='name'
            label={t('套餐名称')}
            placeholder={t('请输入套餐名称')}
            rules={[{ required: true, message: t('请输入套餐名称') }]}
          />
          <Form.Input
            field='display_name'
            label={t('显示名称')}
            placeholder={t('请输入显示名称')}
          />
          <Form.Select
            field='type'
            label={t('套餐类型')}
            optionList={planTypeOptions}
            rules={[{ required: true, message: t('请选择套餐类型') }]}
          />
          <Form.InputNumber
            field='priority'
            label={t('优先级')}
            placeholder={t('数值越大优先级越高')}
          />
          <Form.Select
            field='channel_groups'
            label={t('渠道分组')}
            placeholder={t('请选择渠道分组（可多选）')}
            multiple
            maxTagCount={3}
            optionList={channelGroups}
            loading={loadingChannelGroups}
            filter
          />
          <Form.InputNumber
            field='default_quota'
            label={t('默认额度')}
            placeholder={t('分配给用户的默认额度')}
            min={0}
          />

          {/* Daily Quota Limit - only for subscription plans */}
          {formPlanType === PLAN_TYPES.SUBSCRIPTION && (
            <Form.InputNumber
              field='daily_quota_limit'
              label={t('每日限额')}
              placeholder={t('每日最大消费额度（0表示无限制）')}
              min={0}
              suffix={t('（订阅套餐）')}
            />
          )}

          {/* Rate Limit Rules */}
          <Form.Label text={t('速率限制规则')} />
          <ArrayField field='rate_limit_rules'>
            {({ add, arrayFields }) => (
              <div>
                {arrayFields.map((field, index) => (
                  <div key={field.key} className="flex items-center mb-2 gap-2">
                    <Form.InputNumber
                      field={`${field.field}[window_hours]`}
                      placeholder={t('窗口（小时）')}
                      min={1}
                      style={{ width: 150 }}
                      suffix={t('小时')}
                      noLabel
                    />
                    <Form.InputNumber
                      field={`${field.field}[max_amount]`}
                      placeholder={t('最大金额')}
                      min={0}
                      step={10}
                      style={{ width: 150 }}
                      prefix="$"
                      noLabel
                    />
                    <Button
                      icon={<IconMinusCircle />}
                      type="danger"
                      size="small"
                      onClick={field.remove}
                    />
                  </div>
                ))}
                <Button
                  icon={<IconPlusCircle />}
                  onClick={() => add({ window_hours: 1, max_amount: 20 })}
                  size="small"
                >
                  {t('添加规则')}
                </Button>
              </div>
            )}
          </ArrayField>

          <Form.InputNumber
            field='validity_days'
            label={t('有效天数')}
            placeholder={t('0表示永久有效')}
            min={0}
          />
          <Form.Switch
            field='default_allow_switch'
            label={t('允许切换')}
            checkedText={t('是')}
            uncheckedText={t('否')}
          />
          <Form.Switch
            field='default_allow_toggle'
            label={t('允许自动切换')}
            checkedText={t('是')}
            uncheckedText={t('否')}
          />
          <Form.TextArea
            field='description'
            label={t('描述')}
            placeholder={t('请输入套餐描述')}
            rows={3}
          />
        </Form>
      </Modal>
    </Card>
  );
};

export default PlansTable;
