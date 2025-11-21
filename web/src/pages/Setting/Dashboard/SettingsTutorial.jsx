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
  Space,
  Table,
  Form,
  Typography,
  Empty,
  Divider,
  Modal,
  Switch,
  Tooltip,
  Tag,
  RadioGroup,
  Radio,
  InputNumber,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { Plus, Edit, Trash2, Save, HelpCircle, BookOpen } from 'lucide-react';
import { API, showError, showSuccess } from '../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

const SettingsTutorial = ({ options, refresh }) => {
  const { t } = useTranslation();

  const [tutorialList, setTutorialList] = useState([]);
  const [showTutorialModal, setShowTutorialModal] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [deletingTutorial, setDeletingTutorial] = useState(null);
  const [editingTutorial, setEditingTutorial] = useState(null);
  const [modalLoading, setModalLoading] = useState(false);
  const [loading, setLoading] = useState(false);
  const [hasChanges, setHasChanges] = useState(false);
  const [tutorialForm, setTutorialForm] = useState({
    id: '',
    title: '',
    order: 1,
    enabled: true,
    content: '',
    format: 'markdown',
  });
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [selectedRowKeys, setSelectedRowKeys] = useState([]);

  // 面板启用状态
  const [panelEnabled, setPanelEnabled] = useState(true);

  const columns = [
    {
      title: t('章节 ID'),
      dataIndex: 'id',
      key: 'id',
      width: 150,
      render: (text) => (
        <Tooltip content={text} showArrow>
          <div
            style={{
              maxWidth: '120px',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              fontFamily: 'monospace',
            }}
          >
            {text}
          </div>
        </Tooltip>
      ),
    },
    {
      title: t('标题'),
      dataIndex: 'title',
      key: 'title',
      render: (text) => (
        <Tooltip content={text} showArrow>
          <div
            style={{
              maxWidth: '250px',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              fontWeight: 'bold',
            }}
          >
            {text}
          </div>
        </Tooltip>
      ),
    },
    {
      title: t('顺序'),
      dataIndex: 'order',
      key: 'order',
      width: 80,
      render: (text) => (
        <Tag color='blue' size='small'>
          {text}
        </Tag>
      ),
    },
    {
      title: t('格式'),
      dataIndex: 'format',
      key: 'format',
      width: 100,
      render: (text) => (
        <Tag color={text === 'markdown' ? 'green' : 'orange'} size='small'>
          {text === 'markdown' ? 'Markdown' : 'HTML'}
        </Tag>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'enabled',
      key: 'enabled',
      width: 80,
      render: (enabled) => (
        <Tag color={enabled ? 'green' : 'grey'} size='small'>
          {enabled ? t('启用') : t('禁用')}
        </Tag>
      ),
    },
    {
      title: t('操作'),
      key: 'action',
      fixed: 'right',
      width: 150,
      render: (text, record) => (
        <Space>
          <Button
            icon={<Edit size={14} />}
            theme='light'
            type='tertiary'
            size='small'
            onClick={() => handleEditTutorial(record)}
          >
            {t('编辑')}
          </Button>
          <Button
            icon={<Trash2 size={14} />}
            type='danger'
            theme='light'
            size='small'
            onClick={() => handleDeleteTutorial(record)}
          >
            {t('删除')}
          </Button>
        </Space>
      ),
    },
  ];

  const updateOption = async (key, value) => {
    const res = await API.put('/api/option/', {
      key,
      value,
    });
    const { success, message } = res.data;
    if (success) {
      showSuccess('教程内容已更新');
      if (refresh) refresh();
    } else {
      showError(message);
    }
  };

  const submitTutorial = async () => {
    try {
      setLoading(true);
      const tutorialJson = JSON.stringify({ sections: tutorialList });
      await updateOption('console_setting.tutorial', tutorialJson);
      setHasChanges(false);
    } catch (error) {
      console.error('教程内容更新失败', error);
      showError('教程内容更新失败');
    } finally {
      setLoading(false);
    }
  };

  const handleAddTutorial = () => {
    setEditingTutorial(null);
    setTutorialForm({
      id: '',
      title: '',
      order: tutorialList.length + 1,
      enabled: true,
      content: '',
      format: 'markdown',
    });
    setShowTutorialModal(true);
  };

  const handleEditTutorial = (tutorial) => {
    setEditingTutorial(tutorial);
    setTutorialForm({
      id: tutorial.id,
      title: tutorial.title,
      order: tutorial.order,
      enabled: tutorial.enabled,
      content: tutorial.content,
      format: tutorial.format,
    });
    setShowTutorialModal(true);
  };

  const handleDeleteTutorial = (tutorial) => {
    setDeletingTutorial(tutorial);
    setShowDeleteModal(true);
  };

  const confirmDeleteTutorial = () => {
    if (deletingTutorial) {
      const newList = tutorialList.filter(
        (item) => item.id !== deletingTutorial.id,
      );
      setTutorialList(newList);
      setHasChanges(true);
      showSuccess('教程章节已删除，请及时点击"保存设置"进行保存');
    }
    setShowDeleteModal(false);
    setDeletingTutorial(null);
  };

  const validateTutorialId = (id) => {
    const idPattern = /^[a-z0-9-]+$/;
    return idPattern.test(id);
  };

  const handleSaveTutorial = async () => {
    if (!tutorialForm.id || !tutorialForm.title || !tutorialForm.content) {
      showError('请填写完整的教程信息');
      return;
    }

    if (!validateTutorialId(tutorialForm.id)) {
      showError('章节 ID 只能包含小写字母、数字和连字符');
      return;
    }

    // 检查 ID 重复（编辑时排除自己）
    const idExists = tutorialList.some(
      (item) =>
        item.id === tutorialForm.id &&
        (!editingTutorial || item.id !== editingTutorial.id),
    );
    if (idExists) {
      showError('章节 ID 已存在，请使用其他 ID');
      return;
    }

    try {
      setModalLoading(true);

      let newList;
      if (editingTutorial) {
        newList = tutorialList.map((item) =>
          item.id === editingTutorial.id ? { ...tutorialForm } : item,
        );
      } else {
        const newTutorial = { ...tutorialForm };
        newList = [...tutorialList, newTutorial];
      }

      setTutorialList(newList);
      setHasChanges(true);
      setShowTutorialModal(false);
      showSuccess(
        editingTutorial
          ? '教程章节已更新，请及时点击"保存设置"进行保存'
          : '教程章节已添加，请及时点击"保存设置"进行保存',
      );
    } catch (error) {
      showError('操作失败: ' + error.message);
    } finally {
      setModalLoading(false);
    }
  };

  const parseTutorial = (tutorialStr) => {
    if (!tutorialStr) {
      setTutorialList([]);
      return;
    }

    try {
      const parsed = JSON.parse(tutorialStr);
      const sections = parsed.sections || [];
      setTutorialList(sections);
    } catch (error) {
      console.error('解析教程内容失败:', error);
      setTutorialList([]);
    }
  };

  useEffect(() => {
    if (options['console_setting.tutorial'] !== undefined) {
      parseTutorial(options['console_setting.tutorial']);
    }
  }, [options['console_setting.tutorial']]);

  useEffect(() => {
    const enabledStr = options['console_setting.tutorial_enabled'];
    setPanelEnabled(
      enabledStr === undefined
        ? true
        : enabledStr === 'true' || enabledStr === true,
    );
  }, [options['console_setting.tutorial_enabled']]);

  const handleToggleEnabled = async (checked) => {
    const newValue = checked ? 'true' : 'false';
    try {
      const res = await API.put('/api/option/', {
        key: 'console_setting.tutorial_enabled',
        value: newValue,
      });
      if (res.data.success) {
        setPanelEnabled(checked);
        showSuccess(t('设置已保存'));
        refresh?.();
      } else {
        showError(res.data.message);
      }
    } catch (err) {
      showError(err.message);
    }
  };

  const handleBatchDelete = () => {
    if (selectedRowKeys.length === 0) {
      showError('请先选择要删除的教程章节');
      return;
    }

    const newList = tutorialList.filter(
      (item) => !selectedRowKeys.includes(item.id),
    );
    setTutorialList(newList);
    setSelectedRowKeys([]);
    setHasChanges(true);
    showSuccess(
      `已删除 ${selectedRowKeys.length} 个教程章节，请及时点击"保存设置"进行保存`,
    );
  };

  const renderHeader = () => (
    <div className='flex flex-col w-full'>
      <div className='mb-2'>
        <div className='flex items-center text-blue-500'>
          <BookOpen size={16} className='mr-2' />
          <Text>
            {t(
              '教程内容管理，为用户提供详细的配置教程（最多 20 个章节，支持动态变量 {{BASE_URL}}, {{CLAUDE_API_URL}}, {{OPENAI_API_URL}}）',
            )}
          </Text>
        </div>
      </div>

      <Divider margin='12px' />

      <div className='flex flex-col md:flex-row justify-between items-center gap-4 w-full'>
        <div className='flex gap-2 w-full md:w-auto order-2 md:order-1'>
          <Button
            theme='light'
            type='primary'
            icon={<Plus size={14} />}
            className='w-full md:w-auto'
            onClick={handleAddTutorial}
          >
            {t('添加章节')}
          </Button>
          <Button
            icon={<Trash2 size={14} />}
            type='danger'
            theme='light'
            onClick={handleBatchDelete}
            disabled={selectedRowKeys.length === 0}
            className='w-full md:w-auto'
          >
            {t('批量删除')}{' '}
            {selectedRowKeys.length > 0 && `(${selectedRowKeys.length})`}
          </Button>
          <Button
            icon={<Save size={14} />}
            onClick={submitTutorial}
            loading={loading}
            disabled={!hasChanges}
            type='secondary'
            className='w-full md:w-auto'
          >
            {t('保存设置')}
          </Button>
        </div>

        {/* 启用开关 */}
        <div className='order-1 md:order-2 flex items-center gap-2'>
          <Switch checked={panelEnabled} onChange={handleToggleEnabled} />
          <Text>{panelEnabled ? t('已启用') : t('已禁用')}</Text>
        </div>
      </div>
    </div>
  );

  // 计算当前页显示的数据
  const getCurrentPageData = () => {
    const startIndex = (currentPage - 1) * pageSize;
    const endIndex = startIndex + pageSize;
    // 按 order 字段排序
    const sortedList = [...tutorialList].sort((a, b) => a.order - b.order);
    return sortedList.slice(startIndex, endIndex);
  };

  const rowSelection = {
    selectedRowKeys,
    onChange: (selectedRowKeys, selectedRows) => {
      setSelectedRowKeys(selectedRowKeys);
    },
    onSelect: (record, selected, selectedRows) => {
      console.log(`选择行: ${selected}`, record);
    },
    onSelectAll: (selected, selectedRows) => {
      console.log(`全选: ${selected}`, selectedRows);
    },
    getCheckboxProps: (record) => ({
      disabled: false,
      name: record.id,
    }),
  };

  return (
    <>
      <Form.Section text={renderHeader()}>
        <Table
          columns={columns}
          dataSource={getCurrentPageData()}
          rowSelection={rowSelection}
          rowKey='id'
          scroll={{ x: 'max-content' }}
          pagination={{
            currentPage: currentPage,
            pageSize: pageSize,
            total: tutorialList.length,
            showSizeChanger: true,
            showQuickJumper: true,
            pageSizeOptions: ['5', '10', '20', '50'],
            onChange: (page, size) => {
              setCurrentPage(page);
              setPageSize(size);
            },
            onShowSizeChange: (current, size) => {
              setCurrentPage(1);
              setPageSize(size);
            },
          }}
          size='middle'
          loading={loading}
          empty={
            <Empty
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              description={t('暂无教程章节')}
              style={{ padding: 30 }}
            />
          }
          className='overflow-hidden'
        />
      </Form.Section>

      <Modal
        title={editingTutorial ? t('编辑章节') : t('添加章节')}
        visible={showTutorialModal}
        onOk={handleSaveTutorial}
        onCancel={() => setShowTutorialModal(false)}
        okText={t('保存')}
        cancelText={t('取消')}
        confirmLoading={modalLoading}
        width={900}
        style={{ maxHeight: '90vh' }}
        bodyStyle={{ maxHeight: '70vh', overflowY: 'auto' }}
      >
        <Form
          layout='vertical'
          initValues={tutorialForm}
          key={editingTutorial ? editingTutorial.id : 'new'}
        >
          <Form.Input
            field='id'
            label={t('章节 ID')}
            placeholder={t('例如: claude-code 或 openai-codex')}
            maxLength={50}
            rules={[
              { required: true, message: t('请输入章节 ID') },
              {
                pattern: /^[a-z0-9-]+$/,
                message: t('只能包含小写字母、数字和连字符'),
              },
            ]}
            onChange={(value) => setTutorialForm({ ...tutorialForm, id: value })}
            disabled={!!editingTutorial}
            helpText={t('章节 ID 用于唯一标识，创建后不可修改')}
          />
          <Form.Input
            field='title'
            label={t('章节标题')}
            placeholder={t('请输入章节标题')}
            maxLength={100}
            rules={[{ required: true, message: t('请输入章节标题') }]}
            onChange={(value) =>
              setTutorialForm({ ...tutorialForm, title: value })
            }
          />
          <div style={{ display: 'flex', gap: '16px' }}>
            <div style={{ flex: 1 }}>
              <Form.Label>
                {t('显示顺序')}
                <span style={{ color: 'var(--semi-color-danger)' }}>*</span>
              </Form.Label>
              <InputNumber
                value={tutorialForm.order}
                min={0}
                onChange={(value) =>
                  setTutorialForm({ ...tutorialForm, order: value })
                }
                style={{ width: '100%' }}
              />
            </div>
            <div style={{ flex: 1 }}>
              <Form.Label>{t('是否启用')}</Form.Label>
              <br />
              <Switch
                checked={tutorialForm.enabled}
                onChange={(checked) =>
                  setTutorialForm({ ...tutorialForm, enabled: checked })
                }
              />
            </div>
          </div>
          <Form.Label style={{ marginTop: '16px' }}>
            {t('内容格式')}
            <span style={{ color: 'var(--semi-color-danger)' }}>*</span>
          </Form.Label>
          <RadioGroup
            type='button'
            value={tutorialForm.format}
            onChange={(e) =>
              setTutorialForm({ ...tutorialForm, format: e.target.value })
            }
            buttonSize='middle'
          >
            <Radio value='markdown'>Markdown</Radio>
            <Radio value='html'>HTML</Radio>
          </RadioGroup>
          <Form.TextArea
            field='content'
            label={t('章节内容')}
            placeholder={t(
              '请输入章节内容（支持 Markdown 或 HTML，可使用动态变量 {{BASE_URL}}, {{CLAUDE_API_URL}}, {{OPENAI_API_URL}}）',
            )}
            maxCount={50000}
            rows={12}
            rules={[{ required: true, message: t('请输入章节内容') }]}
            onChange={(value) =>
              setTutorialForm({ ...tutorialForm, content: value })
            }
            helpText={t('动态变量会在显示时自动替换为实际的网站地址')}
          />
        </Form>
      </Modal>

      <Modal
        title={t('确认删除')}
        visible={showDeleteModal}
        onOk={confirmDeleteTutorial}
        onCancel={() => {
          setShowDeleteModal(false);
          setDeletingTutorial(null);
        }}
        okText={t('确认删除')}
        cancelText={t('取消')}
        type='warning'
        okButtonProps={{
          type: 'danger',
          theme: 'solid',
        }}
      >
        <Text>{t('确定要删除此教程章节吗？')}</Text>
      </Modal>
    </>
  );
};

export default SettingsTutorial;
