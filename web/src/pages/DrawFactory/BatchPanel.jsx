/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React, { useMemo, useState } from 'react';
import {
  Button,
  Card,
  Input,
  Space,
  TextArea,
  Toast,
  Tag,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import ModelSelector from '../../components/drawFactory/ModelSelector';
import SizePicker from '../../components/drawFactory/SizePicker';
import TokenSelector from '../../components/drawFactory/TokenSelector';
import PromptInput from '../../components/drawFactory/PromptInput';
import {
  useBatchQueue,
  BATCH_STATUS,
} from '../../hooks/drawFactory/useBatchQueue';

const TAG_COLOR = {
  [BATCH_STATUS.PENDING]: 'grey',
  [BATCH_STATUS.RUNNING]: 'blue',
  [BATCH_STATUS.DONE]: 'green',
  [BATCH_STATUS.FAILED]: 'red',
};

export default function BatchPanel({ models }) {
  const { t } = useTranslation();
  const batchModels = useMemo(
    () => models.filter((m) => m.batchEnabled !== false),
    [models],
  );
  const [modelKey, setModelKey] = useState(batchModels[0]?.key || null);
  const [token, setToken] = useState(null);
  const [prompt, setPrompt] = useState('');
  const [size, setSize] = useState(batchModels[0]?.defaultSize || '');
  const [refUrl, setRefUrl] = useState('');
  const [prodUrls, setProdUrls] = useState('');
  const {
    jobs,
    counts,
    isRunning,
    seed,
    clear,
    run,
    pause,
    cancel,
    retryFailed,
  } = useBatchQueue();

  const currentModel = useMemo(
    () => batchModels.find((m) => m.key === modelKey) || batchModels[0],
    [batchModels, modelKey],
  );

  function handleSeed() {
    const list = prodUrls
      .split('\n')
      .map((s) => s.trim())
      .filter(Boolean);
    if (list.length === 0) {
      Toast.warning('Add at least one product image URL');
      return;
    }
    seed(list.map((u) => ({ refUrl, prodUrl: u })));
  }

  async function handleStart() {
    if (!prompt.trim()) {
      Toast.warning(t('draw_factory.error.prompt_required'));
      return;
    }
    if (!token) {
      Toast.warning(t('draw_factory.empty.no_tokens'));
      return;
    }
    await run({ model: currentModel, token, prompt, size });
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card title={t('draw_factory.field.model')}>
        <Space vertical style={{ width: '100%' }}>
          <ModelSelector
            models={batchModels}
            value={modelKey}
            onChange={setModelKey}
          />
          <TokenSelector value={token} onChange={setToken} />
          {currentModel?.sizes && (
            <SizePicker
              sizes={currentModel.sizes}
              value={size}
              onChange={setSize}
            />
          )}
        </Space>
      </Card>

      <Card title={t('draw_factory.field.prompt')}>
        <PromptInput value={prompt} onChange={setPrompt} />
      </Card>

      <Card title={t('draw_factory.tab.batch')}>
        <Space vertical style={{ width: '100%' }}>
          <Input
            placeholder={t('draw_factory.batch.ref_url')}
            value={refUrl}
            onChange={setRefUrl}
          />
          <TextArea
            placeholder={t('draw_factory.batch.prod_urls')}
            value={prodUrls}
            onChange={setProdUrls}
            autosize={{ minRows: 3, maxRows: 8 }}
          />
          <Space>
            <Button onClick={handleSeed}>Load tasks</Button>
            <Button onClick={clear}>{t('draw_factory.batch.cancel')}</Button>
          </Space>
        </Space>
      </Card>

      <Card
        title={t('draw_factory.batch.summary', {
          done: counts.done,
          failed: counts.failed,
          pending: counts.pending,
        })}
      >
        <Space>
          {!isRunning && (
            <Button
              theme='solid'
              type='primary'
              onClick={handleStart}
              disabled={jobs.length === 0}
            >
              {counts.pending > 0 && counts.done + counts.failed > 0
                ? t('draw_factory.batch.resume')
                : t('draw_factory.batch.start')}
            </Button>
          )}
          {isRunning && (
            <Button onClick={pause}>{t('draw_factory.batch.pause')}</Button>
          )}
          <Button onClick={cancel}>{t('draw_factory.batch.cancel')}</Button>
          <Button
            onClick={retryFailed}
            disabled={counts.failed === 0 || isRunning}
          >
            {t('draw_factory.batch.retry_failed')}
          </Button>
        </Space>
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 8,
            marginTop: 16,
          }}
        >
          {jobs.map((j) => (
            <div
              key={j.id}
              style={{
                display: 'flex',
                gap: 12,
                alignItems: 'center',
                border: '1px solid var(--semi-color-border)',
                borderRadius: 8,
                padding: 8,
              }}
            >
              <Tag color={TAG_COLOR[j.status]}>
                {t(`draw_factory.status.${j.status}`)}
              </Tag>
              <div
                style={{
                  flex: 1,
                  fontSize: 12,
                  wordBreak: 'break-all',
                }}
              >
                {j.prodUrl}
              </div>
              {j.image && (
                <img
                  src={j.image}
                  alt='result'
                  style={{ width: 60, height: 60, objectFit: 'cover' }}
                />
              )}
              {j.error && (
                <div
                  style={{
                    color: 'var(--semi-color-danger)',
                    fontSize: 12,
                    maxWidth: 200,
                  }}
                >
                  {j.error}
                </div>
              )}
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}
