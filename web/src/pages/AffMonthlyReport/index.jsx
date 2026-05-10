import React, { useEffect, useState } from 'react';
import {
  Card,
  Space,
  DatePicker,
  Table,
  Tag,
  Typography,
  Toast,
  Button,
  Modal,
  Form,
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
  legacy: 'grey',
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
  const [legacyModalVisible, setLegacyModalVisible] = useState(false);
  const [legacyFormApi, setLegacyFormApi] = useState(null);
  const [legacySubmitting, setLegacySubmitting] = useState(false);

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

  const handleLegacyMigrate = async (values) => {
    if (!values?.cutoff_date) {
      Toast.error(t('请选择截断时间'));
      return;
    }
    const cutoffMs = new Date(values.cutoff_date).getTime();
    if (cutoffMs <= 0) {
      Toast.error(t('cutoff 时间无效'));
      return;
    }
    if (cutoffMs > Date.now()) {
      Toast.error(t('cutoff 不能选择未来时间'));
      return;
    }
    setLegacySubmitting(true);
    try {
      const res = await API.post('/api/user/manage/aff-audit-logs/mark-legacy', {
        cutoff_ms: cutoffMs,
      });
      if (res?.data?.success) {
        Toast.success(
          t('已迁移 {{n}} 条 pending 为 legacy', {
            n: res.data.data.migrated,
          })
        );
        setLegacyModalVisible(false);
      } else {
        Toast.error(res?.data?.message || t('迁移失败'));
      }
    } catch (e) {
      Toast.error(e.response?.data?.message || e.message);
    } finally {
      setLegacySubmitting(false);
    }
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
          <Button
            type='warning'
            theme='light'
            onClick={() => setLegacyModalVisible(true)}
          >
            {t('历史 pending 一键归档为 legacy')}
          </Button>
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

      <Modal
        title={t('历史 pending 一键归档为 legacy')}
        visible={legacyModalVisible}
        onCancel={() => setLegacyModalVisible(false)}
        footer={null}
      >
        <Text type='warning' style={{ display: 'block', marginBottom: 12 }}>
          {t(
            '此操作会把所有 created_at 早于截断时间的 status="pending" log 改为 legacy。legacy 不会被自动结算 cron 处理,但仍会显示在审计列表里。操作不可逆,请谨慎。'
          )}
        </Text>
        <Form
          getFormApi={(api) => setLegacyFormApi(api)}
          onSubmit={handleLegacyMigrate}
        >
          <Form.DatePicker
            field='cutoff_date'
            label={t('截断时间(此时间之前的 pending 全部归档)')}
            type='dateTime'
            rules={[{ required: true, message: t('必填') }]}
            style={{ width: '100%' }}
            disabledDate={(date) => date && date.getTime() > Date.now()}
          />
          <Space style={{ marginTop: 12 }}>
            <Button
              type='warning'
              theme='solid'
              htmlType='submit'
              loading={legacySubmitting}
            >
              {t('确认归档')}
            </Button>
            <Button onClick={() => setLegacyModalVisible(false)}>
              {t('取消')}
            </Button>
          </Space>
        </Form>
      </Modal>
    </Space>
  );
};

export default AffMonthlyReport;
