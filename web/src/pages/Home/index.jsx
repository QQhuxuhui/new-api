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

import React, { useContext, useEffect, useState, useRef } from 'react';
import { Button } from '@douyinfe/semi-ui';
import { API, showError, renderMarkdown } from '../../helpers';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { StatusContext } from '../../context/Status';
import { useActualTheme } from '../../context/Theme';
import { useTranslation } from 'react-i18next';
import '../../components/common/markdown/markdown.css';
import { IconPlay, IconFile, IconGithubLogo } from '@douyinfe/semi-icons';
import { Link, useNavigate } from 'react-router-dom';
import NoticeModal from '../../components/layout/NoticeModal';
import PosterModal from '../../components/layout/PosterModal';
import OnboardingWizard from '../../components/onboarding/OnboardingWizard';
import HomeBento from './HomeBento';
import './Home.css';

// simpleStringHash8 — 把字符串映射到 8 位 hex(32-bit FNV-1a 变种)。
// 用于 localStorage poster_seen key 的版本标识,不是安全级 hash。
// 海报 URL 变化时自然产生不同 key,触发"换图重弹"。
const simpleStringHash8 = (str) => {
  let h = 0x811c9dc5;
  for (let i = 0; i < str.length; i++) {
    h ^= str.charCodeAt(i);
    h = (h * 0x01000193) >>> 0;
  }
  return ('00000000' + h.toString(16)).slice(-8);
};

const Home = () => {
  const { t, i18n } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const actualTheme = useActualTheme();
  const [homePageContentLoaded, setHomePageContentLoaded] = useState(false);
  const [homePageContent, setHomePageContent] = useState('');
  const [noticeVisible, setNoticeVisible] = useState(false);
  const [posterVisible, setPosterVisible] = useState(false);
  const [posterImageUrl, setPosterImageUrl] = useState('');
  const [posterClickUrl, setPosterClickUrl] = useState('');
  const [onboardingVisible, setOnboardingVisible] = useState(false);
  const iframeRef = useRef(null);
  const isMobile = useIsMobile();
  const isDemoSiteMode = statusState?.status?.demo_site_enabled || false;
  const docsLink = statusState?.status?.docs_link || '';
  const xianyuShopLink = statusState?.status?.xianyu_shop_link || '';
  const isChinese = i18n.language.startsWith('zh');
  const navigate = useNavigate();

  // Check if user is logged in
  const isLoggedIn = !!localStorage.getItem('user');

  const displayHomePageContent = async () => {
    const cached = localStorage.getItem('home_page_content') || '';
    setHomePageContent(
      cached.startsWith('https://') ? cached : renderMarkdown(cached),
    );
    try {
      const res = await API.get('/api/home_page_content');
      const { success, message, data } = res.data;
      if (success) {
        let content = data;
        if (!data.startsWith('https://')) {
          content = renderMarkdown(data);
        }
        setHomePageContent(content);
        localStorage.setItem('home_page_content', content);
      } else {
        showError(message);
        setHomePageContent('加载首页内容失败...');
      }
    } catch (error) {
      console.error('获取首页内容失败:', error);
      setHomePageContent('');
    } finally {
      setHomePageContentLoaded(true);
    }
  };

  // 处理 iframe 消息发送
  const handleIframeLoad = () => {
    if (iframeRef.current?.contentWindow) {
      iframeRef.current.contentWindow.postMessage(
        { themeMode: actualTheme },
        '*',
      );
      iframeRef.current.contentWindow.postMessage({ lang: i18n.language }, '*');
    }
  };

  useEffect(() => {
    // 海报优先级:有海报且 EnablePoster=true → 弹海报;否则回退到现有公告
    // 频率:海报用 poster_seen_<hash8>_<YYYYMMDD>,公告沿用 notice_close_date
    const checkPosterOrNotice = async () => {
      const todayDate = new Date();
      const yyyymmdd = `${todayDate.getFullYear()}${String(
        todayDate.getMonth() + 1
      ).padStart(2, '0')}${String(todayDate.getDate()).padStart(2, '0')}`;

      // 优先尝试海报
      try {
        const res = await API.get('/api/poster');
        const { success, data } = res.data || {};
        if (
          success &&
          data &&
          data.enabled === true &&
          typeof data.image_url === 'string' &&
          data.image_url.trim() !== ''
        ) {
          const hash8 = simpleStringHash8(data.image_url);
          const key = `poster_seen_${hash8}_${yyyymmdd}`;
          if (!localStorage.getItem(key)) {
            setPosterImageUrl(data.image_url);
            setPosterClickUrl(data.click_url || '');
            setPosterVisible(true);
          }
          // 有海报无论是否当天看过,**都不**回退到公告(优先级互斥)
          return;
        }
      } catch (error) {
        // 静默降级到公告
        console.debug('获取海报失败,回退到公告:', error);
      }

      // 回退:现有公告
      const lastCloseDate = localStorage.getItem('notice_close_date');
      const todayStr = todayDate.toDateString();
      if (lastCloseDate !== todayStr) {
        try {
          const res = await API.get('/api/notice');
          const { success, data } = res.data;
          if (success && data && data.trim() !== '') {
            setNoticeVisible(true);
          }
        } catch (error) {
          console.error('获取公告失败:', error);
        }
      }
    };

    checkPosterOrNotice();
  }, []);

  // 普通关闭:仅 setVisible(false),下次刷新还会弹(与公告 NoticeModal 默认行为一致)
  const handlePosterClose = () => {
    setPosterVisible(false);
  };

  // "今日不再弹":写 localStorage 后关闭,当天不再弹同一海报
  const handlePosterCloseToday = () => {
    if (posterImageUrl) {
      const todayDate = new Date();
      const yyyymmdd = `${todayDate.getFullYear()}${String(
        todayDate.getMonth() + 1
      ).padStart(2, '0')}${String(todayDate.getDate()).padStart(2, '0')}`;
      const hash8 = simpleStringHash8(posterImageUrl);
      try {
        localStorage.setItem(`poster_seen_${hash8}_${yyyymmdd}`, '1');
      } catch (_) {
        // localStorage 不可用时静默忽略
      }
    }
    setPosterVisible(false);
  };

  useEffect(() => {
    displayHomePageContent().then();
  }, []);

  // 监听主题和语言变化，实时同步给 iframe
  useEffect(() => {
    if (
      homePageContent.startsWith('https://') &&
      iframeRef.current?.contentWindow
    ) {
      try {
        iframeRef.current.contentWindow.postMessage(
          { themeMode: actualTheme },
          '*',
        );
        iframeRef.current.contentWindow.postMessage(
          { lang: i18n.language },
          '*',
        );
      } catch (error) {
        // 忽略跨域错误
        console.debug('Cannot post message to iframe:', error);
      }
    }
  }, [homePageContent, actualTheme, i18n.language]);

  return (
    <div className='home-container'>
      <NoticeModal
        visible={noticeVisible}
        onClose={() => setNoticeVisible(false)}
        isMobile={isMobile}
      />
      <PosterModal
        visible={posterVisible}
        imageUrl={posterImageUrl}
        clickUrl={posterClickUrl}
        onClose={handlePosterClose}
        onCloseToday={handlePosterCloseToday}
      />
      <OnboardingWizard
        visible={onboardingVisible}
        onClose={() => setOnboardingVisible(false)}
      />
      {homePageContentLoaded && homePageContent === '' ? (
        <HomeBento />
      ) : (
        <div className='overflow-x-hidden w-full'>
          {homePageContent.startsWith('https://') ? (
            <iframe
              ref={iframeRef}
              src={homePageContent}
              onLoad={handleIframeLoad}
              className='w-full h-screen border-none'
            />
          ) : (
            <div
              className='markdown-body mt-[60px]'
              dangerouslySetInnerHTML={{
                __html: homePageContent,
              }}
            />
          )}
        </div>
      )}
    </div>
  );
};

export default Home;
