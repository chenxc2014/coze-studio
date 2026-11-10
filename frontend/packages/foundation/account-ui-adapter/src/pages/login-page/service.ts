/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { useNavigate } from 'react-router-dom';
import { useEffect } from 'react';

import { useRequest } from 'ahooks';
import { passport } from '@coze-studio/api-schema';
import {
  setUserInfo,
  useLoginStatus,
  type UserInfo,
} from '@coze-foundation/account-adapter';

export const useLoginService = ({
  email,
  password,
}: {
  email: string;
  password: string;
}) => {
  const loginService = useRequest(
    async () => {
      const res = (await passport.PassportWebEmailLoginPost({
        email,
        password,
      })) as unknown as { data: UserInfo };
      return res.data;
    },
    {
      manual: true,
      onSuccess: setUserInfo,
    },
  );

  const registerService = useRequest(
    async () => {
      const res = (await passport.PassportWebEmailRegisterV2Post({
        email,
        password,
      })) as unknown as { data: UserInfo };
      return res.data;
    },
    {
      manual: true,
      onSuccess: setUserInfo,
    },
  );

  const loginStatus = useLoginStatus();
  const navigate = useNavigate();

  useEffect(() => {
    if (loginStatus === 'logined') {
      // 登录后恢复本站页面；管理后台由服务端提供，需要整页跳转。
      const target = new URLSearchParams(window.location.search).get('redirect') || '/';
      const url = new URL(target, window.location.origin);
      if (url.origin !== window.location.origin || !target.startsWith('/') || target.startsWith('//')) {
        navigate('/');
        return;
      }
      const pathname = url.pathname.replace(/^\/agent(?=\/|$)/, '') || '/';
      const destination = `${pathname}${url.search}${url.hash}`;
      if (/^\/admin(?:\/|$)/.test(pathname)) {
        const base = /^\/agent(?:\/|$)/.test(window.location.pathname) ? '/agent' : '';
        window.location.assign(`${base}${destination}`);
      } else {
        navigate(destination);
      }
    }
  }, [loginStatus]);

  return {
    login: loginService.run,
    register: registerService.run,
    loginLoading: loginService.loading,
    registerLoading: registerService.loading,
  };
};
