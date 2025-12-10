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

import React, { useState, useEffect, useContext } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Form,
  Button,
  Switch,
  Input,
  Typography,
  Space,
} from '@douyinfe/semi-ui';
import { API, showSuccess, showError } from '../../../helpers';
import { StatusContext } from '../../../context/Status';

const { Text } = Typography;

export default function SettingsPlanCategories(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [statusState, statusDispatch] = useContext(StatusContext);

  // 套餐分类配置状态
  const [categoriesConfig, setCategoriesConfig] = useState({
    daily: { label: t('日卡'), enabled: true },
    weekly: { label: t('周卡'), enabled: true },
    biweekly: { label: t('双周卡'), enabled: true },
    monthly: { label: t('月卡'), enabled: true },
    payg: { label: t('按量付费'), enabled: true },
  });

  // 处理启用/禁用变更
  function handleEnabledChange(categoryKey) {
    return (checked) => {
      const newConfig = {
        ...categoriesConfig,
        [categoryKey]: {
          ...categoriesConfig[categoryKey],
          enabled: checked,
        },
      };
      setCategoriesConfig(newConfig);
    };
  }

  // 处理标签名称变更
  function handleLabelChange(categoryKey) {
    return (value) => {
      const newConfig = {
        ...categoriesConfig,
        [categoryKey]: {
          ...categoriesConfig[categoryKey],
          label: value,
        },
      };
      setCategoriesConfig(newConfig);
    };
  }

  // 重置为默认配置
  function resetCategories() {
    const defaultConfig = {
      daily: { label: t('日卡'), enabled: true },
      weekly: { label: t('周卡'), enabled: true },
      biweekly: { label: t('双周卡'), enabled: true },
      monthly: { label: t('月卡'), enabled: true },
      payg: { label: t('按量付费'), enabled: true },
    };
    setCategoriesConfig(defaultConfig);
    showSuccess(t('已重置为默认配置'));
  }

  // 保存配置
  async function onSubmit() {
    setLoading(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'PlanCategoriesConfig',
        value: JSON.stringify(categoriesConfig),
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('保存成功'));

        // 立即更新StatusContext中的状态
        statusDispatch({
          type: 'set',
          payload: {
            ...statusState.status,
            PlanCategoriesConfig: JSON.stringify(categoriesConfig),
          },
        });

        // 刷新父组件状态
        if (props.refresh) {
          await props.refresh();
        }
      } else {
        showError(message);
      }
    } catch (error) {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    // 从 props.options 中获取配置
    if (props.options && props.options.PlanCategoriesConfig) {
      try {
        const config = JSON.parse(props.options.PlanCategoriesConfig);
        // 确保所有分类都存在（向后兼容）
        const defaultConfig = {
          daily: { label: t('日卡'), enabled: true },
          weekly: { label: t('周卡'), enabled: true },
          biweekly: { label: t('双周卡'), enabled: true },
          monthly: { label: t('月卡'), enabled: true },
          payg: { label: t('按量付费'), enabled: true },
        };
        // 合并配置
        const mergedConfig = { ...defaultConfig };
        Object.keys(config).forEach((key) => {
          if (mergedConfig[key]) {
            mergedConfig[key] = {
              ...mergedConfig[key],
              ...config[key],
            };
          }
        });
        setCategoriesConfig(mergedConfig);
      } catch (error) {
        // 使用默认配置
        setCategoriesConfig({
          daily: { label: t('日卡'), enabled: true },
          weekly: { label: t('周卡'), enabled: true },
          biweekly: { label: t('双周卡'), enabled: true },
          monthly: { label: t('月卡'), enabled: true },
          payg: { label: t('按量付费'), enabled: true },
        });
      }
    }
  }, [props.options, t]);

  // 分类配置数据
  const categoryItems = [
    {
      key: 'daily',
      defaultLabel: t('日卡'),
      description: t('日卡套餐分类'),
    },
    {
      key: 'weekly',
      defaultLabel: t('周卡'),
      description: t('周卡套餐分类'),
    },
    {
      key: 'biweekly',
      defaultLabel: t('双周卡'),
      description: t('双周卡套餐分类'),
    },
    {
      key: 'monthly',
      defaultLabel: t('月卡'),
      description: t('月卡套餐分类'),
    },
    {
      key: 'payg',
      defaultLabel: t('按量付费'),
      description: t('按量付费套餐分类'),
    },
  ];

  return (
    <Card>
      <Form.Section
        text={t('套餐分类管理')}
        extraText={t('配置套餐分类的显示名称和启用状态')}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {categoryItems.map((item) => (
            <Card
              key={item.key}
              style={{
                borderRadius: '8px',
                border: '1px solid var(--semi-color-border)',
                background: 'var(--semi-color-bg-1)',
              }}
              bodyStyle={{ padding: '16px' }}
            >
              <Space
                style={{
                  width: '100%',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                }}
              >
                <div style={{ flex: 1 }}>
                  <div
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: '12px',
                      marginBottom: '8px',
                    }}
                  >
                    <Text
                      strong
                      style={{
                        fontSize: '14px',
                        color: 'var(--semi-color-text-0)',
                      }}
                    >
                      {item.defaultLabel}
                    </Text>
                    <Text
                      type='tertiary'
                      size='small'
                      style={{
                        fontSize: '12px',
                        color: 'var(--semi-color-text-2)',
                      }}
                    >
                      ({item.description})
                    </Text>
                  </div>
                  <Input
                    value={categoriesConfig[item.key]?.label || ''}
                    onChange={handleLabelChange(item.key)}
                    placeholder={t('输入分类显示名称')}
                    disabled={!categoriesConfig[item.key]?.enabled}
                    style={{ maxWidth: '300px' }}
                  />
                </div>
                <div>
                  <Switch
                    checked={categoriesConfig[item.key]?.enabled || false}
                    onChange={handleEnabledChange(item.key)}
                    size='default'
                  />
                </div>
              </Space>
            </Card>
          ))}
        </div>

        <div
          style={{
            display: 'flex',
            gap: '12px',
            justifyContent: 'flex-start',
            alignItems: 'center',
            paddingTop: '16px',
            marginTop: '16px',
            borderTop: '1px solid var(--semi-color-border)',
          }}
        >
          <Button
            size='default'
            type='tertiary'
            onClick={resetCategories}
            style={{
              borderRadius: '6px',
              fontWeight: '500',
            }}
          >
            {t('重置为默认')}
          </Button>
          <Button
            size='default'
            type='primary'
            onClick={onSubmit}
            loading={loading}
            style={{
              borderRadius: '6px',
              fontWeight: '500',
              minWidth: '100px',
            }}
          >
            {t('保存设置')}
          </Button>
        </div>
      </Form.Section>
    </Card>
  );
}
