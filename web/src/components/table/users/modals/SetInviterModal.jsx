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

import React, { useEffect, useRef, useState } from 'react';
import {
  Modal,
  Select,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../../helpers';

const { Text } = Typography;

const SetInviterModal = ({ visible, user, onClose, refresh }) => {
  const { t } = useTranslation();
  const [options, setOptions] = useState([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [selected, setSelected] = useState(null); // selected user id, null = unbind
  const debounceRef = useRef(null);

  useEffect(() => {
    if (visible) {
      setSelected(null);
      setOptions([]);
    }
  }, [visible]);

  const doSearch = async (keyword) => {
    if (!keyword) {
      setOptions([]);
      return;
    }
    setSearchLoading(true);
    try {
      const res = await API.get(
        `/api/user/search?keyword=${encodeURIComponent(keyword)}`,
      );
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        setOptions([]);
        return;
      }
      const items = (data && data.items) || [];
      setOptions(
        items
          .filter((u) => u.id !== user?.id) // hide self
          .map((u) => ({
            value: u.id,
            label: `#${u.id} ${u.username}${
              u.display_name ? ` (${u.display_name})` : ''
            }${u.email ? ` — ${u.email}` : ''}`,
          })),
      );
    } catch (e) {
      showError(e.message);
    } finally {
      setSearchLoading(false);
    }
  };

  const handleSearch = (keyword) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => doSearch(keyword), 300);
  };

  const submit = async () => {
    const newId = selected || 0; // null/undefined → 0 (unbind)
    setSubmitting(true);
    try {
      const res = await API.post('/api/user/manage/inviter', {
        user_id: user.id,
        inviter_id: newId,
      });
      const { success, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      showSuccess(t('邀请人设置成功'));
      refresh && refresh();
      onClose();
    } catch (e) {
      showError(e.message);
    } finally {
      setSubmitting(false);
    }
  };

  const handleConfirm = () => {
    const newId = selected || 0;
    if (newId === (user?.inviter_id || 0)) {
      onClose();
      return;
    }
    if (user?.inviter_id && user.inviter_id !== 0) {
      Modal.warning({
        title: t('设置邀请人'),
        content: t('确认替换该用户的邀请人？此操作会写入审计日志，不可撤销。'),
        okText: t('确认'),
        cancelText: t('取消'),
        onOk: submit,
      });
    } else {
      submit();
    }
  };

  if (!user) return null;

  return (
    <Modal
      title={t('设置邀请人')}
      visible={visible}
      onCancel={onClose}
      onOk={handleConfirm}
      okText={t('确认')}
      cancelText={t('取消')}
      confirmLoading={submitting}
      maskClosable={false}
    >
      <Space vertical align='start' style={{ width: '100%' }} spacing={8}>
        <div>
          <Text type='tertiary'>{t('用户')}</Text>{' '}
          <Tag color='white' shape='circle'>
            #{user.id} {user.username}
            {user.display_name ? ` (${user.display_name})` : ''}
          </Tag>
        </div>
        <div>
          <Text type='tertiary'>{t('当前邀请人')}</Text>{' '}
          <Tag color='white' shape='circle'>
            {user.inviter_id ? `#${user.inviter_id}` : t('无邀请人')}
          </Tag>
        </div>
        <Select
          style={{ width: '100%' }}
          placeholder={t('搜索用户（用户名/邮箱/ID）')}
          filter
          remote
          showClear
          loading={searchLoading}
          onSearch={handleSearch}
          optionList={options}
          value={selected}
          onChange={(v) => setSelected(v)}
        />
        <Text size='small' type='tertiary'>
          {t('留空则解除当前邀请人绑定')}
        </Text>
      </Space>
    </Modal>
  );
};

export default SetInviterModal;
