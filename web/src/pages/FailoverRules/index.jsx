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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Divider,
  Form,
  Modal,
  Row,
  Space,
  Table,
  Tag,
  Toast,
  Typography,
  Banner,
  Switch,
  Spin,
  Layout,
  Empty,
} from '@douyinfe/semi-ui';
import {
  IconPlus,
  IconDelete,
  IconEdit,
  IconRefresh,
  IconSetting,
  IconTickCircle,
  IconMinusCircle,
  IconInfoCircle,
} from '@douyinfe/semi-icons';
import {
  createDisableRule,
  deleteDisableRule,
  getDisableRules,
  testDisableRules,
  updateDisableRule,
  refreshDisableRulesCache,
} from '../../helpers';
import { useTranslation } from 'react-i18next';

const { Content } = Layout;
const { Title, Text } = Typography;

const matchTypeOptions = [
  { label: 'AND', value: 'AND' },
  { label: 'OR', value: 'OR' },
  { label: 'STATUS_ONLY', value: 'STATUS_ONLY' },
  { label: 'KEYWORD_ONLY', value: 'KEYWORD_ONLY' },
];

function parseIntArray(arr) {
  return (arr || [])
    .map((v) => parseInt(v, 10))
    .filter((v) => !Number.isNaN(v));
}

const RuleModal = ({ visible, onCancel, onOk, initial }) => {
  const { t } = useTranslation();
  const formApi = useRef(null);
  const isEdit = !!initial?.id;

  const handleSubmit = () => {
    const values = formApi.current?.getValues() || {};
    const payload = {
      ...values,
      status_codes: parseIntArray(values.status_codes),
      keywords: (values.keywords || []).map((k) => k.trim()).filter(Boolean),
    };
    onOk(payload);
  };

  return (
    <Modal
      title={isEdit ? t('编辑规则') : t('新建规则')}
      visible={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      okText={t('保存')}
      cancelText={t('取消')}
      centered
      width={640}
    >
      <Form
        getFormApi={(api) => (formApi.current = api)}
        labelPosition='top'
        initValues={{
          name: '',
          match_type: 'AND',
          status_codes: (initial?.status_codes || []).map((v) => String(v)),
          keywords: [],
          description: '',
          enabled: true,
          priority: 0,
          ...initial,
        }}
      >
        <Form.Input
          field='name'
          label={t('规则名称')}
          required
          maxLength={100}
          placeholder={t('请输入规则名称')}
        />
        <Form.RadioGroup
          field='match_type'
          label={t('匹配方式')}
          type='button'
          buttonSize='small'
          options={matchTypeOptions}
        />
        <Row gutter={16}>
          <Col span={12}>
            <Form.TagInput
              field='status_codes'
              label={t('状态码')}
              placeholder={t('输入状态码后回车，留空表示不匹配状态码')}
              allowDuplicates={false}
              validateStatus='warning'
            />
          </Col>
          <Col span={12}>
            <Form.TagInput
              field='keywords'
              label={t('关键词')}
              placeholder={t('输入关键词后回车，留空表示不匹配关键词')}
              allowDuplicates={false}
            />
          </Col>
        </Row>
        <Form.TextArea
          field='description'
          label={t('描述')}
          placeholder={t('可选，描述规则用途')}
          autosize={{ minRows: 2, maxRows: 4 }}
        />
        <Row gutter={16}>
          <Col span={12}>
            <Form.Switch field='enabled' label={t('启用')} />
          </Col>
          <Col span={12}>
            <Form.InputNumber
              field='priority'
              label={t('优先级（排序）')}
              min={-1000}
              max={1000}
            />
          </Col>
        </Row>
      </Form>
    </Modal>
  );
};

const TestPanel = ({ loading, onTest, result }) => {
  const { t } = useTranslation();
  const formApi = useRef(null);

  const handleTest = () => {
    const values = formApi.current?.getValues() || {};
    onTest({
      status_code: Number(values.status_code) || 0,
      error_message: values.error_message || '',
    });
  };

  const resultColumns = [
    { title: t('名称'), dataIndex: 'rule_name', width: 160 },
    { title: t('匹配方式'), dataIndex: 'match_type', width: 100 },
    {
      title: t('状态码匹配'),
      dataIndex: 'status_match',
      width: 100,
      render: (v) => (
        <Tag color={v ? 'green' : 'grey'} size='small'>
          {v ? t('是') : t('否')}
        </Tag>
      ),
    },
    {
      title: t('关键词匹配'),
      dataIndex: 'keyword_match',
      width: 100,
      render: (v) => (
        <Tag color={v ? 'green' : 'grey'} size='small'>
          {v ? t('是') : t('否')}
        </Tag>
      ),
    },
    {
      title: t('结果'),
      dataIndex: 'matched',
      width: 100,
      render: (v, row) => {
        if (!row.enabled) {
          return <Tag color='grey' size='small'>{t('已禁用')}</Tag>;
        }
        return v ? (
          <Tag color='red' size='small'>{t('匹配')}</Tag>
        ) : (
          <Tag color='blue' size='small'>{t('未匹配')}</Tag>
        );
      },
    },
  ];

  return (
    <Card
      style={{
        borderRadius: 8,
        border: '1px solid var(--semi-color-border)',
      }}
      bodyStyle={{ padding: 20 }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
        <Space>
          <IconSetting size='large' style={{ color: 'var(--semi-color-primary)' }} />
          <Title heading={5} style={{ margin: 0 }}>{t('规则测试')}</Title>
        </Space>
        <Button
          theme='solid'
          type='primary'
          loading={loading}
          onClick={handleTest}
          icon={<IconSetting />}
        >
          {t('执行测试')}
        </Button>
      </div>

      <Form
        getFormApi={(api) => (formApi.current = api)}
        labelPosition='top'
        initValues={{ status_code: '', error_message: '' }}
      >
        <Row gutter={16}>
          <Col span={6}>
            <Form.InputNumber
              field='status_code'
              label={t('状态码')}
              placeholder='例如: 429'
              style={{ width: '100%' }}
            />
          </Col>
          <Col span={18}>
            <Form.TextArea
              field='error_message'
              label={t('错误消息')}
              placeholder={t('例如: rate limit exceeded, quota exceeded')}
              autosize={{ minRows: 1, maxRows: 3 }}
            />
          </Col>
        </Row>
      </Form>

      {result && (
        <div style={{ marginTop: 20 }}>
          <Divider margin={16} />

          <Row gutter={24} style={{ marginBottom: 16 }}>
            <Col span={12}>
              <div
                style={{
                  padding: '12px 16px',
                  borderRadius: 6,
                  background: result.hardcoded_match
                    ? 'var(--semi-color-warning-light-default)'
                    : 'var(--semi-color-fill-0)',
                  border: `1px solid ${result.hardcoded_match ? 'var(--semi-color-warning)' : 'var(--semi-color-border)'}`,
                }}
              >
                <Space>
                  <IconInfoCircle style={{ color: result.hardcoded_match ? 'var(--semi-color-warning)' : 'var(--semi-color-text-2)' }} />
                  <Text type='secondary'>{t('硬编码规则匹配')}</Text>
                  <Text strong>{result.hardcoded_match ? t('是') : t('否')}</Text>
                </Space>
              </div>
            </Col>
            <Col span={12}>
              <div
                style={{
                  padding: '12px 16px',
                  borderRadius: 6,
                  background: result.would_trigger_failover
                    ? 'var(--semi-color-danger-light-default)'
                    : 'var(--semi-color-success-light-default)',
                  border: `1px solid ${result.would_trigger_failover ? 'var(--semi-color-danger)' : 'var(--semi-color-success)'}`,
                }}
              >
                <Space>
                  {result.would_trigger_failover ? (
                    <IconMinusCircle style={{ color: 'var(--semi-color-danger)' }} />
                  ) : (
                    <IconTickCircle style={{ color: 'var(--semi-color-success)' }} />
                  )}
                  <Text type='secondary'>{t('触发故障转移')}</Text>
                  <Text strong style={{ color: result.would_trigger_failover ? 'var(--semi-color-danger)' : 'var(--semi-color-success)' }}>
                    {result.would_trigger_failover ? t('是') : t('否')}
                  </Text>
                </Space>
              </div>
            </Col>
          </Row>

          {(result.user_rule_matches || []).length > 0 && (
            <>
              <Text type='secondary' size='small' style={{ marginBottom: 8, display: 'block' }}>
                {t('用户规则匹配详情')}
              </Text>
              <Table
                size='small'
                pagination={false}
                dataSource={result.user_rule_matches || []}
                columns={resultColumns}
                rowKey='rule_name'
                style={{ borderRadius: 6, overflow: 'hidden' }}
              />
            </>
          )}
        </div>
      )}
    </Card>
  );
};

const FailoverRules = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [rules, setRules] = useState([]);
  const [modalVisible, setModalVisible] = useState(false);
  const [editing, setEditing] = useState(null);
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState(null);
  const [refreshLoading, setRefreshLoading] = useState(false);

  const loadRules = async () => {
    setLoading(true);
    try {
      const res = await getDisableRules();
      if (res.data?.success) {
        setRules(res.data.data || []);
      }
    } catch (err) {
      // handled globally
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadRules();
  }, []);

  const handleRefreshCache = async () => {
    setRefreshLoading(true);
    try {
      const res = await refreshDisableRulesCache();
      if (res.data?.success) {
        Toast.success(t('缓存刷新成功'));
      }
    } catch {
      // error toast handled globally
    } finally {
      setRefreshLoading(false);
    }
  };

  const handleSave = async (payload) => {
    try {
      if (editing) {
        await updateDisableRule(editing.id, payload);
      } else {
        await createDisableRule(payload);
      }
      Toast.success(t('保存成功'));
      setModalVisible(false);
      setEditing(null);
      loadRules();
      handleRefreshCache();
    } catch {
      // error toast handled globally
    }
  };

  const handleDelete = (rule) => {
    Modal.error({
      title: t('删除确认'),
      content: t('确定要删除该规则吗？'),
      okText: t('删除'),
      cancelText: t('取消'),
      onOk: async () => {
        try {
          await deleteDisableRule(rule.id);
          Toast.success(t('删除成功'));
          loadRules();
          handleRefreshCache();
        } catch {
          // global error
        }
      },
    });
  };

  const handleToggle = async (rule, enabled) => {
    try {
      await updateDisableRule(rule.id, { ...rule, enabled });
      loadRules();
      handleRefreshCache();
    } catch {
      // global error
    }
  };

  const columns = useMemo(
    () => [
      {
        title: t('名称'),
        dataIndex: 'name',
        width: 180,
        render: (text) => <Text strong>{text}</Text>,
      },
      {
        title: t('状态码'),
        dataIndex: 'status_codes',
        render: (arr) =>
          (arr || []).length ? (
            <Space wrap size={4}>
              {arr.map((c) => (
                <Tag key={c} color='amber' size='small' style={{ borderRadius: 4 }}>
                  {c}
                </Tag>
              ))}
            </Space>
          ) : (
            <Text type='tertiary'>-</Text>
          ),
        width: 180,
      },
      {
        title: t('关键词'),
        dataIndex: 'keywords',
        render: (arr) =>
          (arr || []).length ? (
            <Space wrap size={4}>
              {arr.map((k) => (
                <Tag key={k} color='violet' size='small' style={{ borderRadius: 4 }}>
                  {k}
                </Tag>
              ))}
            </Space>
          ) : (
            <Text type='tertiary'>-</Text>
          ),
        width: 200,
      },
      {
        title: t('匹配方式'),
        dataIndex: 'match_type',
        width: 120,
        render: (type) => (
          <Tag color='blue' size='small' style={{ borderRadius: 4 }}>
            {type}
          </Tag>
        ),
      },
      {
        title: t('优先级'),
        dataIndex: 'priority',
        width: 80,
        align: 'center',
        render: (v) => <Text type='secondary'>{v}</Text>,
      },
      {
        title: t('状态'),
        dataIndex: 'enabled',
        width: 80,
        align: 'center',
        render: (v, record) => (
          <Switch
            size='small'
            checked={v}
            onChange={(val) => handleToggle(record, val)}
          />
        ),
      },
      {
        title: t('操作'),
        width: 160,
        fixed: 'right',
        render: (_, record) => (
          <Space size={4}>
            <Button
              size='small'
              type='tertiary'
              icon={<IconEdit />}
              onClick={() => {
                setEditing(record);
                setModalVisible(true);
              }}
            >
              {t('编辑')}
            </Button>
            <Button
              size='small'
              type='danger'
              icon={<IconDelete />}
              onClick={() => handleDelete(record)}
            >
              {t('删除')}
            </Button>
          </Space>
        ),
      },
    ],
    [t, rules],
  );

  const handleTest = async (payload) => {
    setTestLoading(true);
    setTestResult(null);
    try {
      const res = await testDisableRules(payload);
      if (res.data?.success) {
        setTestResult(res.data.data);
      }
    } catch {
      // global error
    } finally {
      setTestLoading(false);
    }
  };

  return (
    <Layout style={{ minHeight: '100%', background: 'transparent' }}>
      <Content style={{ padding: '74px 24px 24px 24px' }}>
        <div style={{ maxWidth: 1400, margin: '0 auto' }}>
          {/* Page Header */}
          <div style={{ marginBottom: 24 }}>
            <Title heading={3} style={{ marginBottom: 8 }}>
              {t('渠道故障转移规则')}
            </Title>
            <Text type='tertiary'>
              {t('配置渠道错误匹配规则，当请求失败时自动触发故障转移')}
            </Text>
          </div>

          {/* Rules Card */}
          <Card
            style={{
              marginBottom: 24,
              borderRadius: 8,
              border: '1px solid var(--semi-color-border)',
            }}
            bodyStyle={{ padding: 0 }}
          >
            <div
              style={{
                padding: '16px 20px',
                borderBottom: '1px solid var(--semi-color-border)',
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
              }}
            >
              <Title heading={5} style={{ margin: 0 }}>{t('规则列表')}</Title>
              <Space>
                <Button
                  type='tertiary'
                  icon={<IconRefresh spin={refreshLoading} />}
                  loading={refreshLoading}
                  onClick={handleRefreshCache}
                >
                  {t('刷新缓存')}
                </Button>
                <Button
                  theme='solid'
                  type='primary'
                  icon={<IconPlus />}
                  onClick={() => {
                    setEditing(null);
                    setModalVisible(true);
                  }}
                >
                  {t('新建规则')}
                </Button>
              </Space>
            </div>

            <div style={{ padding: '0 20px' }}>
              <Banner
                type='info'
                icon={<IconInfoCircle />}
                description={t(
                  '匹配成功会记录到健康检查并触发临时暂停（自动恢复），不会永久禁用渠道。编辑规则后会自动刷新缓存。',
                )}
                style={{ margin: '16px 0', borderRadius: 6 }}
              />
            </div>

            <Spin spinning={loading}>
              <Table
                dataSource={rules}
                columns={columns}
                pagination={false}
                rowKey='id'
                scroll={{ x: 1000 }}
                empty={
                  <Empty
                    description={t('暂无规则，点击上方按钮创建')}
                    style={{ padding: '40px 0' }}
                  />
                }
                style={{ borderRadius: 0 }}
              />
            </Spin>
          </Card>

          {/* Test Panel */}
          <TestPanel loading={testLoading} onTest={handleTest} result={testResult} />

          {/* Rule Modal */}
          {modalVisible && (
            <RuleModal
              visible={modalVisible}
              onCancel={() => {
                setModalVisible(false);
                setEditing(null);
              }}
              onOk={handleSave}
              initial={editing}
            />
          )}
        </div>
      </Content>
    </Layout>
  );
};

export default FailoverRules;
