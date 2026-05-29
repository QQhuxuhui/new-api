/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/
import React, { useEffect, useState } from 'react';
import {
  Button, Card, Table, Switch, Input, InputNumber, Space, Typography,
  Modal, Banner, Popconfirm, Select, Empty, Tag, Spin,
} from '@douyinfe/semi-ui';
import { IconPlus, IconDelete, IconRefresh } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const genLocalId = () =>
  'r_' + Date.now().toString(36) + Math.random().toString(36).slice(2, 8);

const ErrorCapture = () => {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(false);
  const [rules, setRules] = useState([]);
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(false);

  const [selectedRuleId, setSelectedRuleId] = useState('');
  const [logs, setLogs] = useState([]);
  const [logTotal, setLogTotal] = useState(0);
  const [logPage, setLogPage] = useState(1);
  const [detail, setDetail] = useState(null);

  const loadConfig = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/option/');
      if (!res.data.success) {
        showError(res.data.message);
        return;
      }
      const opts = res.data.data || [];
      let en = false;
      let rs = [];
      opts.forEach((o) => {
        if (o.key === 'error_capture_setting.enabled') en = o.value === 'true';
        if (o.key === 'error_capture_setting.rules') {
          try { rs = JSON.parse(o.value || '[]'); } catch (e) { rs = []; }
        }
      });
      setEnabled(en);
      setRules(Array.isArray(rs) ? rs : []);
    } catch (e) {
      // 错误已由全局拦截器提示
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadConfig(); }, []);

  const updateOption = async (key, value) => {
    const res = await API.put('/api/option/', { key, value });
    if (!res.data.success) throw new Error(res.data.message);
  };

  const saveAll = async () => {
    setSaving(true);
    try {
      await updateOption('error_capture_setting.enabled', enabled ? 'true' : 'false');
      await updateOption('error_capture_setting.rules', JSON.stringify(rules));
      showSuccess(t('保存成功'));
      await loadConfig();
    } catch (e) {
      showError(e.message);
    } finally {
      setSaving(false);
    }
  };

  const addRule = () => {
    setRules([...rules, { id: genLocalId(), keyword: '', label: '', enabled: true, max_records: 100, _local: true }]);
  };
  const updateRule = (idx, patch) => {
    const next = rules.slice();
    next[idx] = { ...next[idx], ...patch };
    setRules(next);
  };
  const removeRule = (idx) => {
    const next = rules.slice();
    next.splice(idx, 1);
    setRules(next);
  };

  const loadLogs = async (ruleId, page = 1) => {
    if (!ruleId) return;
    try {
      const res = await API.get(`/api/error_capture/logs?rule_id=${encodeURIComponent(ruleId)}&p=${page}&page_size=20`);
      if (!res.data.success) { showError(res.data.message); return; }
      setLogs(res.data.data.items || []);
      setLogTotal(res.data.data.total || 0);
      setLogPage(page);
    } catch (e) {
      // handled globally
    }
  };

  const openDetail = async (id) => {
    try {
      const res = await API.get(`/api/error_capture/logs/${id}`);
      if (!res.data.success) { showError(res.data.message); return; }
      setDetail(res.data.data);
    } catch (e) {
      // handled globally
    }
  };

  const clearRuleLogs = async (ruleId) => {
    try {
      const res = await API.delete(`/api/error_capture/logs?rule_id=${encodeURIComponent(ruleId)}`);
      if (!res.data.success) { showError(res.data.message); return; }
      showSuccess(t('已清空'));
      loadLogs(ruleId, 1);
    } catch (e) {
      // handled globally
    }
  };

  const ruleColumns = [
    { title: t('备注'), dataIndex: 'label', render: (v, r, i) => (
      <Input value={v} placeholder={t('备注名')} onChange={(val) => updateRule(i, { label: val })} />
    )},
    { title: t('关键词（详情包含即匹配）'), dataIndex: 'keyword', render: (v, r, i) => (
      <Input value={v} placeholder={t('如 insufficient_quota')} onChange={(val) => updateRule(i, { keyword: val })} />
    )},
    { title: t('保留条数'), dataIndex: 'max_records', width: 120, render: (v, r, i) => (
      <InputNumber min={1} max={1000} value={v} onChange={(val) => updateRule(i, { max_records: val })} />
    )},
    { title: t('启用'), dataIndex: 'enabled', width: 90, render: (v, r, i) => (
      <Switch checked={v} onChange={(val) => updateRule(i, { enabled: val })} />
    )},
    { title: t('操作'), width: 80, render: (_, r, i) => (
      <Button icon={<IconDelete />} type='danger' theme='borderless' onClick={() => removeRule(i)} />
    )},
  ];

  const logColumns = [
    { title: t('时间'), dataIndex: 'created_at', width: 170,
      render: (v) => new Date(v * 1000).toLocaleString() },
    { title: t('用户'), dataIndex: 'username', width: 120 },
    { title: t('模型'), dataIndex: 'model_name', width: 160 },
    { title: t('状态码'), dataIndex: 'status_code', width: 90,
      render: (v) => <Tag color='red'>{v}</Tag> },
    { title: t('错误详情'), dataIndex: 'content', ellipsis: true },
    { title: t('操作'), width: 100,
      render: (_, r) => <Button theme='borderless' onClick={() => openDetail(r.id)}>{t('查看请求')}</Button> },
  ];

  const savedRules = rules.filter((r) => r.id && !r._local);

  return (
    <div style={{ padding: 16 }}>
      <Card style={{ marginBottom: 16 }}>
        <Banner type='warning' description={t('捕获的请求体可能包含用户敏感数据，仅超级管理员可见。每个请求最多记录一次，每条规则只保留最近设定的条数。')} />
        <Space style={{ marginTop: 12, marginBottom: 12 }}>
          <Typography.Text strong>{t('总开关')}</Typography.Text>
          <Switch checked={enabled} onChange={setEnabled} />
          <Button icon={<IconPlus />} onClick={addRule}>{t('添加规则')}</Button>
          <Button theme='solid' loading={saving} onClick={saveAll}>{t('保存配置')}</Button>
        </Space>
        <Spin spinning={loading}>
          <Table columns={ruleColumns} dataSource={rules} pagination={false} rowKey={(r) => r.id || r.keyword} />
        </Spin>
      </Card>

      <Card title={t('抓取记录')}>
        <Space style={{ marginBottom: 12 }}>
          <Select
            placeholder={t('选择规则查看记录')}
            style={{ width: 280 }}
            value={selectedRuleId || undefined}
            onChange={(v) => { setSelectedRuleId(v); loadLogs(v, 1); }}
            optionList={savedRules.map((r) => ({
              label: (r.label ? r.label + ' — ' : '') + r.keyword, value: r.id,
            }))}
          />
          <Button icon={<IconRefresh />} disabled={!selectedRuleId} onClick={() => loadLogs(selectedRuleId, logPage)}>{t('刷新')}</Button>
          {selectedRuleId && (
            <Popconfirm title={t('确认清空该规则下所有记录？')} onConfirm={() => clearRuleLogs(selectedRuleId)}>
              <Button type='danger'>{t('清空记录')}</Button>
            </Popconfirm>
          )}
        </Space>
        {selectedRuleId ? (
          <Table
            columns={logColumns}
            dataSource={logs}
            rowKey='id'
            pagination={{
              currentPage: logPage,
              pageSize: 20,
              total: logTotal,
              onPageChange: (p) => loadLogs(selectedRuleId, p),
            }}
          />
        ) : <Empty description={t('请选择规则')} />}
      </Card>

      <Modal
        title={t('完整请求数据')}
        visible={!!detail}
        onCancel={() => setDetail(null)}
        footer={null}
        width={760}
      >
        {detail && (
          <div>
            <Typography.Paragraph>
              <b>{t('路径')}:</b> {detail.request_path} &nbsp; <b>{t('渠道')}:</b> {detail.channel_id}
            </Typography.Paragraph>
            <Typography.Paragraph><b>{t('错误详情')}:</b> {detail.content}</Typography.Paragraph>
            <Typography.Text strong>{t('请求体')}:</Typography.Text>
            <pre style={{ maxHeight: 420, overflow: 'auto', background: 'var(--semi-color-fill-0)', padding: 12, borderRadius: 6 }}>
              {(() => {
                try { return JSON.stringify(JSON.parse(detail.request_body), null, 2); }
                catch (e) { return detail.request_body; }
              })()}
            </pre>
          </div>
        )}
      </Modal>
    </div>
  );
};

export default ErrorCapture;
