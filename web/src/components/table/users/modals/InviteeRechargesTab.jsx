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
  Card,
  Row,
  Col,
  Table,
  Button,
  Empty,
  Space,
  Typography,
  Tag,
  Popconfirm,
  Modal,
  InputNumber,
  Toast,
} from '@douyinfe/semi-ui';
import { InviterRewardAPI } from '../../../../services/inviterRewardApi';
import { formatUSDAmount } from '../../../../utils/currency';
import { timestamp2string } from '../../../../helpers';

const { Text, Title } = Typography;

// 返现记录状态 → 明细行「激励状态」标签
const AFF_STATUS_TAG = {
  pending: { color: 'orange', label: '待发放' },
  settled: { color: 'green', label: '已发放' },
  rejected: { color: 'red', label: '已拒绝' },
  refunded: { color: 'grey', label: '已退款' },
  offline_paid: { color: 'purple', label: '线下已付' },
  legacy: { color: 'grey', label: '历史归档' },
};

const RISK_LABEL = {
  same_ip: '同 IP',
  same_payment_account: '同支付账号',
};

const KpiCard = ({ title, value, color }) => (
  <Card bodyStyle={{ padding: 16, textAlign: 'center' }}>
    <Text type='tertiary' style={{ fontSize: 12 }}>
      {title}
    </Text>
    <div style={{ marginTop: 8 }}>
      <Text strong style={{ fontSize: 20, color }}>
        {value}
      </Text>
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
    pending_reward_usd: 0,
  });
  const [defaultPercent, setDefaultPercent] = useState(10);
  const [items, setItems] = useState([]);
  const [pagination, setPagination] = useState({
    currentPage: 1,
    pageSize: 10,
    total: 0,
  });
  const [history, setHistory] = useState([]);
  const [historyPagination, setHistoryPagination] = useState({
    currentPage: 1,
    pageSize: 10,
    total: 0,
  });
  // 无返现记录的历史充值:手动补录金额
  const [issueTarget, setIssueTarget] = useState(null);
  const [issueAmount, setIssueAmount] = useState(0);
  const [issuing, setIssuing] = useState(false);

  const fetchDetail = useCallback(
    async (page = 1, pageSize = 10) => {
      if (!inviterId) return;
      setLoading(true);
      try {
        const res = await InviterRewardAPI.fetchInviteeRecharges(
          inviterId,
          page,
          pageSize,
        );
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
    },
    [inviterId],
  );

  const fetchHistory = useCallback(
    async (page = 1, pageSize = 10) => {
      if (!inviterId) return;
      setHistoryLoading(true);
      try {
        const res = await InviterRewardAPI.fetchPayoutHistory(
          inviterId,
          page,
          pageSize,
        );
        setHistory(res.items || []);
        setHistoryPagination({
          currentPage: res.pagination?.page || page,
          pageSize: res.pagination?.page_size || pageSize,
          total: res.pagination?.total || 0,
        });
      } finally {
        setHistoryLoading(false);
      }
    },
    [inviterId],
  );

  useEffect(() => {
    if (visible && inviterId) {
      fetchDetail(1, pagination.pageSize);
      fetchHistory(1, historyPagination.pageSize);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, inviterId]);

  const refreshAll = () => {
    fetchDetail(pagination.currentPage, pagination.pageSize);
    fetchHistory(1, historyPagination.pageSize);
  };

  const issue = async (row, rewardUsd) => {
    setIssuing(true);
    try {
      const res = await InviterRewardAPI.issueReward(inviterId, {
        source_type: row.source_type,
        record_id: row.record_id,
        reward_usd: rewardUsd,
      });
      Toast.success(
        t('已发放 {{amount}} 到邀请人账户', {
          amount: formatUSDAmount(res?.reward_usd ?? rewardUsd),
        }),
      );
      setIssueTarget(null);
      refreshAll();
    } catch (_) {
      // 错误已由 API 层 toast
    } finally {
      setIssuing(false);
    }
  };

  const openIssueModal = (row) => {
    const suggested =
      Math.round((Number(row.money_usd) || 0) * (Number(defaultPercent) || 0)) /
      100;
    setIssueAmount(suggested);
    setIssueTarget(row);
  };

  const SOURCE_TYPE_LABELS = {
    topup: t('钱包充值'),
    plan_order: t('月卡订单'),
    topup_order: t('按量订单'),
  };

  const detailColumns = [
    { title: t('被邀请人'), dataIndex: 'invitee_username' },
    {
      title: t('类型'),
      dataIndex: 'source_type',
      render: (v) => SOURCE_TYPE_LABELS[v] || v,
    },
    {
      title: t('完成时间'),
      dataIndex: 'paid_at_ms',
      render: (v) => {
        const n = Number(v);
        if (!n) return '-';
        const sec = n > 1e12 ? Math.floor(n / 1000) : Math.floor(n);
        return timestamp2string(sec);
      },
    },
    {
      title: t('金额'),
      dataIndex: 'money_usd',
      render: (v) => formatUSDAmount(v),
    },
    {
      title: t('支付方式'),
      dataIndex: 'payment_method',
      render: (v) => v || '-',
    },
    { title: t('订单号'), dataIndex: 'order_no', render: (v) => v || '-' },
    {
      title: t('激励额度'),
      dataIndex: 'reward_usd',
      render: (v, row) =>
        row.aff_log_id > 0 ? (
          <Text strong>{formatUSDAmount(v)}</Text>
        ) : (
          <Text type='tertiary'>
            {t('约')}{' '}
            {formatUSDAmount(
              ((Number(row.money_usd) || 0) * (Number(defaultPercent) || 0)) /
                100,
            )}
          </Text>
        ),
    },
    {
      title: t('激励状态'),
      dataIndex: 'aff_status',
      render: (v, row) => {
        const tag = AFF_STATUS_TAG[v];
        return (
          <Space spacing={4}>
            {tag ? (
              <Tag color={tag.color} size='small'>
                {t(tag.label)}
              </Tag>
            ) : (
              <Tag color='grey' size='small'>
                {t('未发放')}
              </Tag>
            )}
            {row.risk_flag && (
              <Tag color='orange' size='small'>
                {t(RISK_LABEL[row.risk_flag] || row.risk_flag)}
              </Tag>
            )}
          </Space>
        );
      },
    },
    {
      title: t('操作'),
      fixed: 'right',
      width: 90,
      render: (_, row) => {
        if (row.aff_status === 'pending' || row.aff_status === 'rejected') {
          return (
            <Popconfirm
              title={t('确认发放?')}
              content={t('{{amount}} 会立即到邀请人账户', {
                amount: formatUSDAmount(row.reward_usd),
              })}
              okText={t('发放')}
              cancelText={t('取消')}
              onConfirm={() => issue(row, 0)}
            >
              <Button
                size='small'
                theme='solid'
                type='primary'
                loading={issuing}
              >
                {t('发放')}
              </Button>
            </Popconfirm>
          );
        }
        if (!row.aff_status) {
          return (
            <Button
              size='small'
              theme='light'
              type='primary'
              onClick={() => openIssueModal(row)}
            >
              {t('发放')}
            </Button>
          );
        }
        return null;
      },
    },
  ];

  const historyColumns = [
    { title: t('批次'), dataIndex: 'id', render: (v) => `#${v}` },
    {
      title: t('发放金额'),
      dataIndex: 'payout_amount_usd',
      render: (v) => formatUSDAmount(v),
    },
    {
      title: t('涉及充值'),
      dataIndex: 'recharge_total_usd',
      render: (v) => formatUSDAmount(v),
    },
    {
      title: t('涉及笔数'),
      dataIndex: 'topup_count',
      render: (v) => `${v || 0}`,
    },
    { title: t('备注'), dataIndex: 'note', render: (v) => v || '-' },
    {
      title: t('操作管理员'),
      dataIndex: 'operator_admin_username',
      render: (v, row) => v || `#${row.operator_admin_id}`,
    },
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

  return (
    <Space vertical style={{ width: '100%' }} size='large'>
      <Row gutter={16}>
        <Col span={6}>
          <KpiCard
            title={t('累计邀请人数')}
            value={summary.invitee_count || 0}
            color='#1890ff'
          />
        </Col>
        <Col span={6}>
          <KpiCard
            title={t('下级累计充值')}
            value={formatUSDAmount(summary.recharge_total_usd || 0)}
            color='#52c41a'
          />
        </Col>
        <Col span={6}>
          <KpiCard
            title={t('已发放奖励')}
            value={formatUSDAmount(summary.payout_total_usd || 0)}
            color='#722ed1'
          />
        </Col>
        <Col span={6}>
          <KpiCard
            title={t('待发放激励')}
            value={formatUSDAmount(summary.pending_reward_usd || 0)}
            color='#faad14'
          />
        </Col>
      </Row>

      <Card>
        <Title heading={5} style={{ marginBottom: 12 }}>
          {t('邀请下级充值明细')}
        </Title>
        <Table
          columns={detailColumns}
          dataSource={items}
          loading={loading}
          rowKey={(row) => `${row.source_type}-${row.record_id}`}
          size='small'
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
        <Title heading={5} style={{ marginBottom: 12 }}>
          {t('激励发放历史')}
        </Title>
        <Table
          columns={historyColumns}
          dataSource={history}
          loading={historyLoading}
          rowKey='id'
          size='small'
          pagination={{
            currentPage: historyPagination.currentPage,
            pageSize: historyPagination.pageSize,
            total: historyPagination.total,
            showSizeChanger: true,
            pageSizeOpts: [10, 20, 50, 100],
            onPageChange: (p, ps) =>
              fetchHistory(p, ps || historyPagination.pageSize),
            onPageSizeChange: (ps) => fetchHistory(1, ps),
          }}
          empty={<Empty description={t('暂无激励发放记录')} />}
          scroll={{ x: 800 }}
        />
      </Card>

      <Modal
        title={t('发放激励')}
        visible={!!issueTarget}
        onCancel={() => setIssueTarget(null)}
        onOk={() => issue(issueTarget, Number(issueAmount))}
        okText={t('确认发放')}
        okButtonProps={{
          loading: issuing,
          disabled: !(Number(issueAmount) > 0),
        }}
        cancelText={t('取消')}
      >
        {issueTarget && (
          <Space vertical align='start' style={{ width: '100%' }}>
            <Text>
              {issueTarget.invitee_username} ·{' '}
              {SOURCE_TYPE_LABELS[issueTarget.source_type] ||
                issueTarget.source_type}{' '}
              · {formatUSDAmount(issueTarget.money_usd)}
            </Text>
            <Text type='tertiary' size='small'>
              {t(
                '这笔充值没有返现记录,请填写要发放的金额(默认按 {{p}}% 计算)',
                { p: defaultPercent },
              )}
            </Text>
            <InputNumber
              value={issueAmount}
              onChange={setIssueAmount}
              min={0}
              step={1}
              precision={2}
              prefix='$'
              style={{ width: '100%' }}
            />
            <Text type='tertiary' size='small'>
              {t('确认后立即到邀请人账户,可在「激励发放历史」查看')}
            </Text>
          </Space>
        )}
      </Modal>
    </Space>
  );
};

export default InviteeRechargesTab;
