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
import { useNavigate } from 'react-router-dom';
import {
  Card,
  Table,
  Tag,
  Button,
  Typography,
  Empty,
  Spin,
  Modal,
  Space,
} from '@douyinfe/semi-ui';
import {
  IconRefresh,
  IconShoppingBag,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';

const { Title } = Typography;

const MyOrders = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [orders, setOrders] = useState([]);
  const [pagination, setPagination] = useState({
    currentPage: 1,
    pageSize: 20,
    total: 0,
  });

  // Load orders
  const loadOrders = async (page = 1) => {
    setLoading(true);
    try {
      const res = await API.get(`/api/user/plan/purchase/my-orders?page=${page}&page_size=${pagination.pageSize}`);
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

  // Cancel order
  const handleCancelOrder = (orderId, orderNo) => {
    Modal.confirm({
      title: t('取消订单'),
      content: t('确定要取消订单 {{orderNo}} 吗？取消后无法恢复。', { orderNo }),
      okText: t('确认取消'),
      cancelText: t('返回'),
      okType: 'danger',
      onOk: async () => {
        try {
          const res = await API.post('/api/user/plan/purchase/cancel', {
            order_id: orderId,
          });
          const { success, message } = res.data;
          if (success) {
            showSuccess(t('订单已取消'));
            loadOrders(pagination.currentPage);
          } else {
            showError(message || t('取消失败'));
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
      width: 200,
      render: (text) => <span style={{ fontFamily: 'monospace' }}>{text}</span>,
    },
    {
      title: t('套餐'),
      dataIndex: 'plan_name',
      key: 'plan_name',
    },
    {
      title: t('金额'),
      dataIndex: 'final_price',
      key: 'final_price',
      width: 120,
      render: (price) => (
        <span style={{ fontWeight: 600, color: 'var(--semi-color-primary)' }}>
          ${price?.toFixed(2) || '0.00'}
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
      title: t('创建时间'),
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (time) => (time ? timestamp2string(time / 1000) : '-'),
    },
    {
      title: t('支付时间'),
      dataIndex: 'paid_at',
      key: 'paid_at',
      width: 180,
      render: (time) => (time ? timestamp2string(time / 1000) : '-'),
    },
    {
      title: t('操作'),
      key: 'action',
      width: 180,
      render: (_, record) => {
        if (record.status === 'pending') {
          const now = Date.now();
          const isExpired = record.expired_at && now > record.expired_at;
          if (!isExpired) {
            return (
              <Space>
                <Button
                  size='small'
                  theme='solid'
                  type='primary'
                  onClick={() => navigate(`/console/order-confirm/${record.order_id}`)}
                >
                  {t('继续支付')}
                </Button>
                <Button
                  size='small'
                  type='danger'
                  onClick={() => handleCancelOrder(record.order_id, record.order_no)}
                >
                  {t('取消')}
                </Button>
              </Space>
            );
          }
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
            {t('我的订单')}
          </Title>
          <Button
            icon={<IconRefresh />}
            onClick={() => loadOrders(pagination.currentPage)}
            loading={loading}
          >
            {t('刷新')}
          </Button>
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
          />
        ) : (
          <Empty
            image={<IconShoppingBag size='extra-large' style={{ fontSize: 64 }} />}
            title={t('暂无订单')}
            description={t('您还没有购买过任何套餐')}
          >
            <Button
              theme='solid'
              type='primary'
              onClick={() => navigate('/plans')}
            >
              {t('去购买')}
            </Button>
          </Empty>
        )}
      </Card>
    </div>
  );
};

export default MyOrders;
