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

import { useMemo } from 'react';
import { Server, KeyRound, Activity, Gauge } from 'lucide-react';
import {
  IconTextStroked,
  IconPulse,
  IconStopwatchStroked,
  IconTypograph,
  IconLayers,
  IconServer,
  IconKey,
  IconUserGroup,
} from '@douyinfe/semi-icons';
import { createSectionTitle } from '../../helpers/dashboard';

export const useDashboardStats = (
  userState,
  consumeTokens,
  times,
  trendData,
  performanceMetrics,
  overview,
  t,
) => {
  const channelsTotal = overview?.channelsTotal || 0;
  const channelsEnabled = overview?.channelsEnabled || 0;
  const modelsTotal = overview?.modelsTotal || 0;
  const tokensTotal = overview?.tokensTotal || 0;
  const defaultGroup = userState?.user?.group || 'default';

  const groupedStatsData = useMemo(
    () => [
      {
        title: createSectionTitle(Server, t('系统概览')),
        cardClassName:
          'bg-semi-color-bg-1 border border-semi-color-border shadow-sm border-l-4 border-l-semi-color-primary',
        items: [
          {
            title: t('可用模型'),
            value: modelsTotal,
            icon: <IconLayers />,
            avatarColor: 'indigo',
            trendData: [],
            trendColor: '#6366f1',
          },
          {
            title: t('启用渠道'),
            value:
              channelsTotal > 0
                ? `${channelsEnabled}/${channelsTotal}`
                : channelsEnabled,
            icon: <IconServer />,
            avatarColor: 'teal',
            trendData: [],
            trendColor: '#14b8a6',
          },
        ],
      },
      {
        title: createSectionTitle(KeyRound, t('令牌与分组')),
        cardClassName:
          'bg-semi-color-bg-1 border border-semi-color-border shadow-sm border-l-4 border-l-semi-color-success',
        items: [
          {
            title: t('Token 数'),
            value: tokensTotal,
            icon: <IconKey />,
            avatarColor: 'green',
            trendData: trendData.tokens,
            trendColor: '#22c55e',
          },
          {
            title: t('默认分组'),
            value: defaultGroup,
            icon: <IconUserGroup />,
            avatarColor: 'cyan',
            trendData: [],
            trendColor: '#06b6d4',
          },
        ],
      },
      {
        title: createSectionTitle(Activity, t('用量统计')),
        cardClassName:
          'bg-semi-color-bg-1 border border-semi-color-border shadow-sm border-l-4 border-l-semi-color-warning',
        items: [
          {
            title: t('统计 Tokens'),
            value: isNaN(consumeTokens) ? 0 : consumeTokens.toLocaleString(),
            icon: <IconTextStroked />,
            avatarColor: 'orange',
            trendData: trendData.tokens,
            trendColor: '#f97316',
          },
          {
            title: t('统计次数'),
            value: times,
            icon: <IconPulse />,
            avatarColor: 'purple',
            trendData: trendData.times,
            trendColor: '#8b5cf6',
          },
        ],
      },
      {
        title: createSectionTitle(Gauge, t('性能指标')),
        cardClassName:
          'bg-semi-color-bg-1 border border-semi-color-border shadow-sm border-l-4 border-l-semi-color-info',
        items: [
          {
            title: t('平均RPM'),
            value: performanceMetrics.avgRPM,
            icon: <IconStopwatchStroked />,
            avatarColor: 'indigo',
            trendData: trendData.rpm,
            trendColor: '#6366f1',
          },
          {
            title: t('平均TPM'),
            value: performanceMetrics.avgTPM,
            icon: <IconTypograph />,
            avatarColor: 'orange',
            trendData: trendData.tpm,
            trendColor: '#f97316',
          },
        ],
      },
    ],
    [
      consumeTokens,
      times,
      trendData,
      performanceMetrics,
      channelsTotal,
      channelsEnabled,
      modelsTotal,
      tokensTotal,
      defaultGroup,
      t,
    ],
  );

  return {
    groupedStatsData,
  };
};
