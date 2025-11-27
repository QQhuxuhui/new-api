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

import React, { useState } from 'react';
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
} from '@douyinfe/semi-ui';
import {
  IconPlus,
  IconSearch,
  IconRefresh,
  IconEdit,
  IconDelete,
} from '@douyinfe/semi-icons';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { usePlansData, PLAN_TYPES, PLAN_STATUS } from '../../../hooks/plans/usePlansData';

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
      dataIndex: 'channel_group',
      width: 120,
    },
    {
      title: t('默认额度'),
      dataIndex: 'default_quota',
      width: 120,
      render: (quota) => quota?.toLocaleString() || '0',
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

  // Handle form submit
  const handleSubmit = async () => {
    if (!editFormApi) return;

    const values = editFormApi.getValues();
    let success = false;

    if (editingPlan.id) {
      success = await updatePlan({ ...values, id: editingPlan.id });
    } else {
      success = await createPlan(values);
    }

    if (success) {
      closeEdit();
    }
  };

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
        width={600}
      >
        <Form
          getFormApi={setEditFormApi}
          initValues={editingPlan.id ? editingPlan : {
            status: PLAN_STATUS.ENABLED,
            type: PLAN_TYPES.CONSUMPTION,
            priority: 0,
            default_quota: 0,
            validity_days: 0,
            default_allow_switch: 1,
            default_allow_toggle: 1,
          }}
          labelPosition='left'
          labelWidth={100}
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
          <Form.Input
            field='channel_group'
            label={t('渠道分组')}
            placeholder={t('请输入渠道分组名称')}
          />
          <Form.InputNumber
            field='default_quota'
            label={t('默认额度')}
            placeholder={t('分配给用户的默认额度')}
            min={0}
          />
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
