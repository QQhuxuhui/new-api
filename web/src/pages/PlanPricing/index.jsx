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

import React, { useState, useEffect, useMemo, useContext } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import {
  Card,
  Typography,
  Tag,
  Button,
  Space,
  Empty,
  Spin,
  Tooltip,
  Banner,
  Skeleton,
} from '@douyinfe/semi-ui';
import {
  IconTick,
  IconClock,
  IconBox,
  IconCalendarClock,
  IconBolt,
  IconCreditCard,
  IconShoppingBag,
  IconStar,
  IconGift,
  IconRefresh,
} from '@douyinfe/semi-icons';
import { API, showError } from '../../helpers';
import { StatusContext } from '../../context/Status';

const { Title, Text } = Typography;

// 检查是否已登录
const isLoggedIn = () => {
  const user = localStorage.getItem('user');
  return !!user;
};

// 格式化额度显示（不依赖 quota_per_unit 配置）
const formatQuotaDisplay = (quotaUsd, defaultQuota) => {
  // 优先使用 quota_usd（后端直接提供的美元金额）
  if (quotaUsd && quotaUsd > 0) {
    return `$${quotaUsd}`;
  }
  // 否则显示原始 tokens 数值
  if (defaultQuota && defaultQuota > 0) {
    // 格式化大数字
    if (defaultQuota >= 1000000) {
      return `${(defaultQuota / 1000000).toFixed(1)}M tokens`;
    } else if (defaultQuota >= 1000) {
      return `${(defaultQuota / 1000).toFixed(1)}K tokens`;
    }
    return `${defaultQuota} tokens`;
  }
  return null;
};

const PlanPricing = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [statusState] = useContext(StatusContext);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [plans, setPlans] = useState([]);
  const [filter, setFilter] = useState('all');

  // 从系统配置获取套餐分类配置
  const categoriesConfig = useMemo(() => {
    try {
      const config = statusState?.status?.PlanCategoriesConfig;
      if (config) {
        const parsed = typeof config === 'string' ? JSON.parse(config) : config;
        return parsed;
      }
    } catch (e) {
      // 忽略解析错误
    }
    // 默认配置
    return {
      daily: { label: t('日卡'), enabled: true },
      weekly: { label: t('周卡'), enabled: true },
      biweekly: { label: t('双周卡'), enabled: true },
      monthly: { label: t('月卡'), enabled: true },
      payg: { label: t('按量付费'), enabled: true },
    };
  }, [statusState?.status?.PlanCategoriesConfig, t]);

  // 从系统配置获取推荐套餐 ID
  const recommendedPlanId = useMemo(() => {
    try {
      // 尝试从状态中获取推荐套餐配置
      const planPricingConfig = statusState?.status?.PlanPricingConfig;
      if (planPricingConfig) {
        const config = typeof planPricingConfig === 'string'
          ? JSON.parse(planPricingConfig)
          : planPricingConfig;
        return config?.recommendedPlanId || null;
      }
    } catch (e) {
      // 忽略解析错误
    }
    return null;
  }, [statusState?.status?.PlanPricingConfig]);

  // Load enabled plans
  const loadPlans = async () => {
    setLoading(true);
    setError(null);
    try {
      // Filter by purchasable plans only
      const res = await API.get('/api/plan/enabled?purchasable=true');
      const { success, message, data } = res.data;
      if (success) {
        setPlans(data || []);
      } else {
        setError(message || t('加载失败'));
        showError(message);
      }
    } catch (e) {
      setError(e.message || t('网络错误'));
      showError(e.message);
    }
    setLoading(false);
  };

  useEffect(() => {
    loadPlans();
  }, []);

  // Filter and sort plans (先拷贝再排序，避免修改原数组)
  const filteredPlans = useMemo(() => {
    let filtered = [...plans]; // 拷贝数组

    if (filter !== 'all') {
      filtered = filtered.filter(p => p.category === filter);
    }

    // Sort by priority (higher first), then by sort_order
    return filtered.sort((a, b) => {
      if (b.priority !== a.priority) {
        return b.priority - a.priority;
      }
      return (a.sort_order || 0) - (b.sort_order || 0);
    });
  }, [plans, filter]);

  // Get plan type configuration
  const getPlanTypeConfig = (type) => {
    const typeConfig = {
      subscription: {
        color: 'blue',
        label: t('订阅套餐'),
        icon: <IconCalendarClock />,
      },
      consumption: {
        color: 'green',
        label: t('按量付费'),
        icon: <IconCreditCard />,
      },
      trial: {
        color: 'orange',
        label: t('试用套餐'),
        icon: <IconGift />,
      },
      enterprise: {
        color: 'purple',
        label: t('企业套餐'),
        icon: <IconBox />,
      },
    };
    return typeConfig[type] || { color: 'grey', label: type, icon: <IconBox /> };
  };

  // Get plan category configuration
  const getCategoryConfig = (category) => {
    // 从配置中读取分类信息
    const configCategory = categoriesConfig[category];
    if (configCategory && configCategory.enabled) {
      return {
        label: configCategory.label,
        color: getCategoryColor(category) // 保留颜色映射
      };
    }
    // 如果配置中没有或未启用，返回null（不显示此分类）
    return null;
  };

  // Get category color (保留原有颜色映射逻辑)
  const getCategoryColor = (category) => {
    const colorMap = {
      daily: 'amber',
      weekly: 'cyan',
      biweekly: 'teal',
      monthly: 'blue',
      payg: 'green',
    };
    return colorMap[category] || 'grey';
  };

  // Get price unit based on category
  const getPriceUnit = (category) => {
    const unitConfig = {
      daily: t('/天'),
      weekly: t('/周'),
      biweekly: t('/双周'),
      monthly: t('/月'),
      payg: '',
    };
    return unitConfig[category] || '';
  };

  // Extract features from plan（混合自动生成和自定义特色）
  const extractFeatures = (plan) => {
    const features = [];

    // 1. 自动生成的系统特色
    // Quota - 使用 formatQuotaDisplay 避免依赖 quota_per_unit
    const quotaDisplay = formatQuotaDisplay(plan.quota_usd, plan.default_quota);
    if (quotaDisplay) {
      features.push({ text: `${quotaDisplay} ${t('额度')}`, icon: 'check' });
    }

    // Validity
    if (plan.validity_days > 0) {
      features.push({ text: `${t('有效期')} ${plan.validity_days} ${t('天')}`, icon: 'check' });
    } else {
      features.push({ text: t('永久有效'), icon: 'check' });
    }

    // Daily limit - 使用 daily_quota_limit_usd 或直接显示数值
    if (plan.daily_quota_limit > 0) {
      const dailyDisplay = plan.daily_quota_limit_usd
        ? `$${plan.daily_quota_limit_usd}`
        : formatQuotaDisplay(null, plan.daily_quota_limit) || `${plan.daily_quota_limit}`;
      features.push({ text: `${t('每日限额')}: ${dailyDisplay}`, icon: 'check' });
    }

    // Queue slot
    if (plan.queue_slot === 0) {
      features.push({ text: t('可叠加使用'), icon: 'check' });
    }

    // Rate limits
    if (plan.rate_limit_rules && plan.rate_limit_rules !== '') {
      features.push({ text: t('含速率限制'), icon: 'info' });
    }

    // 2. 自定义特色（从 custom_features 字段读取）
    try {
      if (plan.custom_features) {
        const customFeatures = typeof plan.custom_features === 'string'
          ? JSON.parse(plan.custom_features)
          : plan.custom_features;
        if (Array.isArray(customFeatures)) {
          customFeatures.forEach(feature => {
            if (feature && feature.text && feature.text.trim() !== '') {
              features.push({
                text: feature.text,
                icon: feature.icon || 'check'
              });
            }
          });
        }
      }
    } catch (e) {
      // 忽略解析错误
    }

    return features;
  };

  // Calculate discount percentage
  const getDiscountPercent = (price, originalPrice) => {
    if (!originalPrice || originalPrice <= price) return 0;
    return Math.round((1 - price / originalPrice) * 100);
  };

  // Handle purchase click
  const handlePurchase = async (plan) => {
    // 检查是否已登录
    if (!isLoggedIn()) {
      // 未登录，跳转到登录页，登录后返回当前页面
      navigate(`/login?redirect=/plans`);
      return;
    }

    // 已登录，创建订单
    try {
      const res = await API.post('/api/user/plan/purchase/create', {
        plan_id: plan.id
      });
      const { success, message, data } = res.data;
      if (success) {
        // 订单创建成功，跳转到订单确认页面
        navigate(`/console/order-confirm/${data.order_id}`);
      } else {
        showError(message || t('创建订单失败'));
      }
    } catch (e) {
      showError(e.message || t('网络错误'));
    }
  };

  // Category filter options - 从配置生成
  const categoryOptions = useMemo(() => {
    const options = [{ key: 'all', label: t('全部') }];

    // 遍历所有分类，只添加启用的分类
    Object.keys(categoriesConfig).forEach((key) => {
      const config = categoriesConfig[key];
      if (config && config.enabled) {
        options.push({
          key: key,
          label: config.label,
        });
      }
    });

    return options;
  }, [categoriesConfig, t]);

  // Render single plan card
  const renderPlanCard = (plan) => {
    const typeConfig = getPlanTypeConfig(plan.type);
    const categoryConfig = getCategoryConfig(plan.category);

    // 如果分类配置不存在或未启用，不渲染此卡片
    if (!categoryConfig) {
      return null;
    }

    const features = extractFeatures(plan);
    const discountPercent = getDiscountPercent(plan.price, plan.original_price);
    const priceUnit = getPriceUnit(plan.category);
    // 推荐计划：优先使用配置的 recommendedPlanId，其次使用高优先级
    const isRecommended = recommendedPlanId
      ? plan.id === recommendedPlanId
      : plan.priority >= 100;

    return (
      <div
        key={plan.id}
        className={`group relative transition-all duration-300 ease-in-out transform hover:-translate-y-2 ${isRecommended ? 'z-10' : 'z-0'}`}
      >
        {/* Highlight effect for recommended plans */}
        {isRecommended && (
          <div className='absolute -inset-0.5 bg-gradient-to-r from-blue-400 to-indigo-500 rounded-2xl blur opacity-30 group-hover:opacity-50 transition-opacity duration-500'></div>
        )}

        <Card
          className={`relative h-full border-none shadow-sm hover:shadow-xl transition-shadow duration-300 ${isRecommended ? 'bg-white dark:bg-gray-800' : 'bg-gray-50 dark:bg-gray-800/50'}`}
          style={{
            borderRadius: '20px',
            overflow: 'hidden',
          }}
          bodyStyle={{ padding: '0' }}
        >
          {/* Header Section */}
          <div className={`p-6 ${isRecommended ? 'bg-gradient-to-br from-blue-500 to-indigo-600 text-white' : 'bg-gray-100 dark:bg-gray-700'}`}>
            {/* Popular Badge */}
            {isRecommended && (
              <div className='absolute top-4 right-4'>
                <Tag
                  style={{ backgroundColor: 'rgba(255,255,255,0.2)', color: 'white', borderColor: 'transparent' }}
                  shape='circle'
                >
                  <IconStar className='mr-1' />
                  {t('热门')}
                </Tag>
              </div>
            )}

            {/* Category Badge */}
            <Tag
              color={categoryConfig.color}
              size='small'
              shape='circle'
              className='mb-3'
            >
              {categoryConfig.label}
            </Tag>

            {/* Plan Name */}
            <Title
              heading={4}
              className={`m-0 mb-2 ${isRecommended ? 'text-white' : ''}`}
            >
              {plan.display_name || plan.name}
            </Title>

            {/* Plan Type */}
            <div className='flex items-center gap-1'>
              {typeConfig.icon}
              <Text className={isRecommended ? 'text-blue-100' : 'text-gray-500 dark:text-gray-400'}>
                {typeConfig.label}
              </Text>
            </div>
          </div>

          {/* Pricing Section */}
          <div className='px-6 py-5 border-b border-gray-100 dark:border-gray-700'>
            <div className='flex items-baseline gap-2'>
              {/* Discount Badge */}
              {discountPercent > 0 && (
                <Tag color='red' size='small' type='solid' shape='circle'>
                  -{discountPercent}%
                </Tag>
              )}

              {/* Original Price */}
              {discountPercent > 0 && (
                <Text delete type='tertiary' className='text-lg'>
                  ${plan.original_price}
                </Text>
              )}
            </div>

            <div className='flex items-baseline mt-1'>
              <Text className='text-3xl font-bold text-gray-900 dark:text-white'>
                ${plan.price}
              </Text>
              <Text type='tertiary' className='ml-1'>
                {priceUnit}
              </Text>
            </div>

            {/* Description */}
            {plan.description && (
              <Text type='tertiary' size='small' className='block mt-2 line-clamp-2'>
                {plan.description}
              </Text>
            )}
          </div>

          {/* Features Section */}
          <div className='px-6 py-5'>
            <div className='space-y-3'>
              {features.map((feature, index) => {
                // 根据图标类型选择不同的显示样式
                const getIconConfig = (iconType) => {
                  switch (iconType) {
                    case 'check':
                      return {
                        bg: 'bg-green-100 dark:bg-green-900/30',
                        color: 'text-green-600 dark:text-green-400',
                        icon: '✓'
                      };
                    case 'warning':
                      return {
                        bg: 'bg-orange-100 dark:bg-orange-900/30',
                        color: 'text-orange-600 dark:text-orange-400',
                        icon: '⚠'
                      };
                    case 'star':
                      return {
                        bg: 'bg-yellow-100 dark:bg-yellow-900/30',
                        color: 'text-yellow-600 dark:text-yellow-400',
                        icon: '★'
                      };
                    case 'info':
                      return {
                        bg: 'bg-blue-100 dark:bg-blue-900/30',
                        color: 'text-blue-600 dark:text-blue-400',
                        icon: 'ℹ'
                      };
                    default:
                      return {
                        bg: 'bg-green-100 dark:bg-green-900/30',
                        color: 'text-green-600 dark:text-green-400',
                        icon: '✓'
                      };
                  }
                };

                const iconConfig = getIconConfig(feature.icon);

                return (
                  <div key={index} className='flex items-center gap-2'>
                    <div className={`w-5 h-5 rounded-full ${iconConfig.bg} flex items-center justify-center flex-shrink-0`}>
                      <span className={`text-xs ${iconConfig.color}`}>{iconConfig.icon}</span>
                    </div>
                    <Text size='small' className='text-gray-600 dark:text-gray-300'>
                      {feature.text}
                    </Text>
                  </div>
                );
              })}
            </div>
          </div>

          {/* CTA Section */}
          <div className='px-6 pb-6'>
            <Button
              theme={isRecommended ? 'solid' : 'light'}
              type='primary'
              size='large'
              block
              onClick={() => handlePurchase(plan)}
              style={{
                borderRadius: '12px',
                height: '48px',
                fontWeight: 600,
              }}
            >
              {t('立即购买')}
            </Button>
          </div>
        </Card>
      </div>
    );
  };

  return (
    <div className='min-h-screen bg-[var(--semi-color-bg-0)]'>
      {/* Hero Section */}
      <div className='relative overflow-hidden bg-gradient-to-br from-blue-600 via-indigo-600 to-purple-700 py-16 px-4'>
        <div className='absolute inset-0 bg-[url("https://www.transparenttextures.com/patterns/cubes.png")] opacity-10'></div>
        <div className='absolute -bottom-20 -right-20 w-80 h-80 bg-white opacity-5 rounded-full blur-3xl'></div>
        <div className='absolute top-10 left-10 w-40 h-40 bg-purple-500 opacity-20 rounded-full blur-2xl'></div>

        <div className='max-w-6xl mx-auto text-center relative z-10'>
          <Title heading={1} className='text-white m-0 mb-4 font-bold'>
            {t('选择适合您的套餐')}
          </Title>
          <Text className='text-blue-100 text-lg block max-w-2xl mx-auto'>
            {t('灵活的定价方案，满足各种使用需求。按需选择，即买即用。')}
          </Text>
        </div>
      </div>

      {/* Main Content */}
      <div className='max-w-6xl mx-auto px-4 py-12'>
        {/* Category Filter */}
        <div className='flex flex-wrap justify-center gap-3 mb-10'>
          {categoryOptions.map(option => (
            <Button
              key={option.key}
              theme={filter === option.key ? 'solid' : 'light'}
              type={filter === option.key ? 'primary' : 'tertiary'}
              onClick={() => setFilter(option.key)}
              style={{ borderRadius: '20px' }}
            >
              {option.label}
            </Button>
          ))}
        </div>

        {/* Plans Grid */}
        {loading ? (
          // 加载骨架
          <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6'>
            {[1, 2, 3].map((i) => (
              <Card
                key={i}
                className='border-none shadow-sm'
                style={{ borderRadius: '20px', overflow: 'hidden' }}
                bodyStyle={{ padding: '0' }}
              >
                <div className='p-6 bg-gray-100 dark:bg-gray-700'>
                  <Skeleton.Title style={{ width: 60, marginBottom: 12 }} />
                  <Skeleton.Title style={{ width: '80%', marginBottom: 8 }} />
                  <Skeleton.Paragraph rows={1} style={{ width: 100 }} />
                </div>
                <div className='px-6 py-5 border-b border-gray-100 dark:border-gray-700'>
                  <Skeleton.Title style={{ width: '50%' }} />
                  <Skeleton.Paragraph rows={1} style={{ width: '70%', marginTop: 8 }} />
                </div>
                <div className='px-6 py-5'>
                  <Skeleton.Paragraph rows={3} />
                </div>
                <div className='px-6 pb-6'>
                  <Skeleton.Button style={{ width: '100%', height: 48 }} />
                </div>
              </Card>
            ))}
          </div>
        ) : error ? (
          // 错误状态
          <div className='bg-white dark:bg-gray-800 p-12 rounded-3xl shadow-sm text-center'>
            <IconBox size='extra-large' className='text-red-300 text-6xl mb-4' />
            <Title heading={4} className='mb-2'>{t('加载失败')}</Title>
            <Text type='tertiary' className='block mb-6'>{error}</Text>
            <Button
              theme='solid'
              type='primary'
              icon={<IconRefresh />}
              onClick={loadPlans}
              style={{ borderRadius: '12px' }}
            >
              {t('重试')}
            </Button>
          </div>
        ) : (() => {
          // 先过滤掉禁用分类的卡片，再判断是否为空
          const renderedCards = filteredPlans
            .map(plan => renderPlanCard(plan))
            .filter(Boolean); // 过滤掉null值（禁用分类的卡片）

          return renderedCards.length > 0 ? (
            <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6'>
              {renderedCards}
            </div>
          ) : (
            <Empty
              image={<IconBox size='extra-large' className='text-gray-300 text-6xl' />}
              title={t('暂无可用套餐')}
              description={t('目前没有可购买的套餐，请稍后再试。')}
              className='bg-white dark:bg-gray-800 p-12 rounded-3xl shadow-sm'
            />
          );
        })()}

        {/* Info Section */}
        <div className='mt-16'>
          <Banner
            type='info'
            description={
              <div className='space-y-2'>
                <Text strong className='block'>{t('关于套餐说明')}</Text>
                <ul className='list-disc list-inside text-sm space-y-1 text-gray-600 dark:text-gray-300'>
                  <li>{t('订阅套餐购买后进入队列，按顺序自动激活')}</li>
                  <li>{t('日卡可与其他套餐叠加使用，当日有效')}</li>
                  <li>{t('额度为预估值，实际扣费以使用量为准')}</li>
                  <li>{t('如有疑问，请联系管理员')}</li>
                </ul>
              </div>
            }
            className='rounded-2xl border-none'
          />
        </div>
      </div>
    </div>
  );
};

export default PlanPricing;
