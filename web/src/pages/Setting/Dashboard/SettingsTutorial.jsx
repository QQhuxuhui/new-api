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

import React, { useEffect, useState } from 'react';
import {
  Button,
  Form,
  Typography,
  Divider,
  Switch,
  RadioGroup,
  Radio,
  Modal,
  Collapsible,
} from '@douyinfe/semi-ui';
import { Save, BookOpen, Eye, HelpCircle, Copy, Check } from 'lucide-react';
import { API, showError, showSuccess } from '../../../helpers';
import { useTranslation } from 'react-i18next';
import DOMPurify from 'dompurify';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

const { Text } = Typography;

// Variable replacement utility
const replaceVariables = (content) => {
  if (!content) return '';
  const baseUrl = window.location.origin;
  const variables = {
    '{{baseUrl}}': baseUrl,
    '{{claudeApiUrl}}': baseUrl,
    '{{openaiApiUrl}}': `${baseUrl}/v1`,
    '{{apiUrl}}': `${baseUrl}/v1`,
  };
  let result = content;
  Object.entries(variables).forEach(([key, value]) => {
    result = result.replace(new RegExp(key.replace(/[{}]/g, '\\$&'), 'g'), value);
  });
  return result;
};

const SettingsTutorial = ({ options, refresh }) => {
  const { t } = useTranslation();

  const [content, setContent] = useState('');
  const [format, setFormat] = useState('markdown');
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(false);
  const [hasChanges, setHasChanges] = useState(false);
  const [showPreview, setShowPreview] = useState(false);
  const [copiedVar, setCopiedVar] = useState('');

  const updateOption = async (key, value) => {
    const res = await API.put('/api/option/', { key, value });
    const { success, message } = res.data;
    if (!success) {
      showError(message);
      return false;
    }
    return true;
  };

  const handleSave = async () => {
    try {
      setLoading(true);
      const results = await Promise.all([
        updateOption('console_setting.tutorial_content', content),
        updateOption('console_setting.tutorial_format', format),
      ]);
      if (results.every(Boolean)) {
        showSuccess(t('教程内容已保存'));
        setHasChanges(false);
        refresh?.();
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  const handleToggleEnabled = async (checked) => {
    try {
      const success = await updateOption(
        'console_setting.tutorial_enabled',
        checked ? 'true' : 'false'
      );
      if (success) {
        setEnabled(checked);
        showSuccess(t('设置已保存'));
        refresh?.();
      }
    } catch (err) {
      showError(err.message);
    }
  };

  const handleCopyVariable = async (variable) => {
    try {
      await navigator.clipboard.writeText(variable);
      setCopiedVar(variable);
      setTimeout(() => setCopiedVar(''), 2000);
    } catch (err) {
      showError('复制失败');
    }
  };

  useEffect(() => {
    const tutorialContent = options['console_setting.tutorial_content'];
    const tutorialFormat = options['console_setting.tutorial_format'];
    const tutorialEnabled = options['console_setting.tutorial_enabled'];

    if (tutorialContent !== undefined) setContent(tutorialContent);
    if (tutorialFormat) setFormat(tutorialFormat);
    setEnabled(
      tutorialEnabled === undefined
        ? false
        : tutorialEnabled === 'true' || tutorialEnabled === true
    );
  }, [
    options['console_setting.tutorial_content'],
    options['console_setting.tutorial_format'],
    options['console_setting.tutorial_enabled'],
  ]);

  const variables = [
    { name: '{{baseUrl}}', desc: t('站点基础 URL'), example: window.location.origin },
    { name: '{{claudeApiUrl}}', desc: t('Claude API 端点'), example: window.location.origin },
    { name: '{{openaiApiUrl}}', desc: t('OpenAI API 端点'), example: `${window.location.origin}/v1` },
    { name: '{{apiUrl}}', desc: t('API 端点 (同 openaiApiUrl)'), example: `${window.location.origin}/v1` },
  ];

  const renderPreviewContent = () => {
    const processedContent = replaceVariables(content);
    if (format === 'markdown') {
      return (
        <div className="prose dark:prose-invert max-w-none">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>
            {processedContent}
          </ReactMarkdown>
        </div>
      );
    }
    return (
      <div
        dangerouslySetInnerHTML={{
          __html: DOMPurify.sanitize(processedContent),
        }}
      />
    );
  };

  const renderHeader = () => (
    <div className="flex flex-col w-full">
      <div className="mb-2">
        <div className="flex items-center text-blue-500">
          <BookOpen size={16} className="mr-2" />
          <Text>
            {t('教程内容管理，支持 Markdown 或 HTML 格式，可使用动态变量')}
          </Text>
        </div>
      </div>

      <Divider margin="12px" />

      <div className="flex flex-col md:flex-row justify-between items-center gap-4 w-full">
        <div className="flex gap-2 w-full md:w-auto order-2 md:order-1">
          <Button
            icon={<Eye size={14} />}
            theme="light"
            type="tertiary"
            onClick={() => setShowPreview(true)}
            disabled={!content}
          >
            {t('预览')}
          </Button>
          <Button
            icon={<Save size={14} />}
            onClick={handleSave}
            loading={loading}
            disabled={!hasChanges}
            type="secondary"
          >
            {t('保存设置')}
          </Button>
        </div>

        <div className="order-1 md:order-2 flex items-center gap-2">
          <Switch checked={enabled} onChange={handleToggleEnabled} />
          <Text>{enabled ? t('已启用') : t('已禁用')}</Text>
        </div>
      </div>
    </div>
  );

  return (
    <>
      <Form.Section text={renderHeader()}>
        <div className="space-y-4">
          {/* Format selector */}
          <div>
            <Text strong className="block mb-2">{t('内容格式')}</Text>
            <RadioGroup
              value={format}
              onChange={(e) => {
                setFormat(e.target.value);
                setHasChanges(true);
              }}
              direction="horizontal"
            >
              <Radio value="markdown">Markdown</Radio>
              <Radio value="html">HTML</Radio>
            </RadioGroup>
          </div>

          {/* Content editor */}
          <div>
            <Text strong className="block mb-2">{t('教程内容')}</Text>
            <Form.TextArea
              field="content"
              noLabel
              placeholder={t('请输入教程内容，支持 {{baseUrl}} 等动态变量')}
              value={content}
              onChange={(value) => {
                setContent(value);
                setHasChanges(true);
              }}
              rows={15}
              style={{ fontFamily: 'monospace' }}
            />
          </div>

          {/* Variable help */}
          <Collapsible>
            <Collapsible.Panel
              header={
                <div className="flex items-center gap-2">
                  <HelpCircle size={16} />
                  <Text>{t('可用变量说明')}</Text>
                </div>
              }
              itemKey="variables"
            >
              <div className="p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b dark:border-gray-700">
                      <th className="text-left py-2">{t('变量')}</th>
                      <th className="text-left py-2">{t('说明')}</th>
                      <th className="text-left py-2">{t('当前值')}</th>
                      <th className="py-2"></th>
                    </tr>
                  </thead>
                  <tbody>
                    {variables.map((v) => (
                      <tr key={v.name} className="border-b dark:border-gray-700 last:border-0">
                        <td className="py-2">
                          <code className="bg-gray-200 dark:bg-gray-700 px-2 py-0.5 rounded">
                            {v.name}
                          </code>
                        </td>
                        <td className="py-2">{v.desc}</td>
                        <td className="py-2 text-gray-500">{v.example}</td>
                        <td className="py-2">
                          <Button
                            size="small"
                            theme="borderless"
                            icon={copiedVar === v.name ? <Check size={14} /> : <Copy size={14} />}
                            onClick={() => handleCopyVariable(v.name)}
                          />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Collapsible.Panel>
          </Collapsible>
        </div>
      </Form.Section>

      {/* Preview Modal */}
      <Modal
        title={t('教程内容预览')}
        visible={showPreview}
        onCancel={() => setShowPreview(false)}
        footer={null}
        width={800}
        style={{ maxHeight: '80vh' }}
        bodyStyle={{ maxHeight: '70vh', overflow: 'auto' }}
      >
        {renderPreviewContent()}
      </Modal>
    </>
  );
};

export default SettingsTutorial;
