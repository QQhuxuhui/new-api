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

import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Table,
  Tag,
  Button,
  Typography,
  Input,
  Select,
  Space,
  Modal,
  Toast,
  Spin,
  Empty,
} from '@douyinfe/semi-ui';
import {
  IconRefresh,
  IconSearch,
  IconShoppingBag,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';

const { Title, Text } = Typography;

const AdminOrders = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [orders, setOrders] = useState([]);
  const [pagination, setPagination] = useState({
    currentPage: 1,
    pageSize: 20,
    total: 0,
  });

  // Filters
  const [filters, setFilters] = useState({
    status: '',
    userId: '',
    orderNo: '',
  });

  // Load orders
  const loadOrders = async (page = 1) => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        page: page.toString(),
        page_size: pagination.pageSize.toString(),
      });

      if (filters.status) params.append('status', filters.status);
      if (filters.userId) params.append('user_id', filters.userId);
      if (filters.orderNo) params.append('order_no', filters.orderNo);

      const res = await API.get(`/api/user/admin/plan-orders?${params.toString()}`);
      const { success, message, data } = res.data;
      if (success && data) {
        setOrders(data.orders || []);
        setPagination({
          ...pagination,
          currentPage: data.page || page,
          total: data.total || 0,
        });
      } else {
        showError(message || t('加载失败'));
      }
    } catch (e) {
      showError(e.message || t('网络错误'));
    }
    setLoading(false);
  };

  useEffect(() => {
    loadOrders(1);
  }, []);

  // Handle search
  const handleSearch = () => {
    loadOrders(1);
  };

  // Handle reset filters
  const handleResetFilters = () => {
    setFilters({
      status: '',
      userId: '',
      orderNo: '',
    });
    setTimeout(() => loadOrders(1), 100);
  };

  // Handle manual completion
  const handleManualComplete = async (orderId) => {
    Modal.confirm({
      title: t('确认手动完成订单'),
      content: t('确认要手动完成该订单并发放套餐吗？此操作不可撤销。'),
      onOk: async () => {
        try {
          const res = await API.post(`/api/user/admin/plan-orders/${orderId}/complete`);
          const { success, message } = res.data;
          if (success) {
            showSuccess(t('操作成功'));
            loadOrders(pagination.currentPage);
          } else {
            showError(message || t('操作失败'));
          }
        } catch (e) {
          showError(e.message || t('网络错误'));
        }
      },
    });
  };

  // Get status tag
  const getStatusTag = (status) => {
    const statusConfig = {
      pending: { color: 'amber', text: t('待支付') },
      paid: { color: 'blue', text: t('已支付') },
      delivered: { color: 'green', text: t('已完成') },
      expired: { color: 'grey', text: t('已过期') },
      cancelled: { color: 'red', text: t('已取消') },
    };
    const config = statusConfig[status] || { color: 'grey', text: status };
    return <Tag color={config.color}>{config.text}</Tag>;
  };

  // Table columns
  const columns = [
    {
      title: t('订单号'),
      dataIndex: 'order_no',
      key: 'order_no',
      width: 180,
      render: (text) => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{text}</span>,
    },
    {
      title: t('用户'),
      key: 'user',
      width: 150,
      render: (_, record) => (
        <div>
          <Text strong>{record.username || `ID: ${record.user_id}`}</Text>
          {record.user_email && (
            <Text type='tertiary' size='small' className='block'>{record.user_email}</Text>
          )}
        </div>
      ),
    },
    {
      title: t('套餐'),
      dataIndex: 'plan_name',
      key: 'plan_name',
      width: 120,
    },
    {
      title: t('金额'),
      dataIndex: 'final_price',
      key: 'final_price',
      width: 100,
      render: (price) => (
        <span style={{ fontWeight: 600 }}>
          ¥{price?.toFixed(2) || '0.00'}
        </span>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status) => getStatusTag(status),
    },
    {
      title: t('支付方式'),
      dataIndex: 'payment_method',
      key: 'payment_method',
      width: 100,
      render: (method) => method || '-',
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (time) => (time ? timestamp2string(time / 1000) : '-'),
    },
    {
      title: t('支付时间'),
      dataIndex: 'paid_at',
      key: 'paid_at',
      width: 160,
      render: (time) => (time ? timestamp2string(time / 1000) : '-'),
    },
    {
      title: t('发放时间'),
      dataIndex: 'delivered_at',
      key: 'delivered_at',
      width: 160,
      render: (time) => (time ? timestamp2string(time / 1000) : '-'),
    },
    {
      title: t('重试次数'),
      dataIndex: 'delivery_retry_count',
      key: 'delivery_retry_count',
      width: 100,
      render: (count) => count || 0,
    },
    {
      title: t('操作'),
      key: 'action',
      width: 120,
      fixed: 'right',
      render: (_, record) => {
        if (record.status === 'paid' && record.user_plan_id === 0) {
          return (
            <Button
              size='small'
              type='warning'
              onClick={() => handleManualComplete(record.order_id)}
            >
              {t('手动完成')}
            </Button>
          );
        }
        return '-';
      },
    },
  ];

  // Handle page change
  const handlePageChange = (page) => {
    loadOrders(page);
  };

  return (
    <div className='p-6'>
      <Card>
        {/* Header */}
        <div className='flex items-center justify-between mb-6'>
          <Title heading={3} className='m-0'>
            {t('订单管理')}
          </Title>
          <Button
            icon={<IconRefresh />}
            onClick={() => loadOrders(pagination.currentPage)}
            loading={loading}
          >
            {t('刷新')}
          </Button>
        </div>

        {/* Filters */}
        <div className='mb-6'>
          <Space spacing='medium' wrap>
            <Select
              placeholder={t('全部状态')}
              style={{ width: 150 }}
              value={filters.status}
              onChange={(value) => setFilters({ ...filters, status: value })}
              showClear
            >
              <Select.Option value='pending'>{t('待支付')}</Select.Option>
              <Select.Option value='paid'>{t('已支付')}</Select.Option>
              <Select.Option value='delivered'>{t('已完成')}</Select.Option>
              <Select.Option value='expired'>{t('已过期')}</Select.Option>
              <Select.Option value='cancelled'>{t('已取消')}</Select.Option>
            </Select>

            <Input
              placeholder={t('用户ID')}
              style={{ width: 150 }}
              value={filters.userId}
              onChange={(value) => setFilters({ ...filters, userId: value })}
            />

            <Input
              placeholder={t('订单号')}
              style={{ width: 200 }}
              value={filters.orderNo}
              onChange={(value) => setFilters({ ...filters, orderNo: value })}
            />

            <Button
              theme='solid'
              type='primary'
              icon={<IconSearch />}
              onClick={handleSearch}
            >
              {t('搜索')}
            </Button>

            <Button
              onClick={handleResetFilters}
            >
              {t('重置')}
            </Button>
          </Space>
        </div>

        {/* Table */}
        {loading && orders.length === 0 ? (
          <div className='flex items-center justify-center py-20'>
            <Spin size='large' />
          </div>
        ) : orders.length > 0 ? (
          <Table
            columns={columns}
            dataSource={orders}
            pagination={{
              currentPage: pagination.currentPage,
              pageSize: pagination.pageSize,
              total: pagination.total,
              onPageChange: handlePageChange,
              showSizeChanger: false,
            }}
            loading={loading}
            rowKey='order_id'
            scroll={{ x: 1800 }}
          />
        ) : (
          <Empty
            image={<IconShoppingBag size='extra-large' style={{ fontSize: 64 }} />}
            title={t('暂无订单')}
            description={t('暂时没有符合条件的订单')}
          />
        )}
      </Card>
    </div>
  );
};

export default AdminOrders;
