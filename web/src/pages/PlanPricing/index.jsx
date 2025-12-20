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
import { useNavigate, useSearchParams } from 'react-router-dom';
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
import { API, showError, convertUSDToCurrency } from '../../helpers';
import { StatusContext } from '../../context/Status';

const { Title, Text } = Typography;

// 检查是否已登录
const isLoggedIn = () => {
  const user = localStorage.getItem('user');
  return !!user;
};

// 格式化额度显示（遵循系统货币配置）
const formatQuotaDisplay = (quotaUsd, defaultQuota) => {
  // 优先使用 quota_usd（后端直接提供的美元金额）
  if (quotaUsd && quotaUsd > 0) {
    // 使用系统的货币配置转换美元金额
    return convertUSDToCurrency(quotaUsd);
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
  const [searchParams] = useSearchParams();
  const [statusState] = useContext(StatusContext);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [plans, setPlans] = useState([]);
  // 从 URL 参数读取初始分类，支持 ?category=payg 预选
  const [filter, setFilter] = useState(searchParams.get('category') || 'all');

  // 按量付费（充值）相关状态
  const [topupLoading, setTopupLoading] = useState(false);
  const [topupInfo, setTopupInfo] = useState(null);
  const [topupAmounts, setTopupAmounts] = useState([]);
  const [creatingOrder, setCreatingOrder] = useState(false);

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

  // Load topup info (pay-as-you-go amounts)
  const loadTopupInfo = async () => {
    if (topupInfo) return; // Already loaded
    setTopupLoading(true);
    try {
      const res = await API.get('/api/user/topup/info');
      const { success, data } = res.data;
      if (success) {
        setTopupInfo(data);
        // Build topup amounts from amount_options and discount
        const amounts = (data.amount_options || []).map((amount) => ({
          value: amount,
          discount: data.discount?.[amount] || 1.0,
        }));
        // If no custom amounts, generate defaults
        if (amounts.length === 0) {
          const minTopup = data.min_topup || 1;
          const multipliers = [1, 5, 10, 30, 50, 100];
          amounts.push(...multipliers.map((m) => ({
            value: minTopup * m,
            discount: 1.0,
          })));
        }
        setTopupAmounts(amounts);
      }
    } catch (e) {
      console.error('Failed to load topup info:', e);
    }
    setTopupLoading(false);
  };

  // Handle topup purchase (create order and redirect)
  const handleTopupPurchase = async (amount) => {
    // Check login
    if (!isLoggedIn()) {
      navigate(`/login?redirect=/plans?category=payg`);
      return;
    }

    setCreatingOrder(true);
    try {
      const res = await API.post('/api/user/topup/order/create', {
        amount: amount,
      });
      const { success, message, data } = res.data;
      if (success) {
        // Redirect to order confirmation page with topup type
        navigate(`/console/order-confirm/${data.order_id}?type=topup`);
      } else {
        showError(message || t('创建订单失败'));
      }
    } catch (e) {
      showError(e.message || t('网络错误'));
    }
    setCreatingOrder(false);
  };

  useEffect(() => {
    loadPlans();
  }, []);

  // Load topup info when filter changes to payg
  useEffect(() => {
    if (filter === 'payg' && !topupInfo) {
      loadTopupInfo();
    }
  }, [filter]);

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

  // Extract features from plan（只显示自定义特色）
  const extractFeatures = (plan) => {
    const features = [];

    // 只显示自定义特色（从 custom_features 字段读取）
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

  // Render single topup amount card (pay-as-you-go)
  const renderTopupCard = (topupAmount, index) => {
    const { value, discount } = topupAmount;
    const hasDiscount = discount < 1;
    const discountPercent = hasDiscount ? Math.round((1 - discount) * 100) : 0;
    const priceRatio = statusState?.status?.price || 7; // Default CNY/USD ratio
    const originalPrice = value * priceRatio;
    const finalPrice = originalPrice * discount;
    // Mark the middle card as recommended
    const isRecommended = index === Math.floor(topupAmounts.length / 2);

    return (
      <div
        key={`topup-${value}`}
        className={`group relative transition-all duration-300 ease-in-out transform hover:-translate-y-2 ${isRecommended ? 'z-10' : 'z-0'}`}
      >
        {/* Highlight effect for recommended */}
        {isRecommended && (
          <div className='absolute -inset-0.5 bg-gradient-to-r from-green-400 to-emerald-500 rounded-2xl blur opacity-30 group-hover:opacity-50 transition-opacity duration-500'></div>
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
          <div className={`p-6 ${isRecommended ? 'bg-gradient-to-br from-green-500 to-emerald-600 text-white' : 'bg-gray-100 dark:bg-gray-700'}`}>
            {/* Popular Badge */}
            {isRecommended && (
              <div className='absolute top-4 right-4'>
                <Tag
                  style={{ backgroundColor: 'rgba(255,255,255,0.2)', color: 'white', borderColor: 'transparent' }}
                  shape='circle'
                >
                  <IconStar className='mr-1' />
                  {t('推荐')}
                </Tag>
              </div>
            )}

            {/* Category Badge */}
            <Tag
              color='green'
              size='small'
              shape='circle'
              className='mb-3'
            >
              {t('按量付费')}
            </Tag>

            {/* Amount Title */}
            <Title
              heading={4}
              className={`m-0 mb-2 ${isRecommended ? 'text-white' : ''}`}
            >
              ${value} {t('额度')}
            </Title>

            {/* Type */}
            <div className='flex items-center gap-1'>
              <IconCreditCard />
              <Text className={isRecommended ? 'text-green-100' : 'text-gray-500 dark:text-gray-400'}>
                {t('钱包充值')}
              </Text>
            </div>
          </div>

          {/* Pricing Section - Split Layout */}
          <div className='flex items-center justify-between px-6 py-8 border-b border-gray-100 dark:border-gray-700'>
            {/* Price Side */}
            <div className='flex-1 text-center border-r border-gray-100 dark:border-gray-700 pr-2'>
              {hasDiscount && (
                  <Tag color='red' size='small' type='solid' shape='circle' className='mb-1'>
                    -{discountPercent}%
                  </Tag>
              )}
              <div className='text-gray-500 text-xs mb-1'>{t('支付金额')}</div>
              <div className='text-3xl font-bold text-blue-600 dark:text-blue-400'>
                <span className='text-lg mr-0.5'>¥</span>
                {finalPrice.toFixed(0)}
              </div>
              {hasDiscount && (
                  <div className='text-xs text-gray-400 line-through'>¥{originalPrice.toFixed(0)}</div>
              )}
            </div>

            {/* Quota Side */}
            <div className='flex-1 text-center pl-2'>
               <div className='text-gray-500 text-xs mb-1'>{t('获得额度')}</div>
               <div className='text-3xl font-bold text-green-600 dark:text-green-400'>
                 <span className='text-lg mr-0.5'>$</span>
                 {value}
               </div>
               <div className='text-xs text-gray-400'>{t('美金')}</div>
            </div>
          </div>

          <div className='px-6 pt-2 pb-0'>
             <Text type='tertiary' size='small' className='block text-center'>
              {t('充值后永不过期，按实际使用量扣费')}
            </Text>
          </div>

          {/* Features Section */}
          <div className='px-6 py-5'>
            <div className='space-y-3'>
              {[
                { text: t('永不过期'), icon: 'check' },
                { text: t('按量扣费'), icon: 'check' },
                { text: t('即时到账'), icon: 'check' },
              ].map((feature, idx) => (
                <div key={idx} className='flex items-center gap-2'>
                  <div className='w-5 h-5 rounded-full bg-green-100 dark:bg-green-900/30 flex items-center justify-center flex-shrink-0'>
                    <span className='text-xs text-green-600 dark:text-green-400'>✓</span>
                  </div>
                  <Text size='small' className='text-gray-600 dark:text-gray-300'>
                    {feature.text}
                  </Text>
                </div>
              ))}
            </div>
          </div>

          {/* CTA Section */}
          <div className='px-6 pb-6'>
            <Button
              theme={isRecommended ? 'solid' : 'light'}
              type='primary'
              size='large'
              block
              loading={creatingOrder}
              onClick={() => handleTopupPurchase(value)}
              style={{
                borderRadius: '12px',
                height: '48px',
                fontWeight: 600,
                backgroundColor: isRecommended ? '#10b981' : undefined,
              }}
            >
              {t('立即充值')}
            </Button>
          </div>
        </Card>
      </div>
    );
  };

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

          {/* Pricing Section - Split Layout */}
          <div className='flex items-center justify-between px-6 py-8 border-b border-gray-100 dark:border-gray-700'>
            {/* Price Side */}
            <div className='flex-1 text-center border-r border-gray-100 dark:border-gray-700 pr-2'>
              {discountPercent > 0 && (
                  <Tag color='red' size='small' type='solid' shape='circle' className='mb-1'>
                    -{discountPercent}%
                  </Tag>
              )}
              <div className='text-gray-500 text-xs mb-1'>{t('价格')}</div>
              <div className='text-3xl font-bold text-blue-600 dark:text-blue-400'>
                <span className='text-lg mr-0.5'>¥</span>
                {plan.price}
              </div>
              {priceUnit && <div className='text-xs text-gray-400'>{priceUnit}</div>}
            </div>

            {/* Quota Side */}
            <div className='flex-1 text-center pl-2'>
               <div className='text-gray-500 text-xs mb-1'>{t('额度')}</div>
               <div className='text-3xl font-bold text-green-600 dark:text-green-400'>
                 {(() => {
                   if (plan.quota_usd > 0) {
                     return `$${plan.quota_usd}`;
                   }
                   return formatQuotaDisplay(plan.quota_usd, plan.default_quota) || '—';
                 })()}
               </div>
               <div className='text-xs text-gray-400'>
                 {plan.quota_usd > 0 ? t('美金') : ''}
               </div>
            </div>
          </div>

          {/* Description */}
          {plan.description && (
            <div className='px-6 pt-2 pb-0 text-center'>
               <Text type='tertiary' size='small' className='line-clamp-2'>
                  {plan.description}
               </Text>
            </div>
          )}

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
        {loading || (filter === 'payg' && topupLoading) ? (
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
        ) : filter === 'payg' ? (
          // 按量付费 - 显示充值金额卡片
          topupAmounts.length > 0 ? (
            <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6'>
              {topupAmounts.map((amount, index) => renderTopupCard(amount, index))}
            </div>
          ) : (
            <Empty
              image={<IconCreditCard size='extra-large' className='text-gray-300 text-6xl' />}
              title={t('暂无充值选项')}
              description={t('管理员尚未配置充值金额选项，请联系管理员或访问钱包页面。')}
              className='bg-white dark:bg-gray-800 p-12 rounded-3xl shadow-sm'
            />
          )
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
