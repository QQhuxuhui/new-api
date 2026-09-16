import React, { useEffect, useState } from 'react';
import {
  Card,
  Table,
  Tag,
  Button,
  Space,
  Typography,
  Toast,
  Modal,
  TextArea,
  Popconfirm,
  RadioGroup,
  Radio,
  Empty,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers/api';

const { Title, Text } = Typography;

// 页面只有三个视图,默认停在"待审核"。
const VIEWS = [
  { value: 'pending', label: '待审核' },
  { value: 'settled', label: '已通过' },
  { value: 'rejected', label: '已拒绝' },
];

const SOURCE_LABEL = {
  topup: '余额充值',
  topup_order: '余额充值',
  plan_order: '购买套餐',
};

const RISK_LABEL = {
  same_ip: '与邀请人用过同一 IP',
  same_payment_account: '与邀请人同一支付账号',
};

const REJECT_LABEL = {
  admin: '管理员拒绝',
  inviter_frozen: '邀请人已冻结',
  same_ip: '系统拦截:同 IP',
  same_payment_account: '系统拦截:同支付账号',
};

const fmtTime = (ms) => (ms ? new Date(ms).toLocaleString() : '-');
const fmtUsd = (v) => `$${Number(v || 0).toFixed(2)}`;

const AffReview = () => {
  const { t } = useTranslation();
  const [view, setView] = useState('pending');
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [pendingCount, setPendingCount] = useState(0);
  const [pendingUsd, setPendingUsd] = useState(0);
  const [rejectTarget, setRejectTarget] = useState(null);
  const [rejectNote, setRejectNote] = useState('');
  const [acting, setActing] = useState(false);

  const reload = async () => {
    setLoading(true);
    try {
      const res = await API.get(
        `/api/user/manage/aff-review?status=${view}&page=${page}&page_size=${pageSize}`,
      );
      if (res?.data?.success) {
        const d = res.data.data;
        setItems(d.items || []);
        setTotal(d.pagination?.total || 0);
        setPendingCount(d.pending_count || 0);
        setPendingUsd(d.pending_reward_usd || 0);
      } else {
        Toast.error(res?.data?.message || t('加载失败'));
      }
    } catch (e) {
      Toast.error(e.response?.data?.message || e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    reload();
    // eslint-disable-next-line
  }, [view, page, pageSize]);

  const approve = async (row) => {
    setActing(true);
    try {
      const res = await API.post(
        `/api/user/manage/aff-audit-logs/${row.id}/approve`,
      );
      if (res?.data?.success) {
        Toast.success(
          t('已通过,{{amount}} 已到 {{name}} 的账户', {
            amount: fmtUsd(row.reward_usd),
            name: row.inviter_username,
          }),
        );
        reload();
      } else {
        Toast.error(res?.data?.message || t('操作失败'));
      }
    } catch (e) {
      Toast.error(e.response?.data?.message || e.message);
    } finally {
      setActing(false);
    }
  };

  const reject = async () => {
    if (!rejectTarget) return;
    setActing(true);
    try {
      const res = await API.post(
        `/api/user/manage/aff-audit-logs/${rejectTarget.id}/reject`,
        { note: rejectNote.trim() },
      );
      if (res?.data?.success) {
        Toast.success(t('已拒绝'));
        setRejectTarget(null);
        setRejectNote('');
        reload();
      } else {
        Toast.error(res?.data?.message || t('操作失败'));
      }
    } catch (e) {
      Toast.error(e.response?.data?.message || e.message);
    } finally {
      setActing(false);
    }
  };

  const userCell = (name, id) => (
    <div>
      <div>{name || '-'}</div>
      <Text type='tertiary' size='small'>
        #{id}
      </Text>
    </div>
  );

  const columns = [
    {
      title: t('充值时间'),
      dataIndex: 'created_at',
      width: 170,
      render: fmtTime,
    },
    {
      title: t('邀请人'),
      width: 160,
      render: (_, r) => userCell(r.inviter_username, r.inviter_user_id),
    },
    {
      title: t('充值用户'),
      width: 160,
      render: (_, r) => userCell(r.invitee_username, r.invitee_user_id),
    },
    {
      title: t('充值金额'),
      width: 150,
      render: (_, r) => (
        <div>
          <div>{fmtUsd(r.amount_usd)}</div>
          <Text type='tertiary' size='small'>
            {t(SOURCE_LABEL[r.source_type] || r.source_type)}
          </Text>
        </div>
      ),
    },
    {
      title: t('返现金额'),
      dataIndex: 'reward_usd',
      width: 120,
      render: (v) => <Text strong>{fmtUsd(v)}</Text>,
    },
    {
      title: t('风险提示'),
      width: 220,
      render: (_, r) =>
        r.risk_flag ? (
          <Tag color='orange'>{t(RISK_LABEL[r.risk_flag] || r.risk_flag)}</Tag>
        ) : (
          <Text type='tertiary'>{t('无')}</Text>
        ),
    },
  ];

  if (view === 'pending') {
    columns.push({
      title: t('操作'),
      width: 170,
      fixed: 'right',
      render: (_, r) => (
        <Space>
          <Popconfirm
            title={t('确认通过?')}
            content={t('{{amount}} 会立即到 {{name}} 的账户', {
              amount: fmtUsd(r.reward_usd),
              name: r.inviter_username,
            })}
            okText={t('通过')}
            cancelText={t('取消')}
            onConfirm={() => approve(r)}
          >
            <Button theme='solid' type='primary' size='small' loading={acting}>
              {t('通过')}
            </Button>
          </Popconfirm>
          <Button
            theme='light'
            type='danger'
            size='small'
            onClick={() => {
              setRejectNote('');
              setRejectTarget(r);
            }}
          >
            {t('拒绝')}
          </Button>
        </Space>
      ),
    });
  } else if (view === 'settled') {
    columns.push({
      title: t('审核'),
      width: 200,
      render: (_, r) => (
        <div>
          <div>
            {r.reviewer_username
              ? r.reviewer_username
              : r.reviewed_admin_id
                ? `#${r.reviewed_admin_id}`
                : t('系统自动')}
          </div>
          <Text type='tertiary' size='small'>
            {fmtTime(r.settled_at)}
          </Text>
        </div>
      ),
    });
  } else {
    columns.push(
      {
        title: t('拒绝原因'),
        width: 220,
        render: (_, r) => (
          <div>
            <div>
              {t(REJECT_LABEL[r.reject_reason] || r.reject_reason || '-')}
            </div>
            {r.review_note && (
              <Text type='tertiary' size='small'>
                {r.review_note}
              </Text>
            )}
            {r.reviewed_at > 0 && (
              <Text type='tertiary' size='small' style={{ display: 'block' }}>
                {r.reviewer_username || `#${r.reviewed_admin_id}`} ·{' '}
                {fmtTime(r.reviewed_at)}
              </Text>
            )}
          </div>
        ),
      },
      {
        title: t('操作'),
        width: 120,
        fixed: 'right',
        render: (_, r) => (
          <Popconfirm
            title={t('改为通过?')}
            content={t('{{amount}} 会立即到 {{name}} 的账户', {
              amount: fmtUsd(r.reward_usd),
              name: r.inviter_username,
            })}
            okText={t('通过')}
            cancelText={t('取消')}
            onConfirm={() => approve(r)}
          >
            <Button theme='light' type='primary' size='small' loading={acting}>
              {t('改为通过')}
            </Button>
          </Popconfirm>
        ),
      },
    );
  }

  return (
    <Space vertical style={{ width: '100%', padding: 16 }} size='large'>
      <Card>
        <Space vertical align='start' spacing='tight' style={{ width: '100%' }}>
          <Space align='center' wrap>
            <Title heading={4} style={{ margin: 0 }}>
              {t('返现审核')}
            </Title>
            <Text type='tertiary'>
              {t('好友充值后不会自动返现,需要在这里审核通过后才会到邀请人账户')}
            </Text>
          </Space>
          <Space align='center' wrap>
            <Text>
              {t('待审核')}{' '}
              <Text strong style={{ fontSize: 18 }}>
                {pendingCount}
              </Text>{' '}
              {t('笔')}
            </Text>
            <Text type='tertiary'>·</Text>
            <Text>
              {t('合计')}{' '}
              <Text strong style={{ fontSize: 18 }}>
                {fmtUsd(pendingUsd)}
              </Text>
            </Text>
          </Space>
        </Space>
      </Card>

      <Card>
        <RadioGroup
          type='button'
          value={view}
          onChange={(e) => {
            setView(e.target.value);
            setPage(1);
          }}
          style={{ marginBottom: 12 }}
        >
          {VIEWS.map((v) => (
            <Radio key={v.value} value={v.value}>
              {t(v.label)}
              {v.value === 'pending' && pendingCount > 0
                ? ` (${pendingCount})`
                : ''}
            </Radio>
          ))}
        </RadioGroup>

        <Table
          columns={columns}
          dataSource={items}
          loading={loading}
          rowKey='id'
          size='small'
          empty={
            <Empty
              description={
                view === 'pending' ? t('没有待审核的返现') : t('暂无记录')
              }
            />
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
          scroll={{ x: 1100 }}
        />
      </Card>

      <Modal
        title={t('拒绝返现')}
        visible={!!rejectTarget}
        onCancel={() => setRejectTarget(null)}
        onOk={reject}
        okText={t('确认拒绝')}
        okButtonProps={{ type: 'danger', loading: acting }}
        cancelText={t('取消')}
      >
        {rejectTarget && (
          <Space vertical align='start' style={{ width: '100%' }}>
            <Text>
              {t('邀请人')} {rejectTarget.inviter_username} · {t('返现')}{' '}
              {fmtUsd(rejectTarget.reward_usd)}
            </Text>
            <TextArea
              value={rejectNote}
              onChange={setRejectNote}
              maxCount={200}
              rows={3}
              placeholder={t('拒绝原因(选填),仅管理员可见')}
            />
            <Text type='tertiary' size='small'>
              {t('拒绝后可以在"已拒绝"里改为通过')}
            </Text>
          </Space>
        )}
      </Modal>
    </Space>
  );
};

export default AffReview;
