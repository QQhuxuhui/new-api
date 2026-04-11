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
import { Button, Card, Space, Toast } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import ModelSelector from '../../components/drawFactory/ModelSelector';
import SizePicker from '../../components/drawFactory/SizePicker';
import TokenSelector from '../../components/drawFactory/TokenSelector';
import ReferenceImageUploader from '../../components/drawFactory/ReferenceImageUploader';
import PromptInput from '../../components/drawFactory/PromptInput';
import HistoryDrawer from './HistoryDrawer';
import { useSingleGeneration } from '../../hooks/drawFactory/useSingleGeneration';

export default function SinglePanel({ models }) {
  const { t } = useTranslation();
  const [modelKey, setModelKey] = useState(models[0]?.key || null);
  const [token, setToken] = useState(null);
  const [prompt, setPrompt] = useState('');
  const [size, setSize] = useState(models[0]?.defaultSize || '');
  const [refs, setRefs] = useState([]);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyTick, setHistoryTick] = useState(0);
  const { loading, image, error, elapsed, run, stop } = useSingleGeneration();

  const currentModel = useMemo(
    () => models.find((m) => m.key === modelKey) || models[0],
    [models, modelKey],
  );

  // When model changes: if current size isn't in its sizes[], fall back to default.
  useEffect(() => {
    if (!currentModel) return;
    if (!currentModel.sizes.includes(size)) {
      setSize(currentModel.defaultSize);
    }
    if (!currentModel.supportRefImage) {
      setRefs([]);
    }
  }, [currentModel]); // eslint-disable-line react-hooks/exhaustive-deps

  async function handleGenerate() {
    if (!prompt.trim()) {
      Toast.warning(t('draw_factory.error.prompt_required'));
      return;
    }
    if (!token) {
      Toast.warning(t('draw_factory.empty.no_tokens'));
      return;
    }
    await run({ model: currentModel, token, prompt, refs, size });
    setHistoryTick((x) => x + 1);
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Card title={t('draw_factory.field.model')}>
        <Space vertical style={{ width: '100%' }}>
          <ModelSelector
            models={models}
            value={modelKey}
            onChange={setModelKey}
          />
          <TokenSelector value={token} onChange={setToken} />
        </Space>
      </Card>

      <Card title={t('draw_factory.field.prompt')}>
        <PromptInput value={prompt} onChange={setPrompt} />
        {currentModel?.sizes && (
          <div style={{ marginTop: 12 }}>
            <SizePicker
              sizes={currentModel.sizes}
              value={size}
              onChange={setSize}
            />
          </div>
        )}
        {currentModel?.supportRefImage && (
          <div style={{ marginTop: 12 }}>
            <ReferenceImageUploader
              refs={refs}
              onChange={setRefs}
              max={currentModel.maxRefImages || 4}
            />
          </div>
        )}
      </Card>

      <Space>
        <Button
          theme='solid'
          type='primary'
          loading={loading}
          onClick={handleGenerate}
          disabled={!prompt.trim() || !currentModel || !token}
        >
          {t('draw_factory.action.generate')}
        </Button>
        {loading && (
          <Button onClick={stop}>{t('draw_factory.action.stop')}</Button>
        )}
        <Button onClick={() => setHistoryOpen(true)}>
          {t('draw_factory.action.history')}
        </Button>
      </Space>

      {error && (
        <Card>
          <div style={{ color: 'var(--semi-color-danger)' }}>{error}</div>
        </Card>
      )}
      {image && (
        <Card>
          <img
            src={image}
            alt='result'
            style={{ maxWidth: '100%', borderRadius: 8 }}
          />
          <div style={{ fontSize: 12, marginTop: 8 }}>{elapsed} ms</div>
          <Button
            onClick={() => {
              const a = document.createElement('a');
              a.href = image;
              a.download = `draw-factory-${Date.now()}.png`;
              a.click();
            }}
            style={{ marginTop: 8 }}
          >
            {t('draw_factory.action.download')}
          </Button>
        </Card>
      )}

      <HistoryDrawer
        visible={historyOpen}
        onClose={() => setHistoryOpen(false)}
        refreshKey={historyTick}
      />
    </div>
  );
}
