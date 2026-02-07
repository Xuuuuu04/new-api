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

import React, { useEffect, useRef, useState } from 'react';
import { Button, Card, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  API,
  removeTrailingSlash,
  showError,
  showSuccess,
  toBoolean,
} from '../../helpers';

const SystemSetting = () => {
  const { t } = useTranslation();
  const formApiRef = useRef(null);
  const [loading, setLoading] = useState(false);
  const [isLoaded, setIsLoaded] = useState(false);
  const [inputs, setInputs] = useState({
    ServerAddress: '',
    PasswordLoginEnabled: true,
    SelfUseModeEnabled: true,
  });

  const loadOptions = async () => {
    setLoading(true);
    const res = await API.get('/api/option/');
    const { success, message, data } = res.data;
    if (!success) {
      showError(message);
      setLoading(false);
      return;
    }

    const nextInputs = { ...inputs };
    data.forEach((item) => {
      if (item.key in nextInputs) {
        nextInputs[item.key] =
          item.key.endsWith('Enabled') || item.key.endsWith('enabled')
            ? toBoolean(item.value)
            : item.value;
      }
    });

    setInputs(nextInputs);
    if (formApiRef.current) {
      formApiRef.current.setValues(nextInputs);
    }
    setIsLoaded(true);
    setLoading(false);
  };

  useEffect(() => {
    loadOptions();
  }, []);

  const updateOption = async (key, value) => {
    setLoading(true);
    const res = await API.put('/api/option/', {
      key,
      value: typeof value === 'boolean' ? value.toString() : value,
    });
    if (!res.data.success) {
      showError(res.data.message);
      setLoading(false);
      return false;
    }
    showSuccess(t('更新成功'));
    setLoading(false);
    return true;
  };

  const submitServerAddress = async () => {
    const ServerAddress = removeTrailingSlash(inputs.ServerAddress);
    const ok = await updateOption('ServerAddress', ServerAddress);
    if (ok) {
      setInputs((prev) => ({ ...prev, ServerAddress }));
      formApiRef.current?.setValue('ServerAddress', ServerAddress);
    }
  };

  const handleToggle = async (key, event) => {
    const value = event.target.checked;
    const ok = await updateOption(key, value);
    if (ok) {
      setInputs((prev) => ({ ...prev, [key]: value }));
      formApiRef.current?.setValue(key, value);
    }
  };

  if (!isLoaded) {
    return (
      <div className='flex items-center justify-center h-[60vh]'>
        <Spin size='large' />
      </div>
    );
  }

  return (
    <Form
      initValues={inputs}
      onValueChange={(values) => setInputs(values)}
      getFormApi={(api) => (formApiRef.current = api)}
    >
      <div className='flex flex-col gap-4 mt-4'>
        <Card>
          <Form.Section text={t('基础设置')}>
            <Row gutter={16}>
              <Col span={24}>
                <Form.Input
                  field='ServerAddress'
                  label={t('服务器地址')}
                  placeholder='https://yourdomain.com'
                  extraText={t('用于 API 接入信息展示')}
                />
              </Col>
            </Row>
            <Button loading={loading} onClick={submitServerAddress}>
              {t('更新服务器地址')}
            </Button>
          </Form.Section>
        </Card>

        <Card>
          <Form.Section text={t('登录与模式')}>
            <Row gutter={16}>
              <Col span={24}>
                <Form.Checkbox
                  field='PasswordLoginEnabled'
                  noLabel
                  onChange={(e) => handleToggle('PasswordLoginEnabled', e)}
                >
                  {t('允许通过密码登录')}
                </Form.Checkbox>
                <Form.Checkbox
                  field='SelfUseModeEnabled'
                  noLabel
                  onChange={(e) => handleToggle('SelfUseModeEnabled', e)}
                >
                  {t('自用模式（隐藏注册与充值入口）')}
                </Form.Checkbox>
              </Col>
            </Row>
          </Form.Section>
        </Card>
      </div>
    </Form>
  );
};

export default SystemSetting;
