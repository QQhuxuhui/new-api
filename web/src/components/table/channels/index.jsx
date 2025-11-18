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

import React, { useState, useEffect } from 'react';
import CardPro from '../../common/ui/CardPro';
import ChannelsTable from './ChannelsTable';
import ChannelsActions from './ChannelsActions';
import ChannelsFilters from './ChannelsFilters';
import ChannelsTabs from './ChannelsTabs';
import { useChannelsData } from '../../../hooks/channels/useChannelsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import BatchTagModal from './modals/BatchTagModal';
import ModelTestModal from './modals/ModelTestModal';
import ColumnSelectorModal from './modals/ColumnSelectorModal';
import EditChannelModal from './modals/EditChannelModal';
import EditTagModal from './modals/EditTagModal';
import MultiKeyManageModal from './modals/MultiKeyManageModal';
import ChannelHealthModal from './ChannelHealthModal';
import { createCardProPagination } from '../../../helpers/utils';
import { API } from '../../../helpers';

const ChannelsPage = () => {
  const channelsData = useChannelsData();
  const isMobile = useIsMobile();

  // Health status states
  const [healthInfo, setHealthInfo] = useState({});
  const [showHealthModal, setShowHealthModal] = useState(false);
  const [currentHealthChannel, setCurrentHealthChannel] = useState(null);

  // Fetch health status for all channels
  const fetchHealthInfo = async () => {
    try {
      const res = await API.get('/api/channel/health');
      if (res.data.success && res.data.data) {
        const healthMap = {};
        res.data.data.forEach((health) => {
          healthMap[health.channel_id] = health;
        });
        setHealthInfo(healthMap);
      }
    } catch (error) {
      // Silently handle error - health status is optional
      console.error('Failed to fetch health info:', error);
    }
  };

  // Fetch health info on mount and refresh
  useEffect(() => {
    fetchHealthInfo();
    // Auto-refresh every 30 seconds
    const interval = setInterval(fetchHealthInfo, 30000);
    return () => clearInterval(interval);
  }, []);

  // Refresh health info when channels refresh
  useEffect(() => {
    fetchHealthInfo();
  }, [channelsData.channels]);

  return (
    <>
      {/* Modals */}
      <ColumnSelectorModal {...channelsData} />
      <EditTagModal
        visible={channelsData.showEditTag}
        tag={channelsData.editingTag}
        handleClose={() => channelsData.setShowEditTag(false)}
        refresh={channelsData.refresh}
      />
      <EditChannelModal
        refresh={channelsData.refresh}
        visible={channelsData.showEdit}
        handleClose={channelsData.closeEdit}
        editingChannel={channelsData.editingChannel}
      />
      <BatchTagModal {...channelsData} />
      <ModelTestModal {...channelsData} />
      <MultiKeyManageModal
        visible={channelsData.showMultiKeyManageModal}
        onCancel={() => channelsData.setShowMultiKeyManageModal(false)}
        channel={channelsData.currentMultiKeyChannel}
        onRefresh={channelsData.refresh}
      />
      <ChannelHealthModal
        visible={showHealthModal}
        health={currentHealthChannel ? healthInfo[currentHealthChannel] : null}
        channelId={currentHealthChannel}
        onClose={() => {
          setShowHealthModal(false);
          setCurrentHealthChannel(null);
        }}
        onHealthReset={() => {
          fetchHealthInfo();
          channelsData.refresh();
        }}
      />

      {/* Main Content */}
      <CardPro
        type='type3'
        tabsArea={<ChannelsTabs {...channelsData} />}
        actionsArea={<ChannelsActions {...channelsData} />}
        searchArea={<ChannelsFilters {...channelsData} />}
        paginationArea={createCardProPagination({
          currentPage: channelsData.activePage,
          pageSize: channelsData.pageSize,
          total: channelsData.channelCount,
          onPageChange: channelsData.handlePageChange,
          onPageSizeChange: channelsData.handlePageSizeChange,
          isMobile: isMobile,
          t: channelsData.t,
        })}
        t={channelsData.t}
      >
        <ChannelsTable
          {...channelsData}
          healthInfo={healthInfo}
          setShowHealthModal={setShowHealthModal}
          setCurrentHealthChannel={setCurrentHealthChannel}
        />
      </CardPro>
    </>
  );
};

export default ChannelsPage;
