import React, { useState, useEffect, useRef } from 'react';
import { Modal, Spin } from '@douyinfe/semi-ui';
import { API, showError } from '../../helpers';
import GoCaptcha from 'go-captcha-react';

const CaptchaModal = ({ visible, onSuccess, onCancel }) => {
  const [loading, setLoading] = useState(false);
  const [captchaData, setCaptchaData] = useState(null);
  const captchaRef = useRef(null);

  // 获取验证码
  const fetchCaptcha = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/captcha/get');
      if (res.data.success) {
        setCaptchaData(res.data.data);
      } else {
        showError(res.data.message || '获取验证码失败');
      }
    } catch (error) {
      showError('获取验证码失败');
    } finally {
      setLoading(false);
    }
  };

  // 验证滑动结果
  const handleConfirm = async (point, reset) => {
    if (!captchaData) return false;

    setLoading(true);
    try {
      const res = await API.post('/api/captcha/verify', {
        captcha_id: captchaData.captcha_id,
        x: Math.round(point.x),
      });

      if (res.data.success) {
        const token = res.data.data.captcha_token;
        onSuccess(token);
        return true;
      } else {
        showError(res.data.message || '验证失败，请重试');
        reset();
        // 刷新验证码
        fetchCaptcha();
        return false;
      }
    } catch (error) {
      showError('验证失败，请重试');
      reset();
      fetchCaptcha();
      return false;
    } finally {
      setLoading(false);
    }
  };

  // 刷新验证码
  const handleRefresh = () => {
    fetchCaptcha();
  };

  useEffect(() => {
    if (visible) {
      fetchCaptcha();
    }
  }, [visible]);

  return (
    <Modal
      title="安全验证"
      visible={visible}
      onCancel={onCancel}
      footer={null}
      width={400}
    >
      <Spin spinning={loading}>
        {captchaData && (
          <div style={{ padding: '20px 0' }}>
            <GoCaptcha.Slide
              config={{
                width: 320,
                height: 180,
                thumbWidth: 60,
                thumbHeight: 60,
                verticalPadding: 10,
                horizontalPadding: 10,
                showTheme: true,
                title: '请拖动滑块完成验证',
              }}
              data={{
                thumbX: 0,
                thumbY: captchaData.slider_y,
                thumbWidth: 60,
                thumbHeight: 60,
                image: captchaData.background_image,
                thumb: captchaData.slider_image,
              }}
              events={{
                confirm: handleConfirm,
                refresh: handleRefresh,
              }}
              ref={captchaRef}
            />
          </div>
        )}
      </Spin>
    </Modal>
  );
};

export default CaptchaModal;
