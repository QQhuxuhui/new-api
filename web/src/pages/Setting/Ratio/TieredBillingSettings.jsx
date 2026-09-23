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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Input,
  InputNumber,
  Modal,
  RadioGroup,
  Radio,
  Space,
  Table,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconDelete,
  IconEdit,
  IconPlus,
  IconSearch,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../../helpers';
import { useTranslation } from 'react-i18next';

const MODE_KEY = 'billing_setting.billing_mode';
const EXPR_KEY = 'billing_setting.billing_expr';
const TIERED = 'tiered_expr';
const RATIO = 'ratio';

// 常用模板：系数为真实价格（美元 / 1M tokens）
const TEMPLATES = [
  {
    label: '单档',
    expr: 'tier("standard", p * 2 + c * 8 + cr * 0.2)',
  },
  {
    label: '按上下文长度分档',
    expr: 'len <= 272000 ? tier("standard", p * 10 + c * 50 + cr * 1 + cc * 12.5) : tier("long_context", p * 20 + c * 75 + cr * 2 + cc * 25)',
  },
  {
    label: '三档',
    expr: 'len <= 32000 ? tier("0_32k", p * 0.8 + c * 8) : len <= 128000 ? tier("32k_128k", p * 1.2 + c * 16) : tier("128k_plus", p * 2.4 + c * 24)',
  },
];

const PREVIEW_FIELDS = [
  { key: 'p', label: '输入' },
  { key: 'c', label: '输出' },
  { key: 'cr', label: '缓存读' },
  { key: 'cc', label: '缓存写' },
];

const parseJSONMap = (raw) => {
  try {
    const parsed = JSON.parse(raw || '{}');
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch (e) {
    return {};
  }
};

export default function TieredBillingSettings(props) {
  const { t } = useTranslation();
  const [modes, setModes] = useState({});
  const [exprs, setExprs] = useState({});
  const [searchText, setSearchText] = useState('');
  const [loading, setLoading] = useState(false);

  const [visible, setVisible] = useState(false);
  const [editingName, setEditingName] = useState(null);
  const [form, setForm] = useState({ name: '', mode: TIERED, expr: '' });
  const [previewTokens, setPreviewTokens] = useState({
    p: 1000,
    c: 1000,
    cr: 0,
    cc: 0,
  });
  const [preview, setPreview] = useState(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  useEffect(() => {
    setModes(parseJSONMap(props.options[MODE_KEY]));
    setExprs(parseJSONMap(props.options[EXPR_KEY]));
  }, [props.options]);

  const rows = useMemo(() => {
    const names = new Set([...Object.keys(modes), ...Object.keys(exprs)]);
    return Array.from(names)
      .filter((name) => modes[name] === TIERED || exprs[name])
      .filter((name) => !searchText || name.includes(searchText))
      .sort()
      .map((name) => ({
        name,
        mode: modes[name] || RATIO,
        expr: exprs[name] || '',
      }));
  }, [modes, exprs, searchText]);

  // 先写表达式再写模式，避免出现「阶梯模式但没有表达式」的中间状态
  const persist = async (nextModes, nextExprs) => {
    setLoading(true);
    try {
      for (const [key, value] of [
        [EXPR_KEY, nextExprs],
        [MODE_KEY, nextModes],
      ]) {
        const res = await API.put('/api/option/', {
          key,
          value: JSON.stringify(value, null, 2),
        });
        if (!res?.data?.success) {
          showError(res?.data?.message || t('保存失败，请重试'));
          return false;
        }
      }
      showSuccess(t('保存成功'));
      props.refresh();
      return true;
    } catch (error) {
      showError(t('保存失败，请重试'));
      return false;
    } finally {
      setLoading(false);
    }
  };

  const openEditor = (row) => {
    setEditingName(row ? row.name : null);
    setForm(
      row
        ? { name: row.name, mode: row.mode, expr: row.expr }
        : { name: '', mode: TIERED, expr: TEMPLATES[1].expr },
    );
    setPreview(null);
    setVisible(true);
  };

  const runPreview = async () => {
    if (!form.expr.trim()) return;
    setPreviewLoading(true);
    try {
      const res = await API.post('/api/option/billing_expr/preview', {
        expr: form.expr,
        params: {
          P: Number(previewTokens.p) || 0,
          C: Number(previewTokens.c) || 0,
          CR: Number(previewTokens.cr) || 0,
          CC: Number(previewTokens.cc) || 0,
        },
      });
      const { success, message, data } = res.data;
      setPreview(success ? { ok: true, ...data } : { ok: false, message });
    } catch (error) {
      setPreview({ ok: false, message: t('预览失败') });
    } finally {
      setPreviewLoading(false);
    }
  };

  const handleSave = async () => {
    const name = form.name.trim();
    if (!name) {
      showError(t('请输入模型名称'));
      return;
    }
    if (!editingName && (modes[name] === TIERED || exprs[name])) {
      showError(t('该模型已存在阶梯计费配置'));
      return;
    }
    if (form.mode === TIERED && !form.expr.trim()) {
      showError(t('请输入计费表达式'));
      return;
    }
    const nextModes = { ...modes };
    const nextExprs = { ...exprs };
    if (editingName && editingName !== name) {
      delete nextModes[editingName];
      delete nextExprs[editingName];
    }
    nextModes[name] = form.mode;
    if (form.expr.trim()) {
      nextExprs[name] = form.expr.trim();
    } else {
      delete nextExprs[name];
    }
    if (await persist(nextModes, nextExprs)) {
      setVisible(false);
    }
  };

  const handleDelete = (row) => {
    Modal.confirm({
      title: t('确认删除'),
      content: t(
        '删除后该模型恢复为倍率计费；若为内置阶梯价模型（如 gpt-6-astra）且没有配置倍率，会重新使用内置表达式。如需停用内置表达式，请把计费模式改为「倍率计费」。',
      ),
      onOk: () => {
        const nextModes = { ...modes };
        const nextExprs = { ...exprs };
        delete nextModes[row.name];
        delete nextExprs[row.name];
        return persist(nextModes, nextExprs);
      },
    });
  };

  const columns = [
    {
      title: t('模型名称'),
      dataIndex: 'name',
      width: 220,
    },
    {
      title: t('计费模式'),
      dataIndex: 'mode',
      width: 120,
      render: (mode) =>
        mode === TIERED ? (
          <Tag color='blue'>{t('阶梯计费')}</Tag>
        ) : (
          <Tag color='grey'>{t('倍率计费')}</Tag>
        ),
    },
    {
      title: t('计费表达式'),
      dataIndex: 'expr',
      render: (expr) => (
        <Typography.Text
          ellipsis={{ showTooltip: { opts: { style: { maxWidth: 600 } } } }}
          style={{ fontFamily: 'monospace', maxWidth: 520 }}
        >
          {expr || '-'}
        </Typography.Text>
      ),
    },
    {
      title: t('操作'),
      width: 160,
      render: (_, row) => (
        <Space>
          <Button
            icon={<IconEdit />}
            size='small'
            onClick={() => openEditor(row)}
          >
            {t('编辑')}
          </Button>
          <Button
            icon={<IconDelete />}
            size='small'
            type='danger'
            onClick={() => handleDelete(row)}
          >
            {t('删除')}
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <>
      <Banner
        type='info'
        closeIcon={null}
        style={{ marginBottom: 12 }}
        description={t(
          '阶梯计费按表达式计价，系数为真实价格（美元 / 1M tokens）。变量：p 输入、c 输出、cr 缓存读、cc 缓存写(5m)、cc1h 缓存写(1h)、img 图片输入、ai 音频输入、ao 音频输出、len 输入上下文总长度（用于分档条件）。阶梯计费优先于模型倍率 / 固定价格，分组倍率、渠道倍率照常叠加。',
        )}
      />
      <Space style={{ marginBottom: 12 }}>
        <Button icon={<IconPlus />} onClick={() => openEditor(null)}>
          {t('添加阶梯计费模型')}
        </Button>
        <Input
          prefix={<IconSearch />}
          placeholder={t('搜索模型名称')}
          value={searchText}
          onChange={setSearchText}
          showClear
          style={{ width: 240 }}
        />
      </Space>
      <Table
        columns={columns}
        dataSource={rows}
        rowKey='name'
        loading={loading}
        pagination={{ pageSize: 10 }}
        empty={t('暂无阶梯计费模型')}
      />

      <Modal
        title={editingName ? t('编辑阶梯计费') : t('添加阶梯计费模型')}
        visible={visible}
        onCancel={() => setVisible(false)}
        onOk={handleSave}
        okButtonProps={{ loading }}
        width={760}
      >
        <Space vertical align='start' style={{ width: '100%' }}>
          <Typography.Text strong>{t('模型名称')}</Typography.Text>
          <Input
            value={form.name}
            placeholder='gpt-6-sol'
            onChange={(v) => setForm({ ...form, name: v })}
          />
          <Typography.Text strong>{t('计费模式')}</Typography.Text>
          <RadioGroup
            value={form.mode}
            onChange={(e) => setForm({ ...form, mode: e.target.value })}
          >
            <Radio value={TIERED}>{t('阶梯计费')}</Radio>
            <Radio value={RATIO}>{t('倍率计费（停用表达式）')}</Radio>
          </RadioGroup>
          <Space>
            <Typography.Text strong>{t('计费表达式')}</Typography.Text>
            {TEMPLATES.map((tpl) => (
              <Button
                key={tpl.label}
                size='small'
                theme='borderless'
                onClick={() => setForm({ ...form, expr: tpl.expr })}
              >
                {t(tpl.label)}
              </Button>
            ))}
          </Space>
          <TextArea
            value={form.expr}
            autosize={{ minRows: 3, maxRows: 10 }}
            style={{ fontFamily: 'monospace' }}
            onChange={(v) => setForm({ ...form, expr: v })}
          />
          <Typography.Text strong>{t('试算')}</Typography.Text>
          <Space wrap>
            {PREVIEW_FIELDS.map((f) => (
              <InputNumber
                key={f.key}
                prefix={t(f.label)}
                suffix='tokens'
                min={0}
                value={previewTokens[f.key]}
                onChange={(v) =>
                  setPreviewTokens({ ...previewTokens, [f.key]: v })
                }
                style={{ width: 200 }}
              />
            ))}
            <Button loading={previewLoading} onClick={runPreview}>
              {t('计算')}
            </Button>
          </Space>
          <Typography.Text type='tertiary' size='small'>
            {t(
              'len 按输入 + 缓存读 + 缓存写自动计算；结果不含分组倍率与渠道倍率。',
            )}
          </Typography.Text>
          {preview &&
            (preview.ok ? (
              <Banner
                type='success'
                closeIcon={null}
                description={`${t('命中档位')}: ${preview.matched_tier || '-'}，${t('费用')}: $${Number(preview.cost).toFixed(6)}（${preview.quota} ${t('额度')}）`}
              />
            ) : (
              <Banner
                type='danger'
                closeIcon={null}
                description={preview.message}
              />
            ))}
        </Space>
      </Modal>
    </>
  );
}
