/*
Copyright (C) 2025 QuantumNous

Tab inside UserDetailModal showing the user's invitee recharge stats and payout history.
*/

import React, { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Row,
  Col,
  Table,
  Button,
  Empty,
  Space,
  Typography,
  Tag,
  Tooltip,
} from '@douyinfe/semi-ui';
import { InviterRewardAPI } from '../../../../services/inviterRewardApi';
import { formatUSDAmount } from '../../../../utils/currency';
import { timestamp2string } from '../../../../helpers';
import PayoutInviterRewardModal from './PayoutInviterRewardModal';

const { Text, Title } = Typography;

const KpiCard = ({ title, value, color }) => (
  <Card bodyStyle={{ padding: 16, textAlign: 'center' }}>
    <Text type="tertiary" style={{ fontSize: 12 }}>{title}</Text>
    <div style={{ marginTop: 8 }}>
      <Text strong style={{ fontSize: 20, color }}>{value}</Text>
    </div>
  </Card>
);

const InviteeRechargesTab = ({ visible, inviterId }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [summary, setSummary] = useState({
    invitee_count: 0,
    recharge_total_usd: 0,
    payout_total_usd: 0,
    pending_total_usd: 0,
  });
  const [defaultPercent, setDefaultPercent] = useState(10);
  const [items, setItems] = useState([]);
  const [pagination, setPagination] = useState({ currentPage: 1, pageSize: 10, total: 0 });
  const [history, setHistory] = useState([]);
  const [historyPagination, setHistoryPagination] = useState({ currentPage: 1, pageSize: 10, total: 0 });
  const [payoutModalVisible, setPayoutModalVisible] = useState(false);

  const fetchDetail = useCallback(async (page = 1, pageSize = 10) => {
    if (!inviterId) return;
    setLoading(true);
    try {
      const res = await InviterRewardAPI.fetchInviteeRecharges(inviterId, page, pageSize);
      setSummary(res.summary || {});
      setItems(res.items || []);
      setDefaultPercent(res.default_percent ?? 10);
      setPagination({
        currentPage: res.pagination?.page || page,
        pageSize: res.pagination?.page_size || pageSize,
        total: res.pagination?.total || 0,
      });
    } finally {
      setLoading(false);
    }
  }, [inviterId]);

  const fetchHistory = useCallback(async (page = 1, pageSize = 10) => {
    if (!inviterId) return;
    setHistoryLoading(true);
    try {
      const res = await InviterRewardAPI.fetchPayoutHistory(inviterId, page, pageSize);
      setHistory(res.items || []);
      setHistoryPagination({
        currentPage: res.pagination?.page || page,
        pageSize: res.pagination?.page_size || pageSize,
        total: res.pagination?.total || 0,
      });
    } finally {
      setHistoryLoading(false);
    }
  }, [inviterId]);

  useEffect(() => {
    if (visible && inviterId) {
      fetchDetail(1, pagination.pageSize);
      fetchHistory(1, historyPagination.pageSize);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, inviterId]);

  const detailColumns = [
    { title: t('被邀请人'), dataIndex: 'invitee_username' },
    {
      title: t('完成时间'),
      dataIndex: 'complete_time',
      render: (v) => {
        const n = Number(v);
        if (!n) return '-';
        const sec = n > 1e12 ? Math.floor(n / 1000) : Math.floor(n);
        return timestamp2string(sec);
      },
    },
    { title: t('金额'), dataIndex: 'money_usd', render: (v) => formatUSDAmount(v) },
    { title: t('支付方式'), dataIndex: 'payment_method' },
    { title: t('订单号'), dataIndex: 'trade_no' },
    {
      title: t('激励状态'),
      dataIndex: 'payout_id',
      render: (v) =>
        v && v > 0
          ? <Tag color="green" size="small">{t('批次')} #{v}</Tag>
          : <Tag color="orange" size="small">{t('待激励')}</Tag>,
    },
  ];

  const historyColumns = [
    { title: t('批次'), dataIndex: 'id', render: (v) => `#${v}` },
    { title: t('发放金额'), dataIndex: 'payout_amount_usd', render: (v) => formatUSDAmount(v) },
    { title: t('涉及充值'), dataIndex: 'recharge_total_usd', render: (v) => formatUSDAmount(v) },
    { title: t('备注'), dataIndex: 'note', render: (v) => v || '-' },
    { title: t('操作管理员'), dataIndex: 'operator_admin_id', render: (v) => `#${v}` },
    {
      title: t('时间'),
      dataIndex: 'created_at',
      render: (v) => {
        const n = Number(v);
        if (!n) return '-';
        const sec = n > 1e12 ? Math.floor(n / 1000) : Math.floor(n);
        return timestamp2string(sec);
      },
    },
  ];

  const noPending = !(Number(summary.pending_total_usd) > 0);

  return (
    <Space vertical style={{ width: '100%' }} size="large">
      <Row gutter={16}>
        <Col span={6}>
          <KpiCard title={t('累计邀请人数')} value={summary.invitee_count || 0} color="#1890ff" />
        </Col>
        <Col span={6}>
          <KpiCard title={t('下级累计充值')} value={formatUSDAmount(summary.recharge_total_usd || 0)} color="#52c41a" />
        </Col>
        <Col span={6}>
          <KpiCard title={t('已发放奖励')} value={formatUSDAmount(summary.payout_total_usd || 0)} color="#722ed1" />
        </Col>
        <Col span={6}>
          <KpiCard title={t('待激励充值')} value={formatUSDAmount(summary.pending_total_usd || 0)} color="#faad14" />
        </Col>
      </Row>

      <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <Tooltip content={noPending ? t('暂无待激励充值') : ''}>
          <Button
            type="primary"
            disabled={noPending}
            onClick={() => setPayoutModalVisible(true)}
          >
            {t('发放激励')}
          </Button>
        </Tooltip>
      </div>

      <Card>
        <Title heading={5} style={{ marginBottom: 12 }}>{t('邀请下级充值明细')}</Title>
        <Table
          columns={detailColumns}
          dataSource={items}
          loading={loading}
          rowKey="topup_id"
          size="small"
          pagination={{
            currentPage: pagination.currentPage,
            pageSize: pagination.pageSize,
            total: pagination.total,
            showSizeChanger: true,
            pageSizeOpts: [10, 20, 50, 100],
            onPageChange: (p, ps) => fetchDetail(p, ps || pagination.pageSize),
            onPageSizeChange: (ps) => fetchDetail(1, ps),
          }}
          empty={<Empty description={t('暂无下级充值记录')} />}
          scroll={{ x: 900 }}
        />
      </Card>

      <Card>
        <Title heading={5} style={{ marginBottom: 12 }}>{t('激励发放历史')}</Title>
        <Table
          columns={historyColumns}
          dataSource={history}
          loading={historyLoading}
          rowKey="id"
          size="small"
          pagination={{
            currentPage: historyPagination.currentPage,
            pageSize: historyPagination.pageSize,
            total: historyPagination.total,
            showSizeChanger: true,
            pageSizeOpts: [10, 20, 50, 100],
            onPageChange: (p, ps) => fetchHistory(p, ps || historyPagination.pageSize),
            onPageSizeChange: (ps) => fetchHistory(1, ps),
          }}
          empty={<Empty description={t('暂无激励发放记录')} />}
          scroll={{ x: 800 }}
        />
      </Card>

      <PayoutInviterRewardModal
        visible={payoutModalVisible}
        inviterId={inviterId}
        pendingTotalUsd={summary.pending_total_usd || 0}
        defaultPercent={defaultPercent}
        onClose={() => setPayoutModalVisible(false)}
        onSuccess={() => {
          fetchDetail(1, pagination.pageSize);
          fetchHistory(1, historyPagination.pageSize);
        }}
      />
    </Space>
  );
};

export default InviteeRechargesTab;
