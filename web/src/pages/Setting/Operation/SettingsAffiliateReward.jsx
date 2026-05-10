import React, { useEffect, useRef, useState } from 'react';
import {
  Card,
  Form,
  Button,
  Banner,
  Typography,
  Spin,
  Tag,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../../helpers/api';
import { showError, showSuccess } from '../../../helpers';

const { Title, Text } = Typography;

const AFF_KEYS = [
  'InviterRewardDefaultPercent', // float 0-100
  'InviterRewardCooldownDays', // int >= 1
  'EnableAffAutoSettle', // bool
];

// InviterRewardCutoffMs 是只读展示(写入路径在"月度报表 → 历史 pending 一键归档")
const READ_ONLY_KEYS = ['InviterRewardCutoffMs'];

const formatCutoffMs = (ms) => {
  const n = Number(ms || 0);
  if (!n || n <= 0) return '未启用';
  try {
    const d = new Date(n);
    if (isNaN(d.getTime())) return String(ms);
    const pad = (x) => String(x).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())} (${n})`;
  } catch (_) {
    return String(ms);
  }
};

const SettingsAffiliateReward = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [config, setConfig] = useState({
    InviterRewardDefaultPercent: 10,
    InviterRewardCooldownDays: 7,
    EnableAffAutoSettle: true,
  });
  const [cutoffMs, setCutoffMs] = useState(0);
  const formApiRef = useRef(null);

  const loadConfig = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/option/');
      const { success, data } = res.data || {};
      if (!success) {
        showError(t('加载分销返佣配置失败'));
        return;
      }
      const next = { ...config };
      data.forEach((item) => {
        if (item.key === 'InviterRewardDefaultPercent') {
          next[item.key] = parseFloat(item.value) || 10;
        } else if (item.key === 'InviterRewardCooldownDays') {
          next[item.key] = parseInt(item.value) || 7;
        } else if (item.key === 'EnableAffAutoSettle') {
          next[item.key] = item.value === 'true' || item.value === true;
        } else if (item.key === 'InviterRewardCutoffMs') {
          setCutoffMs(parseInt(item.value) || 0);
        }
      });
      setConfig(next);
      formApiRef.current?.setValues(next);
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadConfig();
    // eslint-disable-next-line
  }, []);

  const updateOne = (key, value) =>
    API.put('/api/option/', { key, value });

  const handleSubmit = async (values) => {
    // 校验
    const pct = Number(values.InviterRewardDefaultPercent);
    if (!(pct >= 0 && pct <= 100)) {
      showError(t('返佣比例必须在 0-100 之间'));
      return;
    }
    const cooldown = Number(values.InviterRewardCooldownDays);
    if (!(cooldown >= 1 && cooldown <= 365)) {
      showError(t('冷却天数必须在 1-365 之间'));
      return;
    }

    setLoading(true);
    try {
      await updateOne('InviterRewardDefaultPercent', String(pct));
      await updateOne('InviterRewardCooldownDays', String(cooldown));
      await updateOne(
        'EnableAffAutoSettle',
        String(!!values.EnableAffAutoSettle),
      );
      showSuccess(t('分销返佣配置已保存'));
      await loadConfig();
    } catch (e) {
      showError(e?.response?.data?.message || e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Spin spinning={loading}>
      <div style={{ marginBottom: 12 }}>
        <Title heading={5}>{t('一级分销返佣')}</Title>
        <Text type='tertiary' style={{ fontSize: 13 }}>
          {t(
            '一级分销返佣系统:被邀请人通过支付完成充值后,经冷却期自动结算返佣到邀请人 AffQuota(站内额度,不可提现)。',
          )}
        </Text>
      </div>

      <Banner
        fullMode={false}
        type='info'
        description={t(
          '提示:返佣比例与冷却天数仅对修改后**新写入**的 audit log 生效;已存在的 pending 记录使用其写入时冻结的比例和到期时间结算。',
        )}
        closeIcon={null}
        style={{ marginBottom: 12 }}
      />

      <Form
        getFormApi={(api) => (formApiRef.current = api)}
        initValues={config}
        onSubmit={handleSubmit}
      >
        <Form.InputNumber
          field='InviterRewardDefaultPercent'
          label={t('返佣比例 (%)')}
          min={0}
          max={100}
          step={0.5}
          extraText={t(
            '0-100 范围内的小数。新自动结算与现有 admin 手动 payout 共享此变量。',
          )}
          style={{ width: '100%' }}
        />

        <Form.InputNumber
          field='InviterRewardCooldownDays'
          label={t('冷却天数')}
          min={1}
          max={365}
          step={1}
          extraText={t(
            '充值成功后多少天进入自动结算池。默认 7 天,作为安全垫覆盖大多数退款窗口。',
          )}
          style={{ width: '100%' }}
        />

        <Form.Switch
          field='EnableAffAutoSettle'
          label={t('启用自动结算')}
          extraText={t(
            '总开关。关闭时所有 audit log 仍正常写入,但 cron 不结算到 AffQuota。出问题可一键关停。',
          )}
        />

        {/* 只读字段:历史截断点 */}
        <Form.Slot label={t('历史截断点 (cutoff_ms)')}>
          <div style={{ paddingTop: 6 }}>
            <Tag color={cutoffMs > 0 ? 'orange' : 'grey'}>
              {formatCutoffMs(cutoffMs)}
            </Tag>
            <Text
              type='tertiary'
              style={{ fontSize: 12, marginLeft: 8, display: 'block', marginTop: 4 }}
            >
              {t(
                '此值由"分销月度报表 → 历史 pending 一键归档为 legacy"按钮设置,用于"摆脱历史包袱"场景。0 = 未启用截断。',
              )}
            </Text>
          </div>
        </Form.Slot>

        <div style={{ marginTop: 16 }}>
          <Button
            type='primary'
            theme='solid'
            htmlType='submit'
            loading={loading}
          >
            {t('保存分销返佣配置')}
          </Button>
        </div>
      </Form>
    </Spin>
  );
};

export default SettingsAffiliateReward;
