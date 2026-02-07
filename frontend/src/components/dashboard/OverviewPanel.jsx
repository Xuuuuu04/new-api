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

import React, { useMemo } from 'react';
import { Card, Tag, Skeleton } from '@douyinfe/semi-ui';
import { Layers, Server } from 'lucide-react';
import { CHANNEL_OPTIONS } from '../../constants/channel.constants';
import { FLEX_CENTER_GAP2 } from '../../constants/dashboard.constants';

const OverviewPanel = ({ overview, loading, t }) => {
  const channelTypeTags = useMemo(() => {
    const typeCounts = overview?.channelTypeCounts || {};
    const typeMap = new Map(
      CHANNEL_OPTIONS.map((item) => [String(item.value), item]),
    );
    return Object.entries(typeCounts)
      .map(([type, count]) => {
        const option = typeMap.get(String(type));
        return {
          type,
          count,
          color: option?.color || 'grey',
          label: option?.label || `${t('渠道类型')} ${type}`,
        };
      })
      .filter((item) => item.count > 0)
      .sort((a, b) => b.count - a.count);
  }, [overview?.channelTypeCounts, t]);

  const channelsTotal = overview?.channelsTotal || 0;
  const channelsEnabled = overview?.channelsEnabled || 0;
  const modelsTotal = overview?.modelsTotal || 0;
  const tokensTotal = overview?.tokensTotal || 0;

  return (
    <Card
      className='bg-semi-color-bg-1 border border-semi-color-border shadow-sm !rounded-2xl'
      title={
        <div className={FLEX_CENTER_GAP2}>
          <Server size={16} />
          {t('系统概览')}
        </div>
      }
      bodyStyle={{ padding: '12px' }}
    >
      <div className='grid grid-cols-2 gap-3 text-sm'>
        <div className='flex flex-col gap-1'>
          <span className='text-gray-500'>{t('可用模型')}</span>
          <Skeleton loading={loading} active>
            <span className='text-lg font-semibold'>{modelsTotal}</span>
          </Skeleton>
        </div>
        <div className='flex flex-col gap-1'>
          <span className='text-gray-500'>{t('启用渠道')}</span>
          <Skeleton loading={loading} active>
            <span className='text-lg font-semibold'>
              {channelsTotal > 0
                ? `${channelsEnabled}/${channelsTotal}`
                : channelsEnabled}
            </span>
          </Skeleton>
        </div>
        <div className='flex flex-col gap-1'>
          <span className='text-gray-500'>{t('Token 数')}</span>
          <Skeleton loading={loading} active>
            <span className='text-lg font-semibold'>{tokensTotal}</span>
          </Skeleton>
        </div>
        <div className='flex flex-col gap-1'>
          <span className='text-gray-500'>{t('渠道类型')}</span>
          <div className='flex items-center gap-2 text-gray-600'>
            <Layers size={14} />
            <span>{channelTypeTags.length}</span>
          </div>
        </div>
      </div>

      {channelTypeTags.length > 0 && (
        <div className='mt-3 flex flex-wrap gap-2'>
          {channelTypeTags.map((item) => (
            <Tag key={item.type} color={item.color} size='small'>
              {item.label} · {item.count}
            </Tag>
          ))}
        </div>
      )}
    </Card>
  );
};

export default OverviewPanel;
