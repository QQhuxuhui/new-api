import React, { useEffect, useState, useRef } from 'react';
import {
  Banner,
  Button,
  Form,
  Row,
  Col,
  Typography,
  Spin,
  Tag,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

const DEFAULTS = {
  EpUsdtApiUrl: '',
  EpUsdtApiToken: '',
  EpUsdtCreateOrderPath: '/api/v1/order/create-transaction',
  EpUsdtMinTopUp: 1,
  EpUsdtTestMode: false,
  EpUsdtCnyRate: 7.2,
  EpUsdtRateAuto: false,
  EpUsdtRateSource: 'binance',
  EpUsdtRateInterval: 10,
  EpUsdtRateMargin: 0.005,
  EpUsdtRateMin: 5.0,
  EpUsdtRateMax: 10.0,
  EpUsdtRateStaleSec: 3600,
  EpUsdtRateUpdatedAt: 0,
};

export default function SettingsPaymentGatewayUsdt(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [inputs, setInputs] = useState(DEFAULTS);
  const formApiRef = useRef(null);

  useEffect(() => {
    if (!props.options || !formApiRef.current) return;
    const current = {
      EpUsdtApiUrl: props.options.EpUsdtApiUrl || '',
      EpUsdtApiToken: props.options.EpUsdtApiToken || '',
      EpUsdtCreateOrderPath:
        props.options.EpUsdtCreateOrderPath ||
        '/api/v1/order/create-transaction',
      EpUsdtMinTopUp: Number(props.options.EpUsdtMinTopUp) || 1,
      EpUsdtTestMode:
        props.options.EpUsdtTestMode === true ||
        props.options.EpUsdtTestMode === 'true',
      EpUsdtCnyRate: Number(props.options.EpUsdtCnyRate) || 7.2,
      EpUsdtRateAuto:
        props.options.EpUsdtRateAuto === true ||
        props.options.EpUsdtRateAuto === 'true',
      EpUsdtRateSource: props.options.EpUsdtRateSource || 'binance',
      EpUsdtRateInterval: Number(props.options.EpUsdtRateInterval) || 10,
      EpUsdtRateMargin: Number(props.options.EpUsdtRateMargin) || 0.005,
      EpUsdtRateMin: Number(props.options.EpUsdtRateMin) || 5,
      EpUsdtRateMax: Number(props.options.EpUsdtRateMax) || 10,
      EpUsdtRateStaleSec: Number(props.options.EpUsdtRateStaleSec) || 3600,
      EpUsdtRateUpdatedAt: Number(props.options.EpUsdtRateUpdatedAt) || 0,
    };
    setInputs(current);
    formApiRef.current.setValues(current);
  }, [props.options]);

  const handleFormChange = (values) => {
    setInputs((prev) => ({ ...prev, ...values }));
  };

  const submitSettings = async () => {
    if (inputs.EpUsdtRateMin >= inputs.EpUsdtRateMax) {
      showError(t('汇率下限必须小于上限'));
      return;
    }
    if (inputs.EpUsdtRateAuto && inputs.EpUsdtRateInterval < 5) {
      showError(t('自动汇率刷新间隔最小 5 分钟'));
      return;
    }
    if (inputs.EpUsdtCnyRate <= 0) {
      showError(t('CNY→USDT 汇率必须 > 0'));
      return;
    }

    setLoading(true);
    try {
      const payload = [
        { key: 'EpUsdtApiUrl', value: inputs.EpUsdtApiUrl },
        // token 仅在非空时下发（避免空字符串覆盖已设置的 token，与 Stripe/Creem 同行为）
        ...(inputs.EpUsdtApiToken
          ? [{ key: 'EpUsdtApiToken', value: inputs.EpUsdtApiToken }]
          : []),
        {
          key: 'EpUsdtCreateOrderPath',
          value:
            inputs.EpUsdtCreateOrderPath || '/api/v1/order/create-transaction',
        },
        { key: 'EpUsdtMinTopUp', value: String(Math.max(1, parseInt(inputs.EpUsdtMinTopUp) || 1)) },
        { key: 'EpUsdtTestMode', value: inputs.EpUsdtTestMode ? 'true' : 'false' },
        { key: 'EpUsdtCnyRate', value: String(inputs.EpUsdtCnyRate) },
        { key: 'EpUsdtRateAuto', value: inputs.EpUsdtRateAuto ? 'true' : 'false' },
        { key: 'EpUsdtRateSource', value: inputs.EpUsdtRateSource },
        { key: 'EpUsdtRateInterval', value: String(Math.max(5, parseInt(inputs.EpUsdtRateInterval) || 10)) },
        { key: 'EpUsdtRateMargin', value: String(inputs.EpUsdtRateMargin) },
        { key: 'EpUsdtRateMin', value: String(inputs.EpUsdtRateMin) },
        { key: 'EpUsdtRateMax', value: String(inputs.EpUsdtRateMax) },
        { key: 'EpUsdtRateStaleSec', value: String(inputs.EpUsdtRateStaleSec) },
      ];
      const results = await Promise.all(
        payload.map((opt) => API.put('/api/option/', opt)),
      );
      const failed = results.filter((r) => !r.data.success);
      if (failed.length > 0) {
        failed.forEach((r) => showError(r.data.message));
      } else {
        showSuccess(t('更新成功'));
        props.refresh?.();
      }
    } catch (e) {
      showError(t('更新失败'));
    } finally {
      setLoading(false);
    }
  };

  const refreshRateNow = async () => {
    setRefreshing(true);
    try {
      const res = await API.post('/api/user/usdt/rate/refresh');
      if (res.data.success) {
        showSuccess(t('已触发汇率刷新, 请稍后回到本页查看结果'));
        setTimeout(() => props.refresh?.(), 3000);
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(t('请求失败'));
    } finally {
      setRefreshing(false);
    }
  };

  const fmtUpdatedAt = (ts) => {
    if (!ts || ts <= 0) return t('从未刷新');
    const d = new Date(ts * 1000);
    if (Number.isNaN(d.getTime())) return t('未知');
    return d.toLocaleString();
  };

  return (
    <Spin spinning={loading}>
      <Form
        initValues={inputs}
        onValueChange={handleFormChange}
        getFormApi={(api) => (formApiRef.current = api)}
      >
        <Form.Section text={t('USDT (TRC20) 设置')}>
          <Banner
            type='warning'
            description={t(
              '需要先自部署 assimon/epusdt 网关。计价: 系统按 CNY 算出订单金额, 再除 EpUsdtCnyRate 得 USDT。请按市价维护汇率, 自动模式开启后由后台 goroutine 从公网拉取。',
            )}
          />

          <Row gutter={{ xs: 8, sm: 16, md: 24 }}>
            <Col xs={24} sm={24} md={12}>
              <Form.Input
                field='EpUsdtApiUrl'
                label={t('网关 URL')}
                placeholder='https://usdt.example.com'
              />
            </Col>
            <Col xs={24} sm={24} md={12}>
              <Form.Input
                field='EpUsdtApiToken'
                label={t('API Token (签名密钥)')}
                placeholder={t('为空表示不修改已保存的 token')}
                type='password'
              />
            </Col>
            <Col xs={24} sm={24} md={12}>
              <Form.Input
                field='EpUsdtCreateOrderPath'
                label={t('下单 API 路径')}
                placeholder='/api/v1/order/create-transaction'
                extraText={t(
                  'assimon v0.x 默认即可；新版 ePUSDT 或 GMPay 兼容接口请按其文档填写',
                )}
              />
            </Col>
            <Col xs={24} sm={24} md={8}>
              <Form.InputNumber
                field='EpUsdtMinTopUp'
                label={t('USDT 最小充值 (USD 面值)')}
                min={1}
                precision={0}
                style={{ width: '100%' }}
              />
            </Col>
            <Col xs={24} sm={24} md={8}>
              <Form.Switch
                field='EpUsdtTestMode'
                label={t('测试模式 (跳过签名校验)')}
                extraText={t('仅开发环境使用')}
              />
            </Col>
          </Row>
        </Form.Section>

        <Form.Section text={t('CNY → USDT 汇率')}>
          <Row gutter={{ xs: 8, sm: 16, md: 24 }}>
            <Col xs={24} sm={24} md={8}>
              <Form.InputNumber
                field='EpUsdtCnyRate'
                label={t('当前汇率 (1 USDT = ? CNY)')}
                min={0.0001}
                precision={4}
                style={{ width: '100%' }}
                disabled={inputs.EpUsdtRateAuto}
                extraText={inputs.EpUsdtRateAuto ? t('自动模式: 由后台 goroutine 维护') : t('手动模式: 请按市价填写')}
              />
            </Col>
            <Col xs={24} sm={24} md={8}>
              <Form.Switch
                field='EpUsdtRateAuto'
                label={t('启用自动汇率')}
                extraText={t('启动后台 goroutine 定时拉取')}
              />
            </Col>
            <Col xs={24} sm={24} md={8}>
              <Form.Select
                field='EpUsdtRateSource'
                label={t('主源')}
                style={{ width: '100%' }}
                optionList={[
                  { label: 'Binance C2C', value: 'binance' },
                  { label: 'CoinGecko', value: 'coingecko' },
                ]}
              />
            </Col>
          </Row>

          <Row gutter={{ xs: 8, sm: 16, md: 24 }}>
            <Col xs={24} sm={24} md={6}>
              <Form.InputNumber
                field='EpUsdtRateInterval'
                label={t('刷新间隔 (分钟, ≥5)')}
                min={5}
                precision={0}
                style={{ width: '100%' }}
              />
            </Col>
            <Col xs={24} sm={24} md={6}>
              <Form.InputNumber
                field='EpUsdtRateMargin'
                label={t('加价幅度 (0.005=0.5%)')}
                min={0}
                max={0.5}
                precision={4}
                step={0.001}
                style={{ width: '100%' }}
              />
            </Col>
            <Col xs={24} sm={24} md={6}>
              <Form.InputNumber
                field='EpUsdtRateMin'
                label={t('汇率下限 (异常护栏)')}
                min={0.1}
                precision={2}
                style={{ width: '100%' }}
              />
            </Col>
            <Col xs={24} sm={24} md={6}>
              <Form.InputNumber
                field='EpUsdtRateMax'
                label={t('汇率上限 (异常护栏)')}
                min={1}
                precision={2}
                style={{ width: '100%' }}
              />
            </Col>
          </Row>

          <Row gutter={{ xs: 8, sm: 16, md: 24 }}>
            <Col xs={24} sm={24} md={12}>
              <Form.InputNumber
                field='EpUsdtRateStaleSec'
                label={t('陈旧阈值 (秒, 超过则下单拒绝)')}
                min={60}
                precision={0}
                style={{ width: '100%' }}
              />
            </Col>
            <Col xs={24} sm={24} md={12}>
              <div style={{ marginTop: 30 }}>
                <Text type='tertiary'>
                  {t('上次成功更新')}:&nbsp;
                  <Tag color={inputs.EpUsdtRateUpdatedAt ? 'green' : 'grey'}>
                    {fmtUpdatedAt(inputs.EpUsdtRateUpdatedAt)}
                  </Tag>
                </Text>
                <Button
                  size='small'
                  style={{ marginLeft: 12 }}
                  loading={refreshing}
                  disabled={!inputs.EpUsdtRateAuto}
                  onClick={refreshRateNow}
                >
                  {t('立即刷新汇率')}
                </Button>
              </div>
            </Col>
          </Row>
        </Form.Section>

        <Button onClick={submitSettings} type='primary' style={{ marginTop: 16 }}>
          {t('保存 USDT 设置')}
        </Button>
      </Form>
    </Spin>
  );
}
