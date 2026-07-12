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

import { Button, Card, Tag, Typography } from '@douyinfe/semi-ui';
import { CreditCard, Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../../helpers';

const { Text, Title } = Typography;

const WalletCard = ({ balance, rechargeDisabled, onRecharge }) => {
  const { t } = useTranslation();
  return (
    <Card className='!rounded-lg border border-semi-color-border shadow-sm'>
      <div className='flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between'>
        <div className='min-w-0'>
          <div className='flex flex-wrap items-center gap-2'>
            <Title heading={5} className='m-0'>
              <CreditCard size={17} className='mr-1 inline' /> {t('按量付费')}
            </Title>
            <Tag color='green'>{t('钱包余额')}</Tag>
            <Tag>{t('永不过期')}</Tag>
            <Tag>{t('按量扣费')}</Tag>
            <Tag>{t('即时到账')}</Tag>
          </div>
          <Text type='tertiary' className='mt-2 block'>
            {t('余额按实际使用量扣费，永不过期')}
          </Text>
        </div>
        <div className='flex flex-col items-stretch gap-2 sm:items-end'>
          <Text strong className='text-xl'>
            {renderQuota(balance)}
          </Text>
          {!rechargeDisabled && (
            <Button
              className='!min-h-10'
              icon={<Plus size={16} />}
              theme='solid'
              type='primary'
              onClick={onRecharge}
            >
              {t('充值')}
            </Button>
          )}
        </div>
      </div>
    </Card>
  );
};

export default WalletCard;
