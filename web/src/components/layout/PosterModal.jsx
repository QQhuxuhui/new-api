import React, { useState } from 'react';
import { Modal, Button, Space } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

/**
 * PosterModal — 首页海报弹窗(替代/优先于现有公告)
 *
 * Props:
 *   visible:        bool        是否显示
 *   imageUrl:       string      海报图片 URL(OSS 公开 URL 或外链)
 *   clickUrl:       string      点击跳转 URL,空时图片不可点击
 *   onClose:        () => void  普通关闭(下次刷新仍会弹,与公告 NoticeModal 行为一致)
 *   onCloseToday:   () => void  "今日不再弹" 关闭(写 localStorage 当天不再弹)
 *   onImageError:   () => void  海报图片加载失败回调(让父组件 fallback 到公告)
 *
 * 行为:
 *   - imageUrl 非空时显示大图
 *   - clickUrl 非空时整张图包 <a target=_blank rel=noopener>
 *   - <img onError> 同时调 onImageError 通知父组件降级
 *   - 图下方两个按钮:[关闭] [今日不再弹](对齐公告 NoticeModal 双关闭模型)
 *   - 点 X / 遮罩 / Esc → 普通关闭(等价"关闭"按钮)
 */
const PosterModal = ({
  visible,
  imageUrl,
  clickUrl,
  onClose,
  onCloseToday,
  onImageError,
}) => {
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
        maxHeight: '70vh',
        margin: '0 auto',
        borderRadius: 8,
      }}
      onError={() => {
        setImgFailed(true);
        if (typeof onImageError === 'function') onImageError();
      }}
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
          <div style={{ padding: 24, color: '#999', fontSize: 14 }}>
            {t('海报图片加载失败')}
          </div>
        )}

        <div
          style={{
            padding: '12px 16px 4px',
            display: 'flex',
            justifyContent: 'center',
            gap: 12,
          }}
        >
          <Space>
            <Button
              type='secondary'
              onClick={() => {
                if (typeof onCloseToday === 'function') {
                  onCloseToday();
                } else {
                  onClose && onClose();
                }
              }}
            >
              {t('今日不再弹')}
            </Button>
            <Button type='tertiary' onClick={onClose}>
              {t('关闭')}
            </Button>
          </Space>
        </div>
      </div>
    </Modal>
  );
};

export default PosterModal;
