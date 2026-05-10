import React, { useEffect, useState, useRef } from 'react';
import {
  Card,
  Form,
  Button,
  Switch,
  Input,
  Typography,
  Toast,
  Upload,
  Space,
  Spin,
  Banner,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../../helpers/api';
import { showError, showSuccess } from '../../../helpers';

const { Title, Text } = Typography;

const POSTER_KEYS = [
  'OSSAccessKeyId',
  'OSSAccessKeySecret',
  'OSSEndpoint',
  'OSSBucket',
  'PosterImageUrl',
  'PosterClickUrl',
  'EnablePoster',
];

const SettingsPoster = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [config, setConfig] = useState({
    OSSAccessKeyId: '',
    OSSAccessKeySecret: '',
    OSSEndpoint: '',
    OSSBucket: '',
    PosterImageUrl: '',
    PosterClickUrl: '',
    EnablePoster: false,
  });
  const formApiRef = useRef(null);

  // 加载配置
  const loadConfig = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/option/');
      const { success, data } = res.data || {};
      if (!success) {
        showError(t('加载海报配置失败'));
        return;
      }
      const next = { ...config };
      data.forEach((item) => {
        if (POSTER_KEYS.includes(item.key)) {
          if (item.key === 'EnablePoster') {
            next[item.key] = item.value === 'true' || item.value === true;
          } else {
            next[item.key] = item.value || '';
          }
        }
      });
      setConfig(next);
      formApiRef.current?.setValues(next);
    } catch (e) {
      showError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadConfig();
    // eslint-disable-next-line
  }, []);

  // 单 key PUT 更新
  const updateOne = async (key, value) => {
    return API.put('/api/option/', { key, value });
  };

  // 批量保存
  const handleSubmit = async (values) => {
    setLoading(true);
    try {
      // 1. 把 EnablePoster 转 string
      const payload = { ...values };
      payload.EnablePoster = String(!!payload.EnablePoster);

      // 2. 顺序逐个更新
      for (const k of POSTER_KEYS) {
        let v = payload[k];
        // OSSAccessKeySecret:如果用户没改(仍为占位 ***),前端不发送以免后端覆盖
        // (后端也有相同保护,但前端跳过更直观)
        if (k === 'OSSAccessKeySecret' && v === '***') {
          continue;
        }
        await updateOne(k, v == null ? '' : String(v));
      }
      showSuccess(t('海报配置已保存'));
      await loadConfig();
    } catch (e) {
      showError(e?.response?.data?.message || e.message);
    } finally {
      setLoading(false);
    }
  };

  // 上传海报
  const handleUpload = async ({ fileInstance, onSuccess, onError }) => {
    if (!fileInstance) {
      onError && onError(new Error(t('未选择文件')));
      return;
    }
    setUploading(true);
    try {
      const formData = new FormData();
      formData.append('file', fileInstance);
      const res = await API.post('/api/option/poster/upload', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      const { success, data, message } = res.data || {};
      if (success && data?.url) {
        Toast.success(t('上传成功'));
        // 回填到表单
        formApiRef.current?.setValue('PosterImageUrl', data.url);
        onSuccess && onSuccess(data);
      } else {
        Toast.error(message || t('上传失败'));
        onError && onError(new Error(message || 'upload failed'));
      }
    } catch (e) {
      const msg = e?.response?.data?.message || e.message;
      Toast.error(msg);
      onError && onError(e);
    } finally {
      setUploading(false);
    }
  };

  return (
    <Spin spinning={loading}>
      <div style={{ marginBottom: 12 }}>
        <Title heading={5}>{t('海报弹窗')}</Title>
        <Text type='tertiary' style={{ fontSize: 13 }}>
          {t(
            '首页加载时弹出海报图片(优先于系统公告)。图片存储在阿里云 OSS,可点击跳转到自定义链接。',
          )}
        </Text>
      </div>

      <Banner
        fullMode={false}
        type='info'
        description={t(
          '提示:OSSAccessKeySecret 已保存时显示为 ***,留作占位不修改。重新填入新值才会覆盖。请勿把字面量 *** 设为真实 Secret。',
        )}
        closeIcon={null}
        style={{ marginBottom: 12 }}
      />

      <Form
        getFormApi={(api) => (formApiRef.current = api)}
        initValues={config}
        onSubmit={handleSubmit}
      >
        <Card title={t('阿里云 OSS 凭证')} style={{ marginBottom: 12 }}>
          <Form.Input
            field='OSSAccessKeyId'
            label='OSSAccessKeyId'
            placeholder={t('阿里云 RAM 子账号 AccessKey ID')}
          />
          <Form.Input
            field='OSSAccessKeySecret'
            label='OSSAccessKeySecret'
            mode='password'
            placeholder={t('已保存时显示 ***,留空或保留 *** 表示不修改')}
            extraText={t('字面量 *** 会被识别为占位符,不会被保存')}
          />
          <Form.Input
            field='OSSEndpoint'
            label='OSSEndpoint'
            placeholder='oss-cn-shanghai.aliyuncs.com'
          />
          <Form.Input
            field='OSSBucket'
            label='OSSBucket'
            placeholder={t('bucket 名(不含 oss:// 或 https:// 前缀)')}
          />
        </Card>

        <Card title={t('海报内容')}>
          <Form.Slot label={t('上传海报图片')}>
            <Space>
              <Upload
                action=''
                customRequest={handleUpload}
                showUploadList={false}
                accept='image/jpeg,image/png,image/webp,image/gif'
              >
                <Button loading={uploading}>{t('点击选择图片')}</Button>
              </Upload>
              <Text type='tertiary' style={{ fontSize: 12 }}>
                {t('JPG/PNG/WebP/GIF, ≤ 5MB,上传后 URL 自动填入下方,记得点保存')}
              </Text>
            </Space>
          </Form.Slot>

          <Form.Input
            field='PosterImageUrl'
            label={t('海报图片 URL')}
            placeholder='https://your-bucket.oss-cn-shanghai.aliyuncs.com/posters/poster_xxx.jpg'
            extraText={t('上传后自动填入,也可手动填外链;清空可关闭海报')}
          />
          <Form.Input
            field='PosterClickUrl'
            label={t('点击跳转 URL(可选)')}
            placeholder='https://mp.weixin.qq.com/...'
            extraText={t('留空时图片不可点击;非空时整张图包链接,新标签页打开')}
          />
          <Form.Switch
            field='EnablePoster'
            label={t('启用海报弹窗')}
            extraText={t('关闭后立即恢复到现有系统公告弹窗')}
          />

          {/* 实时预览 */}
          <Form.Slot label={t('实时预览')}>
            <PosterPreview formApiRef={formApiRef} />
          </Form.Slot>
        </Card>

        <div style={{ marginTop: 16 }}>
          <Button type='primary' theme='solid' htmlType='submit' loading={loading}>
            {t('保存海报设置')}
          </Button>
        </div>
      </Form>
    </Spin>
  );
};

// PosterPreview 实时显示当前 PosterImageUrl 的图片(独立组件以使用 form values 订阅)
const PosterPreview = ({ formApiRef }) => {
  const { t } = useTranslation();
  const [url, setUrl] = useState('');

  useEffect(() => {
    const tick = () => {
      const v = formApiRef.current?.getValue('PosterImageUrl') || '';
      setUrl(v);
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
    // eslint-disable-next-line
  }, []);

  if (!url) {
    return (
      <Text type='tertiary' style={{ fontSize: 12 }}>
        {t('当前未设置海报图片')}
      </Text>
    );
  }
  return (
    <img
      src={url}
      alt={t('海报预览')}
      style={{
        maxWidth: 320,
        maxHeight: 320,
        border: '1px solid #eee',
        borderRadius: 8,
      }}
      onError={(e) => {
        e.currentTarget.style.display = 'none';
      }}
    />
  );
};

export default SettingsPoster;
