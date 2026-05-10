import React, { useState } from 'react';
import { Modal } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

/**
 * PosterModal — 首页海报弹窗(替代/优先于现有公告)
 *
 * Props:
 *   visible:  bool      是否显示
 *   imageUrl: string    海报图片 URL(OSS 公开 URL 或外链)
 *   clickUrl: string    点击跳转 URL,空时图片不可点击
 *   onClose:  () => void 关闭回调(同时父组件应写 localStorage 阻止当天再弹)
 *
 * 行为:
 *   - imageUrl 非空时显示大图,关闭按钮右上
 *   - clickUrl 非空时整张图包 <a target=_blank rel=noopener>
 *   - <img onError> 静默隐藏图(仍允许 modal 关闭),不阻断退出
 */
const PosterModal = ({ visible, imageUrl, clickUrl, onClose }) => {
  const { t } = useTranslation();
  const [imgFailed, setImgFailed] = useState(false);

  if (!imageUrl) return null;

  const img = (
    <img
      src={imageUrl}
      alt={t('海报')}
      style={{
        display: imgFailed ? 'none' : 'block',
        maxWidth: '100%',
        maxHeight: '80vh',
        margin: '0 auto',
        borderRadius: 8,
      }}
      onError={() => setImgFailed(true)}
    />
  );

  const content = clickUrl ? (
    <a
      href={clickUrl}
      target='_blank'
      rel='noopener noreferrer'
      style={{ display: 'block', cursor: 'pointer' }}
    >
      {img}
    </a>
  ) : (
    img
  );

  return (
    <Modal
      visible={visible}
      onCancel={onClose}
      footer={null}
      centered
      maskClosable={true}
      closeOnEsc={true}
      width={620}
      bodyStyle={{ padding: 0, background: 'transparent' }}
      style={{ maxWidth: '90vw' }}
    >
      <div style={{ padding: 0, textAlign: 'center' }}>
        {content}
        {imgFailed && (
          <div
            style={{
              padding: 24,
              color: '#999',
              fontSize: 14,
            }}
          >
            {t('海报图片加载失败')}
          </div>
        )}
      </div>
    </Modal>
  );
};

export default PosterModal;
