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

import React, { useEffect, useState, useRef } from 'react';
import { Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
} from '../../../helpers';
import { QUOTA_PER_UNIT } from '../../../utils/currency';

// 将 token 转换为 USD 显示
const tokenToUSD = (token) => {
  const num = parseFloat(token);
  if (!Number.isFinite(num) || num === 0) return '';
  return num / QUOTA_PER_UNIT;
};

// 将 USD 转换为 token 存储
const usdToToken = (usd) => {
  const num = parseFloat(usd);
  if (!Number.isFinite(num)) return '0';
  return String(Math.round(num * QUOTA_PER_UNIT));
};

export default function SettingsCreditLimit(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  // 存储原始 token 值（用于提交）
  const [inputs, setInputs] = useState({
    QuotaForNewUser: '',
    PreConsumedQuota: '',
    QuotaForInviter: '',
    QuotaForInvitee: '',
    'quota_setting.enable_free_model_pre_consume': true,
  });
  // 存储 USD 显示值（用于表单显示）
  const [usdInputs, setUsdInputs] = useState({
    QuotaForNewUser: '',
    PreConsumedQuota: '',
    QuotaForInviter: '',
    QuotaForInvitee: '',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);

  // 更新 USD 输入值，同时同步更新 token 值
  const handleUsdChange = (field, usdValue) => {
    setUsdInputs((prev) => ({ ...prev, [field]: usdValue }));
    setInputs((prev) => ({ ...prev, [field]: usdToToken(usdValue) }));
  };

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      let value = '';
      if (typeof inputs[item.key] === 'boolean') {
        value = String(inputs[item.key]);
      } else {
        value = inputs[item.key];
      }
      return API.put('/api/option/', {
        key: item.key,
        value,
      });
    });
    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (requestQueue.length === 1) {
          if (res.includes(undefined)) return;
        } else if (requestQueue.length > 1) {
          if (res.includes(undefined))
            return showError(t('部分保存失败，请重试'));
        }
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const currentInputs = {};
    const currentUsdInputs = {};
    const quotaFields = ['QuotaForNewUser', 'PreConsumedQuota', 'QuotaForInviter', 'QuotaForInvitee'];

    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
        // 对额度字段转换为 USD 显示
        if (quotaFields.includes(key)) {
          currentUsdInputs[key] = tokenToUSD(props.options[key]);
        }
      }
    }
    setInputs(currentInputs);
    setUsdInputs(currentUsdInputs);
    setInputsRow(structuredClone(currentInputs));
    // 设置表单显示 USD 值
    refForm.current.setValues({
      ...currentInputs,
      ...currentUsdInputs,
    });
  }, [props.options]);
  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('额度设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('新用户初始额度')}
                  field={'QuotaForNewUser'}
                  step={0.01}
                  min={0}
                  precision={2}
                  prefix={'$'}
                  placeholder={'0.00'}
                  onChange={(value) => handleUsdChange('QuotaForNewUser', value)}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('请求预扣费额度')}
                  field={'PreConsumedQuota'}
                  step={0.01}
                  min={0}
                  precision={2}
                  prefix={'$'}
                  extraText={t('请求结束后多退少补')}
                  placeholder={'0.00'}
                  onChange={(value) => handleUsdChange('PreConsumedQuota', value)}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('邀请新用户奖励额度')}
                  field={'QuotaForInviter'}
                  step={0.01}
                  min={0}
                  precision={2}
                  prefix={'$'}
                  extraText={''}
                  placeholder={t('例如：0.01')}
                  onChange={(value) => handleUsdChange('QuotaForInviter', value)}
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={6}>
                <Form.InputNumber
                  label={t('新用户使用邀请码奖励额度')}
                  field={'QuotaForInvitee'}
                  step={0.01}
                  min={0}
                  precision={2}
                  prefix={'$'}
                  extraText={''}
                  placeholder={t('例如：0.01')}
                  onChange={(value) => handleUsdChange('QuotaForInvitee', value)}
                />
              </Col>
            </Row>
            <Row>
              <Col>
                <Form.Switch
                  label={t('对免费模型启用预消耗')}
                  field={'quota_setting.enable_free_model_pre_consume'}
                  extraText={t(
                    '开启后，对免费模型（倍率为0，或者价格为0）的模型也会预消耗额度',
                  )}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      'quota_setting.enable_free_model_pre_consume': value,
                    })
                  }
                />
              </Col>
            </Row>

            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存额度设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
