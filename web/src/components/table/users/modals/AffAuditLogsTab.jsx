import React, { useEffect, useState } from 'react';
import {
  Card,
  Table,
  Tag,
  Button,
  Select,
  Space,
  Modal,
  Input,
  InputNumber,
  Form,
  Toast,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../../../helpers/api';

const STATUS_TAGS = {
  pending: { color: 'blue', label: 'pending' },
  settled: { color: 'green', label: 'settled' },
  rejected: { color: 'red', label: 'rejected' },
  refunded: { color: 'orange', label: 'refunded' },
  offline_paid: { color: 'purple', label: 'offline_paid' },
};

const REJECT_REASON_LABEL = {
  same_ip: '同 IP',
  same_payment_account: '同支付账号',
  inviter_frozen: '邀请人已冻结',
};

const AffAuditLogsTab = ({ visible, inviterId }) => {
  const { t } = useTranslation();
  const [items, setItems] = useState([]);
  const [summary, setSummary] = useState(null);
  const [loading, setLoading] = useState(false);
  const [statusFilter, setStatusFilter] = useState('pending');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [selected, setSelected] = useState([]);
  const [markModalVisible, setMarkModalVisible] = useState(false);
  const [markFormApi, setMarkFormApi] = useState(null);

  const reload = async () => {
    if (!inviterId) return;
    setLoading(true);
    try {
      const res = await API.get(
        `/api/user/manage/${inviterId}/aff-audit-logs?status=${statusFilter}&page=${page}&page_size=${pageSize}`
      );
      if (res?.data?.success) {
        setItems(res.data.data.items || []);
        setTotal(res.data.data.pagination?.total || 0);
      }
      const r2 = await API.get(`/api/user/manage/${inviterId}/aff-summary`);
      if (r2?.data?.success) setSummary(r2.data.data);
    } catch (e) {
      Toast.error(e.message || '加载失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (visible) reload();
    // eslint-disable-next-line
  }, [visible, inviterId, statusFilter, page, pageSize]);

  const handleSettleSingle = async (logId) => {
    try {
      const res = await API.post(`/api/user/manage/aff-audit-logs/${logId}/settle`);
      if (res?.data?.success) {
        Toast.success(t('已结算'));
        reload();
      } else {
        Toast.error(res?.data?.message || t('结算失败'));
      }
    } catch (e) {
      Toast.error(e.response?.data?.message || e.message);
    }
  };

  const handleMarkSubmit = async (values) => {
    try {
      const res = await API.post(
        `/api/user/manage/${inviterId}/aff-audit-logs/mark-offline-paid`,
        {
          log_ids: selected.map((id) => parseInt(id)),
          offline_amount_cny: parseFloat(values.amount_cny),
          note: values.note || '',
        }
      );
      if (res?.data?.success) {
        Toast.success(t('已标记 {{n}} 条为线下已返现', { n: selected.length }));
        setMarkModalVisible(false);
        setSelected([]);
        reload();
      }
    } catch (e) {
      Toast.error(e.response?.data?.message || e.message);
    }
  };

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    {
      title: t('下级'),
      dataIndex: 'invitee_username',
      width: 150,
    },
    { title: t('来源'), dataIndex: 'source_type', width: 100 },
    {
      title: t('原币'),
      width: 110,
      render: (_, r) => `${r.currency} ${r.amount_native?.toFixed(2)}`,
    },
    {
      title: 'USD',
      dataIndex: 'amount_usd',
      width: 90,
      render: (v) => `$${v?.toFixed(4)}`,
    },
    {
      title: t('返佣 USD'),
      dataIndex: 'reward_usd',
      width: 100,
      render: (v) => `$${v?.toFixed(4)}`,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 110,
      render: (s, r) => {
        const tag = STATUS_TAGS[s] || { color: 'grey', label: s };
        return (
          <Space spacing={4}>
            <Tag color={tag.color}>{tag.label}</Tag>
            {s === 'rejected' && r.reject_reason && (
              <span style={{ fontSize: 11, color: '#888' }}>
                {REJECT_REASON_LABEL[r.reject_reason] || r.reject_reason}
              </span>
            )}
            {s === 'offline_paid' && (
              <span style={{ fontSize: 11, color: '#888' }}>
                ¥{r.offline_paid_amount_cny?.toFixed(2)}
              </span>
            )}
          </Space>
        );
      },
    },
    {
      title: t('解锁时间'),
      dataIndex: 'eligible_at',
      width: 150,
      render: (v) => (v ? new Date(v).toLocaleString() : '-'),
    },
    {
      title: t('操作'),
      width: 100,
      fixed: 'right',
      render: (_, r) =>
        r.status === 'pending' ? (
          <Button size='small' onClick={() => handleSettleSingle(r.id)}>
            {t('立即结算')}
          </Button>
        ) : null,
    },
  ];

  return (
    <Space vertical style={{ width: '100%' }} size='large'>
      {summary && (
        <Card>
          <Space spacing='loose' wrap>
            <span>{t('邀请人数')}: <strong>{summary.invitee_count}</strong></span>
            <span>{t('待结算 USD')}: <strong>${summary.pending_total_usd?.toFixed(4)}</strong></span>
            <span>{t('已结算 USD')}: <strong>${summary.settled_total_usd?.toFixed(4)}</strong></span>
            <span>{t('线下已返现 CNY')}: <strong>¥{summary.offline_paid_total_cny?.toFixed(2)}</strong></span>
            <span>{t('反作弊拒绝')}: <strong>{summary.rejected_count}</strong></span>
            <span>{t('退款')}: <strong>{summary.refunded_count}</strong></span>
          </Space>
        </Card>
      )}

      <Card>
        <Space style={{ marginBottom: 12 }}>
          <Select
            value={statusFilter}
            onChange={(v) => {
              setStatusFilter(v);
              setPage(1);
              setSelected([]);
            }}
            style={{ width: 160 }}
            optionList={[
              { label: t('全部'), value: '' },
              { label: 'pending', value: 'pending' },
              { label: 'settled', value: 'settled' },
              { label: 'rejected', value: 'rejected' },
              { label: 'refunded', value: 'refunded' },
              { label: 'offline_paid', value: 'offline_paid' },
            ]}
          />
          <Button
            type='primary'
            disabled={selected.length === 0 || statusFilter !== 'pending'}
            onClick={() => setMarkModalVisible(true)}
          >
            {t('标记线下已返现')} ({selected.length})
          </Button>
        </Space>

        <Table
          columns={columns}
          dataSource={items}
          loading={loading}
          rowKey='id'
          size='small'
          rowSelection={
            statusFilter === 'pending'
              ? {
                  selectedRowKeys: selected,
                  onChange: (keys) => setSelected(keys),
                }
              : null
          }
          pagination={{
            currentPage: page,
            pageSize,
            total,
            showSizeChanger: true,
            pageSizeOpts: [10, 20, 50, 100],
            onPageChange: setPage,
            onPageSizeChange: (s) => {
              setPageSize(s);
              setPage(1);
            },
          }}
          scroll={{ x: 1000 }}
        />
      </Card>

      <Modal
        title={t('标记为线下已返现')}
        visible={markModalVisible}
        onCancel={() => setMarkModalVisible(false)}
        footer={null}
      >
        <Form
          getFormApi={(api) => setMarkFormApi(api)}
          onSubmit={handleMarkSubmit}
        >
          <Form.InputNumber
            field='amount_cny'
            label={t('线下实付 CNY')}
            min={0.01}
            step={0.01}
            placeholder='200.00'
            rules={[{ required: true, message: t('必填') }]}
            style={{ width: '100%' }}
          />
          <Form.Input
            field='note'
            label={t('备注')}
            placeholder={t('如:微信转账 / 银行单号')}
          />
          <Space style={{ marginTop: 12 }}>
            <Button type='primary' htmlType='submit'>
              {t('确认标记 {{n}} 条', { n: selected.length })}
            </Button>
            <Button onClick={() => setMarkModalVisible(false)}>{t('取消')}</Button>
          </Space>
        </Form>
      </Modal>
    </Space>
  );
};

export default AffAuditLogsTab;
