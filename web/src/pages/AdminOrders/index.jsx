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
  Descriptions,
  Popconfirm,
} from '@douyinfe/semi-ui';
import {
  IconRefresh,
  IconSearch,
  IconShoppingBag,
  IconEyeOpened,
  IconClose,
  IconDelete,
  IconTickCircle,
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

  // Order type filter
  const [orderType, setOrderType] = useState('all');
  const [planTotal, setPlanTotal] = useState(0);
  const [topupTotal, setTopupTotal] = useState(0);

  // Filters
  const [filters, setFilters] = useState({
    status: '',
    userId: '',
    orderNo: '',
  });

  // Detail modal
  const [detailVisible, setDetailVisible] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [orderDetail, setOrderDetail] = useState(null);

  // Helper function to get API base path based on order type
  const getOrderApiBase = (record) => {
    return record.order_type === 'topup' ? '/api/user/topup-orders' : '/api/user/plan-orders';
  };

  // Load orders
  const loadOrders = async (page = 1) => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        page: page.toString(),
        page_size: pagination.pageSize.toString(),
      });

      if (orderType !== 'all') params.append('order_type', orderType);
      if (filters.status) params.append('status', filters.status);
      if (filters.userId) params.append('user_id', filters.userId);
      if (filters.orderNo) params.append('order_no', filters.orderNo);

      const res = await API.get(`/api/user/plan-orders?${params.toString()}`);
      const { success, message, data } = res.data;
      if (success && data) {
        setOrders(data.orders || []);
        setPagination({
          ...pagination,
          currentPage: data.page || page,
          total: data.total || 0,
        });
        setPlanTotal(data.plan_total || 0);
        setTopupTotal(data.topup_total || 0);
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
  }, [orderType]);

  // Handle search
  const handleSearch = () => {
    loadOrders(1);
  };

  // Handle reset filters
  const handleResetFilters = () => {
    const shouldTriggerByOrderTypeChange = orderType !== 'all';
    if (shouldTriggerByOrderTypeChange) {
      setOrderType('all');
    }
    setFilters({
      status: '',
      userId: '',
      orderNo: '',
    });

    if (!shouldTriggerByOrderTypeChange) {
      setTimeout(() => loadOrders(1), 100);
    }
  };

  // Handle manual completion
  const handleManualComplete = async (record) => {
    const apiBase = getOrderApiBase(record);
    Modal.confirm({
      title: t('确认手动完成订单'),
      content: record.order_type === 'topup' 
        ? t('确认要手动完成该充值订单吗？此操作不可撤销。')
        : t('确认要手动完成该订单并发放套餐吗？此操作不可撤销。'),
      onOk: async () => {
        try {
          const res = await API.post(`${apiBase}/${record.order_id}/complete`);
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

  // Handle view detail
  const handleViewDetail = async (record) => {
    const apiBase = getOrderApiBase(record);
    setDetailLoading(true);
    setDetailVisible(true);
    try {
      const res = await API.get(`${apiBase}/${record.order_id}`);
      const { success, message, data } = res.data;
      if (success && data) {
        setOrderDetail(data);
      } else {
        showError(message || t('获取详情失败'));
        setDetailVisible(false);
      }
    } catch (e) {
      showError(e.message || t('网络错误'));
      setDetailVisible(false);
    }
    setDetailLoading(false);
  };

  // Handle cancel order
  const handleCancelOrder = async (record) => {
    const apiBase = getOrderApiBase(record);
    try {
      const res = await API.post(`${apiBase}/${record.order_id}/cancel`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('订单已取消'));
        loadOrders(pagination.currentPage);
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(e.message || t('网络错误'));
    }
  };

  // Handle delete order
  const handleDeleteOrder = async (record) => {
    const apiBase = getOrderApiBase(record);
    try {
      const res = await API.delete(`${apiBase}/${record.order_id}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('订单已删除'));
        loadOrders(pagination.currentPage);
      } else {
        showError(message || t('操作失败'));
      }
    } catch (e) {
      showError(e.message || t('网络错误'));
    }
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
      title: t('类型'),
      dataIndex: 'order_type',
      key: 'order_type',
      width: 80,
      render: (type) => type === 'topup' 
        ? <Tag color='cyan'>{t('充值')}</Tag> 
        : <Tag color='violet'>{t('套餐')}</Tag>,
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
      title: t('名称'),
      dataIndex: 'plan_name',
      key: 'plan_name',
      width: 120,
      render: (text) => text || '-',
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
      width: 200,
      fixed: 'right',
      render: (_, record) => {
        const actions = [];

        // View detail button - always show
        actions.push(
          <Button
            key='view'
            size='small'
            icon={<IconEyeOpened />}
            onClick={() => handleViewDetail(record)}
          />
        );

        // Manual complete button
        // For plan orders: show when status === 'paid' && !record.user_plan_id
        // For topup orders: show when status === 'pending'
        const showManualComplete = record.order_type === 'topup'
          ? record.status === 'pending'
          : (record.status === 'paid' && !record.user_plan_id);

        if (showManualComplete) {
          actions.push(
            <Button
              key='complete'
              size='small'
              type='warning'
              icon={<IconTickCircle />}
              onClick={() => handleManualComplete(record)}
            />
          );
        }

        // Cancel button - for pending orders
        if (record.status === 'pending') {
          actions.push(
            <Popconfirm
              key='cancel'
              title={t('确认取消订单')}
              content={t('确认要取消该订单吗？')}
              onConfirm={() => handleCancelOrder(record)}
            >
              <Button
                size='small'
                type='tertiary'
                icon={<IconClose />}
              />
            </Popconfirm>
          );
        }

        // Delete button - for expired or cancelled orders
        if (record.status === 'expired' || record.status === 'cancelled') {
          actions.push(
            <Popconfirm
              key='delete'
              title={t('确认删除订单')}
              content={t('确认要删除该订单吗？此操作不可撤销。')}
              onConfirm={() => handleDeleteOrder(record)}
            >
              <Button
                size='small'
                type='danger'
                icon={<IconDelete />}
              />
            </Popconfirm>
          );
        }

        return <Space>{actions}</Space>;
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
              value={orderType}
              onChange={(value) => setOrderType(value)}
              style={{ width: 160 }}
            >
              <Select.Option value='all'>
                {t('全部类型')} ({planTotal + topupTotal})
              </Select.Option>
              <Select.Option value='plan'>
                {t('套餐')} ({planTotal})
              </Select.Option>
              <Select.Option value='topup'>
                {t('充值')} ({topupTotal})
              </Select.Option>
            </Select>

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

      {/* Order Detail Modal */}
      <Modal
        title={t('订单详情')}
        visible={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={700}
      >
        {detailLoading ? (
          <div className='flex items-center justify-center py-10'>
            <Spin size='large' />
          </div>
        ) : orderDetail ? (
          <Descriptions>
            <Descriptions.Item itemKey={t('订单号')}>
              <span style={{ fontFamily: 'monospace' }}>{orderDetail.order_no}</span>
            </Descriptions.Item>
            <Descriptions.Item itemKey={t('类型')}>
              {orderDetail.order_type === 'topup' 
                ? <Tag color='cyan'>{t('充值')}</Tag> 
                : <Tag color='violet'>{t('套餐')}</Tag>}
            </Descriptions.Item>
            <Descriptions.Item itemKey={t('状态')}>
              {getStatusTag(orderDetail.status)}
            </Descriptions.Item>
            <Descriptions.Item itemKey={t('用户')}>
              {orderDetail.username || `ID: ${orderDetail.user_id}`}
              {orderDetail.user_email && ` (${orderDetail.user_email})`}
            </Descriptions.Item>

            {/* Plan order specific fields */}
            {orderDetail.order_type === 'plan' && (
              <>
                <Descriptions.Item itemKey={t('套餐名称')}>
                  {orderDetail.plan_display_name || orderDetail.plan_name || '-'}
                </Descriptions.Item>
                <Descriptions.Item itemKey={t('套餐类型')}>
                  {orderDetail.plan_type || '-'}
                </Descriptions.Item>
                <Descriptions.Item itemKey={t('套餐分类')}>
                  {orderDetail.plan_category || '-'}
                </Descriptions.Item>
                <Descriptions.Item itemKey={t('套餐额度')}>
                  {orderDetail.plan_quota ? `$${(orderDetail.plan_quota / 500000).toFixed(2)}` : '-'}
                </Descriptions.Item>
                <Descriptions.Item itemKey={t('有效期')}>
                  {orderDetail.plan_validity_days ? `${orderDetail.plan_validity_days} 天` : '-'}
                </Descriptions.Item>
                <Descriptions.Item itemKey={t('用户套餐ID')}>
                  {orderDetail.user_plan_id || '-'}
                </Descriptions.Item>
                <Descriptions.Item itemKey={t('发放重试次数')}>
                  {orderDetail.delivery_retry_count || 0}
                </Descriptions.Item>
                <Descriptions.Item itemKey={t('发放时间')}>
                  {orderDetail.delivered_at ? timestamp2string(orderDetail.delivered_at / 1000) : '-'}
                </Descriptions.Item>
              </>
            )}

            {/* Topup order specific fields */}
            {orderDetail.order_type === 'topup' && (
              <>
                <Descriptions.Item itemKey={t('充值金额')}>
                  ¥{orderDetail.amount?.toFixed(2) || '0.00'}
                </Descriptions.Item>
                <Descriptions.Item itemKey={t('获得额度')}>
                  {orderDetail.quota ? `$${(orderDetail.quota / 500000).toFixed(2)}` : '-'}
                </Descriptions.Item>
                <Descriptions.Item itemKey={t('折扣率')}>
                  {orderDetail.discount_rate ? `${(orderDetail.discount_rate * 100).toFixed(0)}%` : '-'}
                </Descriptions.Item>
              </>
            )}

            {/* Common fields */}
            <Descriptions.Item itemKey={t('原价')}>
              ¥{orderDetail.plan_original_price?.toFixed(2) || orderDetail.original_price?.toFixed(2) || '0.00'}
            </Descriptions.Item>
            <Descriptions.Item itemKey={t('实付金额')}>
              <span style={{ fontWeight: 600, color: '#f5222d' }}>
                ¥{orderDetail.final_price?.toFixed(2) || '0.00'}
              </span>
            </Descriptions.Item>
            <Descriptions.Item itemKey={t('支付方式')}>
              {orderDetail.payment_method || '-'}
            </Descriptions.Item>
            <Descriptions.Item itemKey={t('支付流水号')}>
              {orderDetail.payment_trade_no || '-'}
            </Descriptions.Item>
            <Descriptions.Item itemKey={t('创建时间')}>
              {orderDetail.created_at ? timestamp2string(orderDetail.created_at / 1000) : '-'}
            </Descriptions.Item>
            <Descriptions.Item itemKey={t('过期时间')}>
              {orderDetail.expired_at ? timestamp2string(orderDetail.expired_at / 1000) : '-'}
            </Descriptions.Item>
            <Descriptions.Item itemKey={t('支付时间')}>
              {orderDetail.paid_at ? timestamp2string(orderDetail.paid_at / 1000) : '-'}
            </Descriptions.Item>
            <Descriptions.Item itemKey={t('取消时间')}>
              {orderDetail.cancelled_at ? timestamp2string(orderDetail.cancelled_at / 1000) : '-'}
            </Descriptions.Item>
          </Descriptions>
        ) : null}
      </Modal>
    </div>
  );
};

export default AdminOrders;
