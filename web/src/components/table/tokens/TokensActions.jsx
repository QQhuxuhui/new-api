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

import React, { useState } from 'react';
import { Button, Space } from '@douyinfe/semi-ui';
import { showError } from '../../../helpers';
import CopyTokensModal from './modals/CopyTokensModal';
import DeleteTokensModal from './modals/DeleteTokensModal';
import TokenCreateModeSelector from './modals/TokenCreateModeSelector';
import QuickCreateTokenModal from './modals/QuickCreateTokenModal';
import TokenCreatedSuccess from './modals/TokenCreatedSuccess';

const TokensActions = ({
  selectedKeys,
  setEditingToken,
  setShowEdit,
  batchCopyTokens,
  batchDeleteTokens,
  copyText,
  refresh,
  t,
}) => {
  // Modal states
  const [showCopyModal, setShowCopyModal] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [showModeSelector, setShowModeSelector] = useState(false);
  const [showQuickCreate, setShowQuickCreate] = useState(false);
  const [showSuccessModal, setShowSuccessModal] = useState(false);
  const [createdTokenData, setCreatedTokenData] = useState(null);

  // Handle copy selected tokens with options
  const handleCopySelectedTokens = () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个令牌！'));
      return;
    }
    setShowCopyModal(true);
  };

  // Handle delete selected tokens with confirmation
  const handleDeleteSelectedTokens = () => {
    if (selectedKeys.length === 0) {
      showError(t('请至少选择一个令牌！'));
      return;
    }
    setShowDeleteModal(true);
  };

  // Handle delete confirmation
  const handleConfirmDelete = () => {
    batchDeleteTokens();
    setShowDeleteModal(false);
  };

  // Handle add token button click
  const handleAddTokenClick = () => {
    setShowModeSelector(true);
  };

  // Handle mode selection
  const handleModeSelect = (mode) => {
    setShowModeSelector(false);
    if (mode === 'quick') {
      setShowQuickCreate(true);
    } else if (mode === 'advanced') {
      setEditingToken({ id: undefined });
      setShowEdit(true);
    }
  };

  // Handle quick create success
  const handleQuickCreateSuccess = (tokenData) => {
    setShowQuickCreate(false);
    setCreatedTokenData(tokenData);
    setShowSuccessModal(true);
  };

  // Handle switch from quick create to advanced mode
  const handleSwitchToAdvanced = () => {
    setShowQuickCreate(false);
    setEditingToken({ id: undefined });
    setShowEdit(true);
  };

  // Handle success modal close
  const handleSuccessClose = () => {
    setShowSuccessModal(false);
    setCreatedTokenData(null);
    if (refresh) {
      refresh();
    }
  };

  return (
    <>
      <div className='flex flex-wrap gap-2 w-full md:w-auto order-2 md:order-1'>
        <Button
          type='primary'
          className='flex-1 md:flex-initial'
          onClick={handleAddTokenClick}
          size='small'
        >
          {t('添加令牌')}
        </Button>

        <Button
          type='tertiary'
          className='flex-1 md:flex-initial'
          onClick={handleCopySelectedTokens}
          size='small'
        >
          {t('复制所选令牌')}
        </Button>

        <Button
          type='danger'
          className='w-full md:w-auto'
          onClick={handleDeleteSelectedTokens}
          size='small'
        >
          {t('删除所选令牌')}
        </Button>
      </div>

      <CopyTokensModal
        visible={showCopyModal}
        onCancel={() => setShowCopyModal(false)}
        selectedKeys={selectedKeys}
        copyText={copyText}
        t={t}
      />

      <DeleteTokensModal
        visible={showDeleteModal}
        onCancel={() => setShowDeleteModal(false)}
        onConfirm={handleConfirmDelete}
        selectedKeys={selectedKeys}
        t={t}
      />

      <TokenCreateModeSelector
        visible={showModeSelector}
        onSelect={handleModeSelect}
        onCancel={() => setShowModeSelector(false)}
        t={t}
      />

      <QuickCreateTokenModal
        visible={showQuickCreate}
        onSuccess={handleQuickCreateSuccess}
        onCancel={() => setShowQuickCreate(false)}
        onSwitchMode={handleSwitchToAdvanced}
        t={t}
      />

      <TokenCreatedSuccess
        visible={showSuccessModal}
        tokenData={createdTokenData}
        onClose={handleSuccessClose}
        t={t}
      />
    </>
  );
};

export default TokensActions;
