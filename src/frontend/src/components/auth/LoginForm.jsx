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

import React, { useContext, useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import Turnstile from 'react-turnstile';
import { Button, Card, Checkbox, Form, Icon, Modal } from '@douyinfe/semi-ui';
import Title from '@douyinfe/semi-ui/lib/es/typography/title';
import Text from '@douyinfe/semi-ui/lib/es/typography/text';
import { IconMail, IconLock } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import {
  API,
  getLogo,
  showError,
  showInfo,
  showSuccess,
  updateAPI,
  getSystemName,
  setUserData,
} from '../../helpers';

const LoginForm = () => {
  const navigate = useNavigate();
  const { t } = useTranslation();
  const [, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const [searchParams] = useSearchParams();

  const [inputs, setInputs] = useState({ username: '', password: '' });
  const [turnstileEnabled, setTurnstileEnabled] = useState(false);
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');
  const [loginLoading, setLoginLoading] = useState(false);
  const [agreedToTerms, setAgreedToTerms] = useState(false);
  const [hasUserAgreement, setHasUserAgreement] = useState(false);
  const [hasPrivacyPolicy, setHasPrivacyPolicy] = useState(false);

  const logo = getLogo();
  const systemName = getSystemName();

  const status = useMemo(() => {
    if (statusState?.status) return statusState.status;
    const savedStatus = localStorage.getItem('status');
    if (!savedStatus) return {};
    try {
      return JSON.parse(savedStatus) || {};
    } catch (err) {
      return {};
    }
  }, [statusState?.status]);

  useEffect(() => {
    if (status?.turnstile_check) {
      setTurnstileEnabled(true);
      setTurnstileSiteKey(status.turnstile_site_key);
    }
    setHasUserAgreement(status?.user_agreement_enabled || false);
    setHasPrivacyPolicy(status?.privacy_policy_enabled || false);
  }, [status]);

  useEffect(() => {
    if (searchParams.get('expired')) {
      showError(t('未登录或登录已过期，请重新登录'));
    }
  }, [searchParams, t]);

  const handleChange = (name, value) => {
    setInputs((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async () => {
    if ((hasUserAgreement || hasPrivacyPolicy) && !agreedToTerms) {
      showInfo(t('请先阅读并同意用户协议和隐私政策'));
      return;
    }
    if (turnstileEnabled && turnstileToken === '') {
      showInfo('请稍后几秒重试，Turnstile 正在检查用户环境！');
      return;
    }
    const { username, password } = inputs;
    if (!username || !password) {
      showError('请输入用户名和密码！');
      return;
    }
    setLoginLoading(true);
    try {
      const res = await API.post(
        `/api/user/login?turnstile=${turnstileToken}`,
        { username, password },
      );
      const { success, message, data } = res.data;
      if (success) {
        userDispatch({ type: 'login', payload: data });
        setUserData(data);
        updateAPI();
        showSuccess('登录成功！');
        if (username === 'root' && password === '123456') {
          Modal.error({
            title: '您正在使用默认密码！',
            content: '请立刻修改默认密码！',
            centered: true,
          });
        }
        navigate('/console');
      } else {
        showError(message);
      }
    } catch (error) {
      showError('登录失败，请重试');
    } finally {
      setLoginLoading(false);
    }
  };

  const renderAgreement = () => {
    if (!hasUserAgreement && !hasPrivacyPolicy) return null;

    return (
      <Checkbox
        checked={agreedToTerms}
        onChange={(checked) => setAgreedToTerms(checked)}
      >
        <span className='text-xs'>
          {t('我已阅读并同意')}
          {hasUserAgreement && (
            <a
              href='/user-agreement'
              target='_blank'
              rel='noopener noreferrer'
              className='mx-1 text-semi-color-link'
            >
              {t('用户协议')}
            </a>
          )}
          {hasUserAgreement && hasPrivacyPolicy && <span>{t('和')}</span>}
          {hasPrivacyPolicy && (
            <a
              href='/privacy-policy'
              target='_blank'
              rel='noopener noreferrer'
              className='mx-1 text-semi-color-link'
            >
              {t('隐私政策')}
            </a>
          )}
        </span>
      </Checkbox>
    );
  };

  return (
    <div className='relative overflow-hidden bg-gray-100 flex items-center justify-center py-12 px-4 sm:px-6 lg:px-8'>
      <div
        className='blur-ball blur-ball-indigo'
        style={{ top: '-80px', right: '-80px', transform: 'none' }}
      />
      <div
        className='blur-ball blur-ball-teal'
        style={{ top: '50%', left: '-120px' }}
      />
      <div className='w-full max-w-sm mt-[60px]'>
        <Card className='!bg-white !border-0 !shadow-xl !rounded-2xl'>
          <div className='flex flex-col items-center gap-2 mb-6'>
            {logo && (
              <img src={logo} alt='logo' className='w-12 h-12 rounded' />
            )}
            <Title heading={4} className='!mb-0'>
              {systemName || 'New API'}
            </Title>
            <Text type='tertiary'>{t('使用账号密码登录')}</Text>
          </div>
          <Form
            layout='vertical'
            onSubmit={handleSubmit}
            labelPosition='inset'
          >
            <Form.Input
              field='username'
              label={t('用户名')}
              placeholder={t('请输入用户名')}
              value={inputs.username}
              onChange={(value) => handleChange('username', value)}
              prefix={<IconMail />}
              showClear
            />
            <Form.Input
              field='password'
              label={t('密码')}
              type='password'
              placeholder={t('请输入密码')}
              value={inputs.password}
              onChange={(value) => handleChange('password', value)}
              prefix={<IconLock />}
              showClear
            />
            <div className='flex items-center justify-between mb-4'>
              {renderAgreement()}
            </div>
            <Button
              htmlType='submit'
              type='primary'
              theme='solid'
              block
              loading={loginLoading}
              icon={<Icon type='enter' />}
            >
              {t('登录')}
            </Button>
          </Form>
        </Card>
        {turnstileEnabled && (
          <div className='flex justify-center mt-6'>
            <Turnstile
              sitekey={turnstileSiteKey}
              onVerify={(token) => {
                setTurnstileToken(token);
              }}
            />
          </div>
        )}
      </div>
    </div>
  );
};

export default LoginForm;
