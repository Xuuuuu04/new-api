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
import { Skeleton, Tag } from '@douyinfe/semi-ui';
import { Activity, Clock, Hash, Star, Zap } from 'lucide-react';

const UsageSummaryStrip = ({ summary, loading, t }) => {
  const topTokens = summary?.topByTokens || [];
  const topCalls = summary?.topByCalls || [];
  const topTokenModel = topTokens[0]?.[0] || t('暂无数据');
  const topTokenValue = Number(topTokens[0]?.[1] || 0).toLocaleString();
  const topCallModel = topCalls[0]?.[0] || t('暂无数据');
  const topCallValue = Number(topCalls[0]?.[1] || 0).toLocaleString();

  const stats = [
    {
      id: 'last',
      label: t('最近请求'),
      value: summary?.lastActivityLabel || t('暂无数据'),
      icon: <Clock size={16} />,
    },
    {
      id: 'tokens',
      label: t('统计 Tokens'),
      value: Number(summary?.totalTokens || 0).toLocaleString(),
      icon: <Hash size={16} />,
    },
    {
      id: 'calls',
      label: t('统计次数'),
      value: Number(summary?.totalCalls || 0).toLocaleString(),
      icon: <Activity size={16} />,
    },
    {
      id: 'top-tokens',
      label: t('Tokens Top'),
      value: `${topTokenModel} · ${topTokenValue}`,
      icon: <Star size={16} />,
    },
    {
      id: 'top-calls',
      label: t('调用次数 Top'),
      value: `${topCallModel} · ${topCallValue}`,
      icon: <Zap size={16} />,
    },
  ];

  return (
    <div className='border-t border-semi-color-border px-4 pb-4 pt-3'>
      <div className='flex items-center gap-2 text-xs text-gray-500 mb-3'>
        <Activity size={14} />
        {t('最近使用')}
        <Tag size='small' type='light' color='grey'>
          {summary?.timeRange || '-'}
        </Tag>
      </div>

      <div className='grid grid-cols-1 md:grid-cols-5 gap-3'>
        {stats.map((item) => (
          <div
            key={item.id}
            className='flex items-start gap-3 rounded-xl border border-semi-color-border bg-semi-color-bg-0 px-3 py-2'
          >
            <div className='text-gray-500'>{item.icon}</div>
            <div className='min-w-0'>
              <div className='text-[11px] text-gray-500'>{item.label}</div>
              <Skeleton loading={loading} active>
                <div className='text-sm font-semibold text-gray-900 truncate'>
                  {item.value}
                </div>
              </Skeleton>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

export default UsageSummaryStrip;
