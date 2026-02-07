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

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Card,
  Divider,
  InputNumber,
  Select,
  Switch,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  API,
  getUserIdFromLocalStorage,
  processModelsData,
  showError,
  showSuccess,
} from '../../helpers';

const { Text } = Typography;
const STATUS_LABELS = {
  1: { label: '启用', color: 'green' },
  2: { label: '禁用', color: 'red' },
  3: { label: '自动禁用', color: 'yellow' },
};

const collectText = (content) => {
  if (!content) return '';
  if (typeof content === 'string') return content;
  if (Array.isArray(content)) {
    return content.map(collectText).filter(Boolean).join('');
  }
  if (typeof content.text === 'string') return content.text;
  if (typeof content.output_text === 'string') return content.output_text;
  if (content.content) return collectText(content.content);
  return '';
};

const extractTextFromResponse = (data) => {
  if (!data) return '';
  if (Array.isArray(data.choices)) {
    return data.choices
      .map((choice) => choice?.message?.content || choice?.text || '')
      .filter(Boolean)
      .join('\n');
  }
  if (Array.isArray(data.output)) {
    const pieces = data.output.map((item) => {
      if (item?.content) return collectText(item.content);
      if (typeof item?.text === 'string') return item.text;
      return '';
    });
    return pieces.filter(Boolean).join('');
  }
  if (Array.isArray(data.content)) {
    return collectText(data.content);
  }
  if (typeof data.output_text === 'string') return data.output_text;
  if (typeof data.text === 'string') return data.text;
  return '';
};

const extractTextFromStreamEvent = (payload) => {
  if (!payload) return '';
  const choice = payload?.choices?.[0];
  if (choice?.delta?.content) return choice.delta.content;
  if (choice?.delta?.reasoning_content) return choice.delta.reasoning_content;
  if (choice?.message?.content) return choice.message.content;
  if (typeof payload?.delta === 'string') return payload.delta;
  if (typeof payload?.delta?.text === 'string') return payload.delta.text;
  if (typeof payload?.text === 'string') return payload.text;
  if (payload?.content_block?.text) return payload.content_block.text;
  if (payload?.content) return collectText(payload.content);
  return '';
};

const ModelTest = () => {
  const { t } = useTranslation();
  const abortRef = useRef(null);

  const [tokens, setTokens] = useState([]);
  const [tokensLoading, setTokensLoading] = useState(false);
  const [tokenId, setTokenId] = useState(null);

  const [models, setModels] = useState([]);
  const [model, setModel] = useState('');

  const [group, setGroup] = useState('');

  const [endpoint, setEndpoint] = useState('chat');
  const [prompt, setPrompt] = useState('');
  const [output, setOutput] = useState('');
  const [rawEvents, setRawEvents] = useState([]);

  const [temperature, setTemperature] = useState(0.7);
  const [topP, setTopP] = useState(1);
  const [maxTokens, setMaxTokens] = useState(1024);
  const [stream, setStream] = useState(true);
  const [loading, setLoading] = useState(false);

  const endpointConfig = useMemo(
    () => ({
      chat: {
        label: 'OpenAI Chat',
        value: 'chat',
        desc: 'chat/completions 兼容',
        path: '/api/model_test/chat/completions',
        maxTokensKey: 'max_tokens',
      },
      responses: {
        label: 'OpenAI Responses',
        value: 'responses',
        desc: 'responses 原生接口',
        path: '/api/model_test/responses',
        maxTokensKey: 'max_output_tokens',
      },
      messages: {
        label: 'Claude Messages',
        value: 'messages',
        desc: 'Anthropic messages',
        path: '/api/model_test/messages',
        maxTokensKey: 'max_tokens',
      },
    }),
    [],
  );

  const endpointOptions = useMemo(
    () => Object.values(endpointConfig),
    [endpointConfig],
  );

  const tokenOptions = useMemo(
    () =>
      tokens.map((token) => ({
        label: `${token.name} (#${token.id})`,
        value: token.id,
        status: token.status,
        group: token.group,
        userId: token.user_id,
        disabled: token.status !== 1,
      })),
    [tokens],
  );

  const selectedToken = useMemo(
    () => tokens.find((item) => item.id === tokenId) || null,
    [tokens, tokenId],
  );

  const loadTokens = useCallback(async () => {
    setTokensLoading(true);
    try {
      const res = await API.get('/api/model_test/tokens');
      if (res.data.success) {
        const list = res.data.data || [];
        setTokens(list);
        if (!tokenId && list.length > 0) {
          const preferred = list.find((item) => item.status === 1) || list[0];
          setTokenId(preferred.id);
        }
      } else {
        showError(res.data.message || t('加载 Token 失败'));
      }
    } finally {
      setTokensLoading(false);
    }
  }, [tokenId, t]);

  const loadModels = useCallback(async () => {
    const res = await API.get('/api/user/models');
    if (res.data.success) {
      const { modelOptions, selectedModel } = processModelsData(
        res.data.data || [],
        model,
      );
      setModels(modelOptions);
      if (selectedModel && selectedModel !== model) {
        setModel(selectedModel);
      }
    } else {
      showError(res.data.message || t('加载模型失败'));
    }
  }, [model, t]);

  useEffect(() => {
    if (selectedToken?.group && selectedToken.group !== group) {
      setGroup(selectedToken.group);
    }
  }, [selectedToken, group]);

  useEffect(() => {
    loadTokens();
    loadModels();
    return () => {
      if (abortRef.current) {
        abortRef.current.abort();
      }
    };
  }, []);

  const buildPayload = () => {
    const base = {
      model,
      stream,
      temperature,
      top_p: topP,
    };
    if (group) {
      base.group = group;
    }
    if (endpoint === 'responses') {
      return {
        ...base,
        input: prompt,
        max_output_tokens: maxTokens,
      };
    }
    return {
      ...base,
      messages: [
        {
          role: 'user',
          content: prompt,
        },
      ],
      max_tokens: maxTokens,
    };
  };

  const handleStop = () => {
    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
      setLoading(false);
    }
  };

  const handleStart = async () => {
    if (!tokenId) {
      showError(t('请选择 Token'));
      return;
    }
    if (!model) {
      showError(t('请选择模型'));
      return;
    }
    if (!prompt.trim()) {
      showError(t('请输入测试内容'));
      return;
    }

    setOutput('');
    setRawEvents([]);
    setLoading(true);
    const controller = new AbortController();
    abortRef.current = controller;

    try {
      const res = await fetch(endpointConfig[endpoint].path, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'New-Api-User': getUserIdFromLocalStorage(),
        },
        credentials: 'include',
        body: JSON.stringify({
          token_id: tokenId,
          payload: buildPayload(),
        }),
        signal: controller.signal,
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || `HTTP ${res.status}`);
      }

      if (!stream) {
        const data = await res.json();
        const content = extractTextFromResponse(data);
        setOutput(content || JSON.stringify(data, null, 2));
        setLoading(false);
        showSuccess(t('测试完成'));
        return;
      }

      const reader = res.body?.getReader();
      if (!reader) {
        throw new Error(t('无法读取流式响应'));
      }
      const decoder = new TextDecoder('utf-8');
      let buffer = '';

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        let currentEvent = '';
        for (const line of lines) {
          const trimmed = line.trim();
          if (trimmed.startsWith('event:')) {
            currentEvent = trimmed.replace(/^event:\s*/, '');
            continue;
          }
          if (!trimmed.startsWith('data:')) continue;
          const data = trimmed.replace(/^data:\s*/, '');
          if (!data) continue;
          if (data === '[DONE]') {
            break;
          }
          setRawEvents((prev) => [
            ...prev,
            currentEvent ? `[${currentEvent}] ${data}` : data,
          ]);
          try {
            const json = JSON.parse(data);
            const chunk = extractTextFromStreamEvent(json);
            if (chunk) setOutput((prev) => prev + chunk);
          } catch (err) {
            // ignore parse errors
          }
        }
      }
      showSuccess(t('测试完成'));
    } catch (err) {
      if (err.name !== 'AbortError') {
        showError(err);
      }
    } finally {
      setLoading(false);
    }
  };

  const tokenRender = (option) => {
    const status = STATUS_LABELS[option.status] || {
      label: t('未知'),
      color: 'grey',
    };
    return (
      <div className='flex flex-col gap-1'>
        <div className='flex items-center gap-2'>
          <Text strong>{option.label}</Text>
          <Tag size='small' color={status.color}>
            {status.label}
          </Tag>
        </div>
        <Text size='small' type='secondary'>
          {t('分组')}: {option.group || '-'} · UID {option.userId}
        </Text>
      </div>
    );
  };

  const tokenSelectedRender = (option) => option?.label || '';

  const endpointRender = (option) => (
    <div className='flex flex-col gap-1'>
      <Text strong>{option.label}</Text>
      <Text size='small' type='secondary'>
        {option.desc}
      </Text>
    </div>
  );

  const endpointSelectedRender = (option) => (
    <Text strong>{option?.label || ''}</Text>
  );

  const tokenStatus =
    STATUS_LABELS[selectedToken?.status] || STATUS_LABELS[2];

  return (
    <div className='mt-[60px] px-2'>
      <Card className='!rounded-2xl !border-none bg-[radial-gradient(circle_at_top,_rgba(88,101,242,0.12),_rgba(15,17,23,0.68)_55%,_rgba(0,0,0,0.75)_100%)] shadow-[0_20px_50px_rgba(0,0,0,0.45)]'>
        <div className='flex flex-col gap-5'>
          <div className='flex flex-col md:flex-row md:items-center md:justify-between gap-3'>
            <div className='flex flex-col gap-1'>
              <Text strong className='text-lg'>
                {t('模型测试台')}
              </Text>
              <Text size='small' type='secondary'>
                {t('选择系统 Token，验证任意已启用模型的真实输出')}
              </Text>
            </div>
            <div className='flex flex-wrap items-center gap-2'>
              <Tag color='violet'>{endpointConfig[endpoint].label}</Tag>
              {selectedToken && (
                <Tag color={tokenStatus.color}>
                  {tokenStatus.label} · {selectedToken.name}
                </Tag>
              )}
            </div>
          </div>

          <div className='grid grid-cols-1 lg:grid-cols-3 gap-3'>
            <Select
              loading={tokensLoading}
              optionList={tokenOptions}
              value={tokenId}
              onChange={(value) => setTokenId(value)}
              renderSelectedItem={tokenSelectedRender}
              renderOptionItem={tokenRender}
              placeholder={t('选择 Token')}
              filter
            />
            <Select
              optionList={models}
              value={model}
              onChange={(value) => setModel(value)}
              placeholder={t('选择模型')}
              filter
            />
            <Select
              optionList={endpointOptions}
              value={endpoint}
              onChange={(value) => setEndpoint(value)}
              renderSelectedItem={endpointSelectedRender}
              renderOptionItem={endpointRender}
              placeholder={t('选择接口')}
              style={{ minWidth: 240 }}
            />
          </div>

          <div className='flex flex-wrap items-center gap-2 text-xs text-[var(--semi-color-text-2)]'>
            <span>{t('Token 分组')}:</span>
            <Tag size='small' color='blue'>
              {selectedToken?.group || group || 'default'}
            </Tag>
            <Divider layout='vertical' />
            <span>{t('参数键')}:</span>
            <Tag size='small' color='grey'>
              {endpointConfig[endpoint].maxTokensKey}
            </Tag>
          </div>

          <div className='grid grid-cols-1 md:grid-cols-4 gap-3'>
            <InputNumber
              value={temperature}
              min={0}
              max={2}
              step={0.1}
              onChange={setTemperature}
              placeholder='temperature'
              style={{ width: '100%' }}
            />
            <InputNumber
              value={topP}
              min={0}
              max={1}
              step={0.05}
              onChange={setTopP}
              placeholder='top_p'
              style={{ width: '100%' }}
            />
            <InputNumber
              value={maxTokens}
              min={1}
              max={8192}
              step={64}
              onChange={setMaxTokens}
              placeholder={endpointConfig[endpoint].maxTokensKey}
              style={{ width: '100%' }}
            />
            <div className='flex items-center gap-2'>
              <Switch checked={stream} onChange={setStream} />
              <Text size='small'>{t('流式输出')}</Text>
            </div>
          </div>

          <TextArea
            value={prompt}
            onChange={setPrompt}
            rows={5}
            placeholder={t('输入测试内容')}
          />

          <div className='flex flex-wrap gap-2'>
            <Button
              type='primary'
              theme='solid'
              loading={loading}
              onClick={handleStart}
            >
              {t('开始测试')}
            </Button>
            <Button disabled={!loading} onClick={handleStop}>
              {t('停止')}
            </Button>
            <Button
              theme='outline'
              onClick={() => {
                setOutput('');
                setRawEvents([]);
              }}
            >
              {t('清空')}
            </Button>
          </div>

          <div className='grid grid-cols-1 lg:grid-cols-2 gap-4'>
            <Card className='!rounded-xl !border-none bg-black/30' title={t('输出')}>
              <pre className='whitespace-pre-wrap text-sm min-h-[180px]'>
                {output || t('暂无输出')}
              </pre>
            </Card>
            <Card className='!rounded-xl !border-none bg-black/30' title={t('事件')}>
              <pre className='whitespace-pre-wrap text-xs min-h-[180px]'>
                {rawEvents.length > 0
                  ? rawEvents.join('\n')
                  : t('暂无事件')}
              </pre>
            </Card>
          </div>
        </div>
      </Card>
    </div>
  );
};

export default ModelTest;
