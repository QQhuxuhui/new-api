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

import React from 'react';
import { Button, DatePicker, Select } from '@douyinfe/semi-ui';
import { QUICK_DATE_PRESETS, TIME_OPTIONS } from '../../constants/dashboard.constants';
import { selectFilter } from '../../helpers';

const QuickFilterBar = ({
  activePreset,
  inputs,
  dataExportDefaultTime,
  isAdminUser,
  isMobile,
  onPresetChange,
  onDateRangeChange,
  onGranularityChange,
  onUsernameChange,
  onUsernameSearch,
  usernameOptions,
  usernameSearchLoading,
  t,
}) => {
  const { start_timestamp, end_timestamp, username } = inputs;

  const translatedPresets = QUICK_DATE_PRESETS.map((preset) => ({
    ...preset,
    label: t(preset.label),
  }));

  const translatedTimeOptions = TIME_OPTIONS.map((option) => ({
    ...option,
    label: t(option.label),
  }));

  const handleStartDateChange = (value) => {
    onDateRangeChange(value, end_timestamp);
  };

  const handleEndDateChange = (value) => {
    onDateRangeChange(start_timestamp, value);
  };

  if (isMobile) {
    return (
      <div className="mb-4 p-4 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700">
        {/* 第一行: 快捷按钮 (可横向滚动) */}
        <div className="overflow-x-auto pb-2 -mx-1">
          <div className="flex gap-2 px-1 min-w-max">
            {translatedPresets.map((preset) => (
              <Button
                key={preset.key}
                type={activePreset === preset.key ? 'primary' : 'tertiary'}
                size="small"
                onClick={() => onPresetChange(preset.key)}
              >
                {preset.label}
              </Button>
            ))}
          </div>
        </div>

        {/* 第二行: 日期选择器和粒度 */}
        <div className="flex flex-wrap gap-2 mt-3">
          <DatePicker
            type="dateTime"
            value={start_timestamp}
            onChange={handleStartDateChange}
            placeholder={t('起始时间')}
            className="flex-1 min-w-[140px]"
            size="small"
          />
          <span className="flex items-center text-gray-500">-</span>
          <DatePicker
            type="dateTime"
            value={end_timestamp}
            onChange={handleEndDateChange}
            placeholder={t('结束时间')}
            className="flex-1 min-w-[140px]"
            size="small"
          />
          <Select
            value={dataExportDefaultTime}
            onChange={onGranularityChange}
            optionList={translatedTimeOptions}
            className="w-20"
            size="small"
            prefix={t('粒度')}
          />
        </div>

        {/* 第三行: 用户名输入框 (仅管理员) */}
        {isAdminUser && (
          <div className="flex gap-2 mt-3">
            <Select
              value={username || undefined}
              onChange={(value) => onUsernameChange(value || '')}
              placeholder={t('用户名')}
              optionList={usernameOptions}
              filter={selectFilter}
              onSearch={onUsernameSearch}
              remote
              autoClearSearchValue={false}
              searchPosition='dropdown'
              loading={usernameSearchLoading}
              showClear
              className="flex-1"
              size="small"
            />
          </div>
        )}
      </div>
    );
  }

  // 桌面端布局 (一行)
  return (
    <div className="mb-4 p-4 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700">
      <div className="flex flex-wrap items-center gap-3">
        {/* 快捷按钮组 */}
        <div className="flex gap-2">
          {translatedPresets.map((preset) => (
            <Button
              key={preset.key}
              type={activePreset === preset.key ? 'primary' : 'tertiary'}
              size="small"
              onClick={() => onPresetChange(preset.key)}
            >
              {preset.label}
            </Button>
          ))}
        </div>

        {/* 分隔符 */}
        <div className="h-6 w-px bg-gray-300 dark:bg-gray-600" />

        {/* 日期范围选择器 */}
        <div className="flex items-center gap-2">
          <DatePicker
            type="dateTime"
            value={start_timestamp}
            onChange={handleStartDateChange}
            placeholder={t('起始时间')}
            className="w-44"
            size="small"
          />
          <span className="text-gray-500">-</span>
          <DatePicker
            type="dateTime"
            value={end_timestamp}
            onChange={handleEndDateChange}
            placeholder={t('结束时间')}
            className="w-44"
            size="small"
          />
        </div>

        {/* 分隔符 */}
        <div className="h-6 w-px bg-gray-300 dark:bg-gray-600" />

        {/* 时间粒度选择 */}
        <div className="flex items-center gap-2">
          <span className="text-gray-600 dark:text-gray-400 text-sm">{t('粒度')}:</span>
          <Select
            value={dataExportDefaultTime}
            onChange={onGranularityChange}
            optionList={translatedTimeOptions}
            className="w-20"
            size="small"
          />
        </div>

        {/* 分隔符 (仅管理员显示) */}
        {isAdminUser && <div className="h-6 w-px bg-gray-300 dark:bg-gray-600" />}

        {/* 用户名输入框 (仅管理员) */}
        {isAdminUser && (
          <Select
            value={username || undefined}
            onChange={(value) => onUsernameChange(value || '')}
            placeholder={t('用户名')}
            optionList={usernameOptions}
            filter={selectFilter}
            onSearch={onUsernameSearch}
            remote
            autoClearSearchValue={false}
            searchPosition='dropdown'
            loading={usernameSearchLoading}
            showClear
            className="w-40"
            size="small"
          />
        )}

      </div>
    </div>
  );
};

export default QuickFilterBar;
