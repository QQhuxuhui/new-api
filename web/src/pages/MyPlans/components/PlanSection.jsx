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

import { Typography } from '@douyinfe/semi-ui';

const { Text, Title } = Typography;

const PlanSection = ({ id, title, count, children }) => {
  if (!count) return null;
  return (
    <section id={id} aria-labelledby={`${id}-title`}>
      <div className='mb-3 flex items-baseline gap-2'>
        <Title id={`${id}-title`} heading={5} className='m-0'>
          {title}
        </Title>
        <Text type='tertiary'>{count}</Text>
      </div>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3'>
        {children}
      </div>
    </section>
  );
};

export default PlanSection;
