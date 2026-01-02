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
} from '@douyinfe/semi-ui';
import { Plus, Trash, Edit, TestTube, RefreshCw } from 'lucide-react';
import {
  createDisableRule,
  deleteDisableRule,
  getDisableRules,
  testDisableRules,
  updateDisableRule,
  refreshDisableRulesCache,
} from '../../helpers';
import { useTranslation } from 'react-i18next';

const { Header, Content } = Layout;

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

  return (
    <Card
      title={
        <Space>
          <TestTube size={16} />
          {t('规则测试')}
        </Space>
      }
      headerExtraContent={
        <Button loading={loading} onClick={handleTest}>
          {t('测试')}
        </Button>
      }
    >
      <Form
        getFormApi={(api) => (formApi.current = api)}
        labelPosition='top'
        initValues={{ status_code: '', error_message: '' }}
      >
        <Row gutter={16}>
          <Col span={8}>
            <Form.InputNumber
              field='status_code'
              label={t('状态码')}
              placeholder='429'
            />
          </Col>
          <Col span={16}>
            <Form.TextArea
              field='error_message'
              label={t('错误消息')}
              placeholder='rate limit exceeded'
              autosize={{ minRows: 2, maxRows: 4 }}
            />
          </Col>
        </Row>
      </Form>
      {result && (
        <>
          <Divider />
          <Space vertical align='start'>
            <Typography.Text>
              {t('硬编码规则匹配')}：{result.hardcoded_match ? t('是') : t('否')}
            </Typography.Text>
            <Typography.Text>
              {t('最终是否触发故障转移')}：
              {result.would_trigger_failover ? t('是') : t('否')}
            </Typography.Text>
          </Space>
          <Divider />
          <Table
            size='small'
            pagination={false}
            dataSource={result.user_rule_matches || []}
            columns={[
              { title: t('名称'), dataIndex: 'rule_name', width: 160 },
              { title: t('匹配方式'), dataIndex: 'match_type', width: 120 },
              {
                title: t('状态码匹配'),
                dataIndex: 'status_match',
                render: (v) => (v ? t('是') : t('否')),
                width: 120,
              },
              {
                title: t('关键词匹配'),
                dataIndex: 'keyword_match',
                render: (v) => (v ? t('是') : t('否')),
                width: 120,
              },
              {
                title: t('结果'),
                dataIndex: 'matched',
                render: (v, row) =>
                  row.enabled ? (v ? t('匹配') : t('未匹配')) : t('已禁用'),
                width: 120,
              },
            ]}
          />
        </>
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
      // 编辑后主动刷新缓存
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
          // 删除后主动刷新缓存
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
      // 切换状态后主动刷新缓存
      handleRefreshCache();
    } catch {
      // global error
    }
  };

  const columns = useMemo(
    () => [
      { title: t('名称'), dataIndex: 'name', width: 180 },
      {
        title: t('状态码'),
        dataIndex: 'status_codes',
        render: (arr) =>
          (arr || []).length ? (
            <Space wrap>
              {arr.map((c) => (
                <Tag key={c}>{c}</Tag>
              ))}
            </Space>
          ) : (
            '-'
          ),
        width: 200,
      },
      {
        title: t('关键词'),
        dataIndex: 'keywords',
        render: (arr) =>
          (arr || []).length ? (
            <Space wrap>
              {arr.map((k) => (
                <Tag key={k}>{k}</Tag>
              ))}
            </Space>
          ) : (
            '-'
          ),
        width: 220,
      },
      { title: t('匹配方式'), dataIndex: 'match_type', width: 130 },
      {
        title: t('优先级'),
        dataIndex: 'priority',
        width: 90,
      },
      {
        title: t('启用'),
        dataIndex: 'enabled',
        width: 100,
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
        width: 140,
        render: (_, record) => (
          <Space>
            <Button
              size='small'
              icon={<Edit size={14} />}
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
              icon={<Trash size={14} />}
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
    <Layout>
      <Content style={{ padding: '24px' }}>
        <Card
          title={t('渠道故障转移规则')}
          headerExtraContent={
            <Space>
              <Button
                icon={<RefreshCw size={16} />}
                loading={refreshLoading}
                onClick={handleRefreshCache}
              >
                {t('刷新缓存')}
              </Button>
              <Button
                icon={<Plus size={16} />}
                onClick={() => {
                  setEditing(null);
                  setModalVisible(true);
                }}
              >
                {t('新建规则')}
              </Button>
            </Space>
          }
        >
          <Banner
            type='info'
            description={t(
              '匹配成功会记录到健康检查并触发临时暂停（自动恢复），不会永久禁用渠道。编辑规则后会自动刷新缓存。',
            )}
          />
          <Spin spinning={loading}>
            <Table
              dataSource={rules}
              columns={columns}
              pagination={false}
              bordered
              rowKey='id'
              style={{ marginTop: 12 }}
              scroll={{ x: 980 }}
            />
          </Spin>

          <Divider />
          <TestPanel loading={testLoading} onTest={handleTest} result={testResult} />

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
        </Card>
      </Content>
    </Layout>
  );
};

export default FailoverRules;
