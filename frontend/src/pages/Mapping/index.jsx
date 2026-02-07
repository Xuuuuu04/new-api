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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Input,
  InputNumber,
  Modal,
  Select,
  Switch,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { CHANNEL_OPTIONS } from '../../constants/channel.constants';

const { Text } = Typography;

const buildChannelTypeMap = () => {
  const map = new Map();
  CHANNEL_OPTIONS.forEach((option) => {
    map.set(option.value, option);
  });
  return map;
};

const channelTypeMap = buildChannelTypeMap();

const statusTag = (status, t) => {
  switch (status) {
    case 1:
      return (
        <Tag color='green' shape='circle'>
          {t('已启用')}
        </Tag>
      );
    case 2:
      return (
        <Tag color='red' shape='circle'>
          {t('已禁用')}
        </Tag>
      );
    case 3:
      return (
        <Tag color='yellow' shape='circle'>
          {t('自动禁用')}
        </Tag>
      );
    default:
      return (
        <Tag color='grey' shape='circle'>
          {t('未知状态')}
        </Tag>
      );
  }
};

const MappingPage = () => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState([]);
  const [groups, setGroups] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState('all');
  const [enabledOnly, setEnabledOnly] = useState(true);
  const [keyword, setKeyword] = useState('');
  const [editValues, setEditValues] = useState({});
  const [mappingModal, setMappingModal] = useState({
    visible: false,
    loading: false,
    channelId: null,
    channelName: '',
    rows: [],
  });

  const loadData = useCallback(async (searchKeyword = '') => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      if (selectedGroup && selectedGroup !== 'all') {
        params.set('group', selectedGroup);
      }
      if (!enabledOnly) {
        params.set('enabled_only', 'false');
      }
      if (searchKeyword.trim()) {
        params.set('q', searchKeyword.trim());
      }
      const url = params.toString()
        ? `/api/mapping/?${params.toString()}`
        : '/api/mapping/';
      const res = await API.get(url);
      if (res.data.success) {
        setItems(res.data.data?.items || []);
        setGroups(res.data.data?.groups || []);
      } else {
        showError(res.data.message || t('加载失败'));
      }
    } finally {
      setLoading(false);
    }
  }, [selectedGroup, enabledOnly, t]);

  useEffect(() => {
    loadData(keyword);
  }, [selectedGroup, enabledOnly, loadData]);

  const groupOptions = useMemo(() => {
    return [
      { label: t('全部分组'), value: 'all' },
      ...groups.map((group) => ({ label: group, value: group })),
    ];
  }, [groups, t]);

  const tableData = useMemo(() => {
    return items.map((item) => ({
      ...item,
      key: `${item.group}::${item.model}`,
      channelsCount: item.channels?.length || 0,
    }));
  }, [items]);

  const getEditKey = (row) =>
    `${row.group}::${row.model}::${row.channel_id}`;

  const getEditState = (row) => {
    const key = getEditKey(row);
    if (!editValues[key]) {
      return { weight: row.weight, priority: row.priority };
    }
    return editValues[key];
  };

  const handleEditChange = (row, field, value) => {
    const key = getEditKey(row);
    setEditValues((prev) => ({
      ...prev,
      [key]: {
        ...getEditState(row),
        [field]: value,
      },
    }));
  };

  const handleSaveAbility = async (row) => {
    const key = getEditKey(row);
    const values = getEditState(row);
    const payload = {
      group: row.group,
      model: row.model,
      channel_id: row.channel_id,
      weight: Number(values.weight) || 0,
      priority: Number(values.priority) || 0,
    };
    const res = await API.patch('/api/mapping/ability', payload);
    if (res.data.success) {
      showSuccess(t('更新成功'));
      setEditValues((prev) => {
        const next = { ...prev };
        delete next[key];
        return next;
      });
      await loadData();
    } else {
      showError(res.data.message || t('更新失败'));
    }
  };

  const handleToggleAbility = async (row, enabled) => {
    const res = await API.patch('/api/mapping/ability_enabled', {
      group: row.group,
      model: row.model,
      channel_id: row.channel_id,
      enabled,
    });
    if (res.data.success) {
      showSuccess(t('更新成功'));
      await loadData();
    } else {
      showError(res.data.message || t('更新失败'));
    }
  };

  const handleToggleChannel = async (row, enabled) => {
    const res = await API.patch('/api/mapping/channel_status', {
      channel_id: row.channel_id,
      enabled,
    });
    if (res.data.success) {
      showSuccess(t('更新成功'));
      await loadData();
    } else {
      showError(res.data.message || t('更新失败'));
    }
  };

  const openMappingModal = async (row) => {
    setMappingModal({
      visible: true,
      loading: true,
      channelId: row.channel_id,
      channelName: row.channel_name,
      rows: [],
    });
    try {
      const res = await API.get(`/api/channel/${row.channel_id}`);
      if (!res.data.success) {
        showError(res.data.message || t('加载失败'));
        setMappingModal((prev) => ({ ...prev, loading: false }));
        return;
      }
      const channel = res.data.data || {};
      let mapping = {};
      if (channel.model_mapping) {
        try {
          mapping = JSON.parse(channel.model_mapping);
        } catch (_) {
          mapping = {};
        }
      }
      const rows = Object.entries(mapping).map(([model, upstream]) => ({
        id: `${model}-${upstream}-${Date.now()}`,
        model,
        upstream_model: upstream,
      }));
      setMappingModal((prev) => ({
        ...prev,
        loading: false,
        rows,
      }));
    } catch (err) {
      setMappingModal((prev) => ({ ...prev, loading: false }));
    }
  };

  const updateMappingRow = (id, field, value) => {
    setMappingModal((prev) => ({
      ...prev,
      rows: prev.rows.map((row) =>
        row.id === id ? { ...row, [field]: value } : row,
      ),
    }));
  };

  const addMappingRow = () => {
    setMappingModal((prev) => ({
      ...prev,
      rows: [
        ...prev.rows,
        {
          id: `new-${Date.now()}-${Math.random()}`,
          model: '',
          upstream_model: '',
        },
      ],
    }));
  };

  const removeMappingRow = (id) => {
    setMappingModal((prev) => ({
      ...prev,
      rows: prev.rows.filter((row) => row.id !== id),
    }));
  };

  const saveMapping = async () => {
    const rows = mappingModal.rows
      .map((row) => ({
        model: row.model.trim(),
        upstream_model: row.upstream_model.trim(),
      }))
      .filter((row) => row.model && row.upstream_model);

    const seen = new Set();
    for (const row of rows) {
      if (seen.has(row.model)) {
        showError(t('存在重复的模型映射'));
        return;
      }
      seen.add(row.model);
    }

    const res = await API.patch('/api/mapping/channel_mapping', {
      channel_id: mappingModal.channelId,
      mappings: rows,
    });

    if (res.data.success) {
      showSuccess(t('更新成功'));
      setMappingModal((prev) => ({ ...prev, visible: false }));
      await loadData();
    } else {
      showError(res.data.message || t('更新失败'));
    }
  };

  const columns = [
    {
      title: t('分组'),
      dataIndex: 'group',
      width: 140,
    },
    {
      title: t('系统模型'),
      dataIndex: 'model',
      render: (text) => (
        <Text style={{ fontWeight: 600 }} ellipsis={{ showTooltip: true }}>
          {text}
        </Text>
      ),
    },
    {
      title: t('可用渠道'),
      dataIndex: 'channelsCount',
      width: 100,
      align: 'center',
    },
  ];

  const renderChannelTable = (record) => {
    const channelRows = (record.channels || []).map((channel) => ({
      ...channel,
      key: `${record.group}-${record.model}-${channel.channel_id}`,
      group: record.group,
      model: record.model,
    }));

    const channelColumns = [
      {
        title: t('渠道'),
        dataIndex: 'channel_name',
        render: (text, row) => {
          const channelType = channelTypeMap.get(row.channel_type);
          return (
            <div className='flex flex-col'>
              <Text strong>
                {text} #{row.channel_id}
              </Text>
              <div className='flex items-center gap-2 mt-1'>
                {channelType && (
                  <Tag color={channelType.color} size='small'>
                    {channelType.label}
                  </Tag>
                )}
                {row.tag && <Tag size='small'>{row.tag}</Tag>}
              </div>
            </div>
          );
        },
      },
      {
        title: t('上游模型'),
        dataIndex: 'upstream_model',
        render: (text, row) => (
          <div className='flex flex-col'>
            <Text>{text}</Text>
            {row.mapping_applied && (
              <Tag color='blue' size='small' style={{ marginTop: 4 }}>
                {t('已映射')}
              </Tag>
            )}
          </div>
        ),
      },
      {
        title: t('权重'),
        dataIndex: 'weight',
        width: 120,
        render: (text, row) => (
          <InputNumber
            min={0}
            value={getEditState(row).weight}
            onChange={(value) => handleEditChange(row, 'weight', value)}
            style={{ width: 96 }}
          />
        ),
      },
      {
        title: t('优先级'),
        dataIndex: 'priority',
        width: 120,
        render: (text, row) => (
          <InputNumber
            min={0}
            value={getEditState(row).priority}
            onChange={(value) => handleEditChange(row, 'priority', value)}
            style={{ width: 96 }}
          />
        ),
      },
      {
        title: t('能力'),
        dataIndex: 'ability_enabled',
        width: 120,
        render: (value, row) => (
          <Switch
            checked={value}
            onChange={(checked) => handleToggleAbility(row, checked)}
          />
        ),
      },
      {
        title: t('渠道'),
        dataIndex: 'channel_status',
        width: 160,
        render: (value, row) => (
          <div className='flex items-center gap-2'>
            <Switch
              checked={value === 1}
              onChange={(checked) => handleToggleChannel(row, checked)}
            />
            {statusTag(value, t)}
          </div>
        ),
      },
      {
        title: t('操作'),
        dataIndex: 'actions',
        width: 180,
        render: (text, row) => {
          const values = getEditState(row);
          const isDirty =
            Number(values.weight) !== Number(row.weight) ||
            Number(values.priority) !== Number(row.priority);
          return (
            <div className='flex items-center gap-2'>
              <Button
                size='small'
                theme='solid'
                type='primary'
                disabled={!isDirty}
                onClick={() => handleSaveAbility(row)}
              >
                {t('保存')}
              </Button>
              <Button
                size='small'
                theme='outline'
                onClick={() => openMappingModal(row)}
              >
                {t('编辑映射')}
              </Button>
            </div>
          );
        },
      },
    ];

    return (
      <Table
        dataSource={channelRows}
        columns={channelColumns}
        pagination={false}
        size='small'
        rowKey='key'
      />
    );
  };

  return (
    <div className='mt-[60px] px-2'>
      <Card className='!rounded-2xl'>
        <div className='flex flex-col gap-3'>
          <div className='flex flex-col md:flex-row md:items-center gap-3'>
            <Select
              value={selectedGroup}
              onChange={(value) => setSelectedGroup(value)}
              optionList={groupOptions}
              style={{ minWidth: isMobile ? '100%' : 200 }}
            />
            <Input
              value={keyword}
              onChange={(value) => setKeyword(value)}
              placeholder={t('搜索模型')}
              onEnterPress={() => loadData(keyword)}
              style={{ minWidth: isMobile ? '100%' : 240 }}
            />
            <div className='flex items-center gap-2'>
              <Switch checked={enabledOnly} onChange={setEnabledOnly} />
              <Text size='small'>{t('仅启用渠道')}</Text>
            </div>
            <Button onClick={() => loadData(keyword)}>{t('刷新')}</Button>
          </div>

          <Table
            loading={loading}
            dataSource={tableData}
            columns={columns}
            expandedRowRender={renderChannelTable}
            pagination={{ pageSize: 20 }}
            rowKey='key'
          />
        </div>
      </Card>

      <Modal
        title={`${t('编辑映射')} - ${mappingModal.channelName || ''}`}
        visible={mappingModal.visible}
        onCancel={() =>
          setMappingModal((prev) => ({ ...prev, visible: false }))
        }
        onOk={saveMapping}
        okText={t('保存')}
        cancelText={t('取消')}
        width={720}
      >
        <div className='flex items-center justify-between mb-3'>
          <Text type='secondary'>{t('系统模型 → 上游模型')}</Text>
          <Button size='small' onClick={addMappingRow}>
            {t('新增映射')}
          </Button>
        </div>
        <Table
          dataSource={mappingModal.rows}
          loading={mappingModal.loading}
          pagination={false}
          size='small'
          rowKey='id'
          columns={[
            {
              title: t('系统模型'),
              dataIndex: 'model',
              render: (text, row) => (
                <Input
                  value={row.model}
                  onChange={(value) => updateMappingRow(row.id, 'model', value)}
                  placeholder='gpt-4o'
                />
              ),
            },
            {
              title: t('上游模型'),
              dataIndex: 'upstream_model',
              render: (text, row) => (
                <Input
                  value={row.upstream_model}
                  onChange={(value) =>
                    updateMappingRow(row.id, 'upstream_model', value)
                  }
                  placeholder='claude-3-5-sonnet-20241022'
                />
              ),
            },
            {
              title: t('操作'),
              width: 100,
              render: (text, row) => (
                <Button
                  size='small'
                  theme='borderless'
                  type='danger'
                  onClick={() => removeMappingRow(row.id)}
                >
                  {t('删除')}
                </Button>
              ),
            },
          ]}
        />
      </Modal>
    </div>
  );
};

export default MappingPage;
