import React, { useEffect, useState } from 'react';
import {
  Card,
  Space,
  DatePicker,
  Table,
  Tag,
  Typography,
  Toast,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers/api';

const { Title, Text } = Typography;

const STATUS_COLOR = {
  pending: 'blue',
  settled: 'green',
  rejected: 'red',
  refunded: 'orange',
  offline_paid: 'purple',
};

const REJECT_LABEL = {
  same_ip: '同 IP',
  same_payment_account: '同支付账号',
  inviter_frozen: '邀请人已冻结',
};

const AffMonthlyReport = () => {
  const { t } = useTranslation();
  const now = new Date();
  const [year, setYear] = useState(now.getFullYear());
  const [month, setMonth] = useState(now.getMonth() + 1);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);

  const reload = async () => {
    setLoading(true);
    try {
      const res = await API.get(
        `/api/user/manage/aff-monthly-report?year=${year}&month=${month}`
      );
      if (res?.data?.success) {
        setData(res.data.data);
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
  }, [year, month]);

  const handleMonthChange = (date) => {
    if (!date) return;
    const d = new Date(date);
    setYear(d.getFullYear());
    setMonth(d.getMonth() + 1);
  };

  return (
    <Space vertical style={{ width: '100%', padding: 16 }} size='large'>
      <Card>
        <Space>
          <Title heading={4}>{t('一级分销返佣 - 月度对账报表')}</Title>
          <DatePicker
            type='month'
            value={`${year}-${String(month).padStart(2, '0')}`}
            onChange={handleMonthChange}
          />
        </Space>
      </Card>

      {data && (
        <>
          <Card title={t('总览(Asia/Shanghai)')} loading={loading}>
            <Space wrap spacing='loose'>
              <span>
                {t('已结算返佣 USD')}:{' '}
                <Text strong>
                  ${(data.total_settled_reward_usd || 0).toFixed(4)}
                </Text>
              </span>
              <span>
                {t('线下已返现 CNY')}:{' '}
                <Text strong>
                  ¥{(data.total_offline_paid_cny || 0).toFixed(2)}
                </Text>
              </span>
            </Space>
          </Card>

          <Card title={t('本月新建 audit log 按状态分组')} loading={loading}>
            <Table
              size='small'
              dataSource={data.total_audit_logs_created || []}
              rowKey='status'
              pagination={false}
              columns={[
                {
                  title: t('状态'),
                  dataIndex: 'status',
                  render: (s) => (
                    <Tag color={STATUS_COLOR[s] || 'grey'}>{s}</Tag>
                  ),
                },
                { title: t('数量'), dataIndex: 'count' },
              ]}
            />
          </Card>

          <Card title={t('反作弊拒绝明细')} loading={loading}>
            <Table
              size='small'
              dataSource={data.total_rejected_count_by_reason || []}
              rowKey='reject_reason'
              pagination={false}
              columns={[
                {
                  title: t('原因'),
                  dataIndex: 'reject_reason',
                  render: (r) => REJECT_LABEL[r] || r,
                },
                { title: t('数量'), dataIndex: 'count' },
              ]}
            />
          </Card>

          <Card title={t('Top 10 邀请人(已结算 USD 排序)')} loading={loading}>
            <Table
              size='small'
              dataSource={data.top_inviters || []}
              rowKey='inviter_user_id'
              pagination={false}
              columns={[
                { title: 'User ID', dataIndex: 'inviter_user_id', width: 100 },
                { title: t('用户名'), dataIndex: 'username' },
                {
                  title: t('已结算 USD'),
                  dataIndex: 'settled_usd',
                  render: (v) => `$${(v || 0).toFixed(4)}`,
                },
              ]}
            />
          </Card>
        </>
      )}
    </Space>
  );
};

export default AffMonthlyReport;
