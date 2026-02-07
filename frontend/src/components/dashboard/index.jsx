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

import React, { useContext, useEffect, useMemo } from 'react';
import { getRelativeTime, timestamp2string } from '../../helpers';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';

import DashboardHeader from './DashboardHeader';
import StatsCards from './StatsCards';
import ChartsPanel from './ChartsPanel';
import ApiInfoPanel from './ApiInfoPanel';
import OverviewPanel from './OverviewPanel';
import SearchModal from './modals/SearchModal';

import { useDashboardData } from '../../hooks/dashboard/useDashboardData';
import { useDashboardStats } from '../../hooks/dashboard/useDashboardStats';
import { useDashboardCharts } from '../../hooks/dashboard/useDashboardCharts';

import {
  CHART_CONFIG,
  CARD_PROPS,
  FLEX_CENTER_GAP2,
  ILLUSTRATION_SIZE,
} from '../../constants/dashboard.constants';
import { getTrendSpec, handleCopyUrl, handleSpeedTest } from '../../helpers/dashboard';

const Dashboard = () => {
  // ========== Context ==========
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState, statusDispatch] = useContext(StatusContext);

  // ========== 主要数据管理 ==========
  const dashboardData = useDashboardData(userState, userDispatch, statusState);

  // ========== 图表管理 ==========
  const dashboardCharts = useDashboardCharts(
    dashboardData.dataExportDefaultTime,
    dashboardData.setTrendData,
    dashboardData.setConsumeQuota,
    dashboardData.setTimes,
    dashboardData.setConsumeTokens,
    dashboardData.setPieData,
    dashboardData.setLineData,
    dashboardData.setModelColors,
    dashboardData.t,
  );

  // ========== 统计数据 ==========
  const { groupedStatsData } = useDashboardStats(
    userState,
    dashboardData.consumeTokens,
    dashboardData.times,
    dashboardData.trendData,
    dashboardData.performanceMetrics,
    dashboardData.overview,
    dashboardData.t,
  );

  // ========== 数据处理 ==========
  const initChart = async () => {
    await dashboardData.loadQuotaData().then((data) => {
      if (data && data.length > 0) {
        dashboardCharts.updateChartData(data);
      }
    });
    await dashboardData.loadOverviewData();
  };

  const handleRefresh = async () => {
    const data = await dashboardData.refresh();
    if (data && data.length > 0) {
      dashboardCharts.updateChartData(data);
    }
  };

  const handleSearchConfirm = async () => {
    await dashboardData.handleSearchConfirm(dashboardCharts.updateChartData);
  };

  // ========== 数据准备 ==========
  const apiInfoData = statusState?.status?.api_info || [];
  const baseUrl = useMemo(() => {
    const serverAddress = statusState?.status?.server_address;
    const fallback =
      typeof window !== 'undefined' ? window.location.origin : '';
    const raw = (serverAddress || fallback || '').trim();
    return raw.replace(/\/$/, '');
  }, [statusState?.status?.server_address]);

  const wsBaseUrl = useMemo(() => {
    if (!baseUrl) return '';
    return baseUrl.replace(/^https?:\/\//, (match) =>
      match.startsWith('https') ? 'wss://' : 'ws://',
    );
  }, [baseUrl]);

  const autoApiInfoData = useMemo(() => {
    if (!baseUrl) return [];
    return [
      {
        id: 'openai-base',
        route: '/v1',
        url: `${baseUrl}/v1`,
        description: dashboardData.t('OpenAI 兼容基础地址'),
        color: 'blue',
      },
      {
        id: 'openai-chat',
        route: '/v1/chat/completions',
        url: `${baseUrl}/v1/chat/completions`,
        description: dashboardData.t('OpenAI Chat Completions'),
        color: 'green',
      },
      {
        id: 'openai-responses',
        route: '/v1/responses',
        url: `${baseUrl}/v1/responses`,
        description: dashboardData.t('OpenAI Responses'),
        color: 'violet',
      },
      {
        id: 'claude-messages',
        route: '/v1/messages',
        url: `${baseUrl}/v1/messages`,
        description: dashboardData.t('Claude Messages'),
        color: 'orange',
      },
      {
        id: 'realtime',
        route: '/v1/realtime',
        url: `${wsBaseUrl}/v1/realtime`,
        description: dashboardData.t('Realtime WebSocket'),
        color: 'pink',
      },
    ];
  }, [baseUrl, wsBaseUrl, dashboardData.t]);

  const usageSummary = useMemo(() => {
    const modelTokens = new Map();
    const modelCalls = new Map();
    let lastActivity = 0;
    const data = dashboardData.quotaData || [];

    data.forEach((item) => {
      const modelName = item.model_name || dashboardData.t('未知模型');
      if (modelName === '无数据') return;
      const tokens = Number(item.token_used || 0);
      const calls = Number(item.count || 0);
      modelTokens.set(modelName, (modelTokens.get(modelName) || 0) + tokens);
      modelCalls.set(modelName, (modelCalls.get(modelName) || 0) + calls);
      if (item.created_at && item.created_at > lastActivity) {
        lastActivity = item.created_at;
      }
    });

    const topByTokens = Array.from(modelTokens.entries())
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5);
    const topByCalls = Array.from(modelCalls.entries())
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5);

    const lastActivityLabel = lastActivity
      ? `${timestamp2string(lastActivity)} (${getRelativeTime(timestamp2string(lastActivity))})`
      : dashboardData.t('暂无数据');

    return {
      lastActivityLabel,
      topByTokens,
      topByCalls,
      totalTokens: dashboardData.consumeTokens,
      totalCalls: dashboardData.times,
      timeRange: `${dashboardData.inputs.start_timestamp} ~ ${dashboardData.inputs.end_timestamp}`,
    };
  }, [
    dashboardData.quotaData,
    dashboardData.consumeTokens,
    dashboardData.times,
    dashboardData.inputs.start_timestamp,
    dashboardData.inputs.end_timestamp,
    dashboardData.t,
  ]);

  // ========== Effects ==========
  useEffect(() => {
    initChart();
  }, []);

  return (
    <div className='h-full'>
      <DashboardHeader
        getGreeting={dashboardData.getGreeting}
        greetingVisible={dashboardData.greetingVisible}
        showSearchModal={dashboardData.showSearchModal}
        refresh={handleRefresh}
        loading={dashboardData.loading}
        t={dashboardData.t}
      />

      <SearchModal
        searchModalVisible={dashboardData.searchModalVisible}
        handleSearchConfirm={handleSearchConfirm}
        handleCloseModal={dashboardData.handleCloseModal}
        isMobile={dashboardData.isMobile}
        isAdminUser={dashboardData.isAdminUser}
        inputs={dashboardData.inputs}
        dataExportDefaultTime={dashboardData.dataExportDefaultTime}
        timeOptions={dashboardData.timeOptions}
        handleInputChange={dashboardData.handleInputChange}
        t={dashboardData.t}
      />

      <StatsCards
        groupedStatsData={groupedStatsData}
        loading={dashboardData.loading}
        getTrendSpec={getTrendSpec}
        CARD_PROPS={CARD_PROPS}
        CHART_CONFIG={CHART_CONFIG}
      />

      {/* 图表与接入/概览面板 */}
      <div className='mb-4'>
        <div className='grid grid-cols-1 lg:grid-cols-3 gap-4'>
          <div className='flex flex-col gap-4 lg:col-span-2'>
            <ChartsPanel
              activeChartTab={dashboardData.activeChartTab}
              setActiveChartTab={dashboardData.setActiveChartTab}
              spec_line={dashboardCharts.spec_line}
              spec_model_line={dashboardCharts.spec_model_line}
              spec_pie={dashboardCharts.spec_pie}
              spec_rank_bar={dashboardCharts.spec_rank_bar}
              CARD_PROPS={CARD_PROPS}
              CHART_CONFIG={CHART_CONFIG}
              FLEX_CENTER_GAP2={FLEX_CENTER_GAP2}
              usageSummary={usageSummary}
              usageSummaryLoading={dashboardData.loading}
              t={dashboardData.t}
            />
          </div>

          <div className='flex flex-col gap-4'>
            <ApiInfoPanel
              apiInfoData={apiInfoData}
              autoApiInfoData={autoApiInfoData}
              handleCopyUrl={(url) => handleCopyUrl(url, dashboardData.t)}
              handleSpeedTest={handleSpeedTest}
              CARD_PROPS={CARD_PROPS}
              FLEX_CENTER_GAP2={FLEX_CENTER_GAP2}
              ILLUSTRATION_SIZE={ILLUSTRATION_SIZE}
              title={dashboardData.t('接入信息')}
              emptyDescription={dashboardData.t('未获取到可用服务地址')}
              t={dashboardData.t}
            />
            <OverviewPanel
              overview={dashboardData.overview}
              loading={dashboardData.loading || dashboardData.overviewLoading}
              t={dashboardData.t}
            />
          </div>
        </div>
      </div>
    </div>
  );
};

export default Dashboard;
