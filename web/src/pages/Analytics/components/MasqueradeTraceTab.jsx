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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Card, Table, Button, Popconfirm, Tag, Empty, Typography } from '@douyinfe/semi-ui';
import { IconRefresh, IconDelete } from '@douyinfe/semi-icons';
import { MasqueradeAPI } from '../../../services/masqueradeApi';
import { timestamp2string } from '../../../helpers';
import MasqueradeDetailModal from '../../../components/dashboard/MasqueradeDetailModal';

const { Text } = Typography;

/**
 * MasqueradeTraceTab - Masquerade trace panel for Analytics page
 * Uses lightweight list API for better performance
 */
const MasqueradeTraceTab = ({ t }) => {
  const [loading, setLoading] = useState(false);
  const [traces, setTraces] = useState([]);
  const [selectedRecordId, setSelectedRecordId] = useState(null);
  const [modalVisible, setModalVisible] = useState(false);

  // Load lightweight trace list
  const loadTraces = useCallback(async () => {
    setLoading(true);
    try {
      const data = await MasqueradeAPI.fetchTraceList();
      setTraces(Array.isArray(data) ? data : []);
    } catch (e) {
      // Error toast handled in service
    } finally {
      setLoading(false);
    }
  }, []);

  const handleClear = useCallback(async () => {
    try {
      await MasqueradeAPI.clearTraces();
      setTraces([]);
    } catch (e) {
      // Error toast handled in service
    }
  }, []);

  // Open detail modal with record ID (lazy loading)
  const handleViewDetail = useCallback((record) => {
    setSelectedRecordId(record.id);
    setModalVisible(true);
  }, []);

  const handleCloseModal = useCallback(() => {
    setModalVisible(false);
    setSelectedRecordId(null);
  }, []);

  useEffect(() => {
    loadTraces();
  }, [loadTraces]);

  const columns = useMemo(
    () => [
      {
        title: t('时间'),
        dataIndex: 'timestamp',
        width: 170,
        render: (timestamp) => (timestamp ? timestamp2string(timestamp) : '-'),
      },
      {
        title: t('模型'),
        dataIndex: 'model',
        width: 220,
        render: (model) => (
          <Tag color='blue' size='small'>
            {model || '-'}
          </Tag>
        ),
      },
      {
        title: t('渠道'),
        dataIndex: 'channel_name',
        width: 140,
        render: (name) => name || '-',
      },
      {
        title: t('原始用户ID'),
        dataIndex: 'original_user_id',
        width: 220,
        render: (id) => (
          <span className='text-xs truncate block max-w-[210px]' title={id}>
            {id === '<empty>' ? <Tag color='grey'>{t('空')}</Tag> : id || '-'}
          </span>
        ),
      },
      {
        title: t('伪装用户ID'),
        dataIndex: 'masked_user_id',
        width: 220,
        render: (id) => (
          <span className='text-xs truncate block max-w-[210px]' title={id}>
            {id ? `${id.substring(0, 24)}...` : '-'}
          </span>
        ),
      },
      {
        title: t('操作'),
        width: 90,
        render: (_, record) => (
          <Button size='small' theme='light' onClick={() => handleViewDetail(record)}>
            {t('详情')}
          </Button>
        ),
      },
    ],
    [handleViewDetail, t],
  );

  return (
    <>
      <Card
        className='shadow-sm !rounded-2xl'
        title={
          <div className='flex items-center justify-between w-full'>
            <span>
              {t('伪装追踪')} ({traces.length}/100)
            </span>
            <div className='flex gap-2'>
              <Button
                icon={<IconRefresh />}
                size='small'
                loading={loading}
                onClick={loadTraces}
              >
                {t('刷新')}
              </Button>
              <Popconfirm
                title={t('确定要清空所有追踪记录吗？')}
                onConfirm={handleClear}
              >
                <Button icon={<IconDelete />} size='small' type='danger'>
                  {t('清空')}
                </Button>
              </Popconfirm>
            </div>
          </div>
        }
        bodyStyle={{ padding: 0 }}
      >
        {traces.length === 0 ? (
          <div className='flex justify-center items-center py-10'>
            <Empty
              title={t('暂无追踪记录')}
              description={t('发送 Claude 请求后将显示最近 100 条伪装对比记录')}
            />
          </div>
        ) : (
          <Table
            dataSource={traces}
            columns={columns}
            loading={loading}
            rowKey={(record) =>
              record.id || `${record.timestamp}-${record.channel_id}-${record.model}`
            }
            pagination={{ pageSize: 10 }}
          />
        )}
      </Card>

      <MasqueradeDetailModal
        visible={modalVisible}
        recordId={selectedRecordId}
        onClose={handleCloseModal}
        t={t}
      />
    </>
  );
};

export default MasqueradeTraceTab;
