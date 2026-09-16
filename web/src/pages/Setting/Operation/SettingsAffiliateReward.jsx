import React, { useEffect, useRef, useState } from 'react';
import {
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

// InviterRewardCutoffMs 是只读展示(写入路径在"月度报表 → 历史 pending 一键归档")
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
          // 不能用 `parseFloat(...) || 10` — "0" 是合法配置值(临时停发新返佣),
          // 但会被 || 当成 falsy 还原成 10,刷新后保存又把 0 覆盖回 10。
          const n = parseFloat(item.value);
          if (Number.isFinite(n)) next[item.key] = n;
        } else if (item.key === 'InviterRewardCutoffMs') {
          const n = parseInt(item.value, 10);
          setCutoffMs(Number.isFinite(n) && n >= 0 ? n : 0);
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

  const handleSubmit = async (values) => {
    const pct = Number(values.InviterRewardDefaultPercent);
    if (!(pct >= 0 && pct <= 100)) {
      showError(t('返佣比例必须在 0-100 之间'));
      return;
    }
    setLoading(true);
    try {
      await API.put('/api/option/', {
        key: 'InviterRewardDefaultPercent',
        value: String(pct),
      });
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
            '被邀请人完成充值后生成一条待审核返现;管理员在"返现审核"页通过后,返佣才会到邀请人 AffQuota(站内额度,不可提现)。不会自动返现。',
          )}
        </Text>
      </div>

      <Banner
        fullMode={false}
        type='info'
        description={t(
          '提示:返佣比例仅对修改后新产生的返现记录生效;已存在的待审核记录按其生成时冻结的比例入账。',
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
          extraText={t('0-100 范围内的小数。设为 0 即暂停产生新的返现记录金额。')}
          style={{ width: '100%' }}
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
