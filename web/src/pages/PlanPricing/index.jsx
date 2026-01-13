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
  Collapsible,
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
  IconChevronDown,
  IconChevronUp,
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
  // Tab state: 'subscription' or 'payg'
  const initialTab = searchParams.get('category') === 'payg' ? 'payg' : 'subscription';
  const [activeTab, setActiveTab] = useState(initialTab);
  // 从 URL 参数读取初始分类，支持 ?category=payg 预选
  const [filter, setFilter] = useState(searchParams.get('category') || 'all');
  // FAQ展开状态
  const [expandedFaq, setExpandedFaq] = useState(null);

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

  // 同步标签页与过滤条件，避免进入 payg 后切换回来仍按 payg 过滤导致订阅列表为空
  useEffect(() => {
    if (activeTab === 'payg') {
      setFilter('payg');
    } else if (filter === 'payg') {
      // 仅当此前处于 payg 过滤时重置，保留其他可能的分类过滤
      setFilter('all');
    }
  }, [activeTab, filter]);

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

  // 新版套餐卡片 - 简约高级风格
  const renderNewPlanCard = (plan) => {
    const categoryConfig = getCategoryConfig(plan.category);
    if (!categoryConfig) return null;

    const features = extractFeatures(plan);
    const isRecommended = recommendedPlanId
      ? plan.id === recommendedPlanId
      : plan.priority >= 100;

    // 获取有效期显示
    const getValidityDisplay = (category) => {
      const validityMap = {
        daily: '24小时有效',
        weekly: '7天有效',
        biweekly: '14天有效',
        monthly: '30天有效',
      };
      return validityMap[category] || '';
    };

    const validity = getValidityDisplay(plan.category);

    return (
      <div
        key={plan.id}
        className='group relative transition-all duration-300 ease-in-out transform hover:-translate-y-2'
      >
        <Card
          className='relative h-full bg-white dark:bg-gray-800 border-2 border-slate-200 dark:border-gray-700 hover:border-blue-500 hover:shadow-xl hover:shadow-blue-500/10 transition-all duration-300 cursor-pointer flex flex-col'
          style={{ borderRadius: '20px', overflow: 'hidden' }}
          bodyStyle={{ padding: '32px', display: 'flex', flexDirection: 'column', height: '100%' }}
        >
          {/* Header */}
          <div className='mb-6'>
            <Title heading={4} className='m-0 mb-2 text-slate-800 dark:text-white'>
              {plan.display_name || plan.name}
            </Title>
            {validity && (
              <Tag color='blue' size='small' shape='circle' className='bg-blue-500/10 text-blue-600 border-none'>
                ⏱️ {t(validity)}
              </Tag>
            )}
          </div>

          {/* Price */}
          <div className='mb-6'>
            <span className='text-5xl font-bold text-blue-600 dark:text-blue-400'>¥{plan.price}</span>
          </div>

          {/* Quota Info Box */}
          {plan.quota_usd > 0 && (
            <div className='bg-gradient-to-br from-sky-50 to-blue-50 dark:from-blue-900/20 dark:to-sky-900/20 rounded-xl p-5 mb-5'>
              <Text className='text-slate-500 dark:text-slate-400 text-sm block mb-3 font-medium'>{t('您将获得')}</Text>
              <Text className='text-3xl font-bold text-blue-600 dark:text-blue-400 block'>
                ${plan.quota_usd} {t('美金额度')}
              </Text>
            </div>
          )}

          {/* Custom Features */}
          {features.length > 0 && (
            <div className='space-y-2 mb-6'>
              {features.map((feature, idx) => (
                <div key={idx} className='flex items-center gap-2 text-slate-600 dark:text-slate-300 text-sm'>
                  <span className='text-emerald-500 font-bold'>✓</span>
                  {feature.text}
                </div>
              ))}
            </div>
          )}

          {/* CTA Button - pushed to bottom */}
          <div className='mt-auto'>
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
                background: isRecommended ? 'linear-gradient(135deg, #2563EB, #3B82F6)' : undefined,
              }}
            >
              {t('选择') + (categoryConfig?.label || '')}
            </Button>
          </div>
        </Card>
      </div>
    );
  };

  // 新版充值卡片 - 简约高级风格
  const renderNewTopupCard = (topupAmount, index) => {
    const { value, discount } = topupAmount;
    const hasDiscount = discount < 1;
    const discountPercent = hasDiscount ? Math.round((1 - discount) * 100) : 0;
    const priceRatio = statusState?.status?.price || 7; // 从后端读取单价
    const originalPrice = value * priceRatio;
    const finalPrice = originalPrice * discount;
    // 中间的卡片标记为推荐
    const isRecommended = index === Math.floor(topupAmounts.length / 2);

    return (
      <div
        key={`topup-${value}`}
        className='group relative transition-all duration-300 ease-in-out transform hover:-translate-y-2'
      >
        <Card
          className='relative h-full bg-white dark:bg-gray-800 border-2 border-slate-200 dark:border-gray-700 hover:border-green-500 hover:shadow-xl hover:shadow-green-500/10 transition-all duration-300 cursor-pointer flex flex-col'
          style={{ borderRadius: '20px', overflow: 'hidden' }}
          bodyStyle={{ padding: '32px', display: 'flex', flexDirection: 'column', height: '100%' }}
        >
          {/* Recommended Badge */}
          {isRecommended && (
            <div className='absolute top-4 right-4'>
              <Tag color='green' size='small' shape='circle'>
                <IconStar className='mr-1' size='small' />
                {t('推荐')}
              </Tag>
            </div>
          )}

          {/* Header */}
          <div className='mb-6'>
            <Title heading={4} className='m-0 mb-2 text-slate-800 dark:text-white'>
              ${value} {t('额度')}
            </Title>
            <Tag color='green' size='small' shape='circle' className='bg-green-500/10 text-green-600 border-none'>
              💳 {t('钱包充值')}
            </Tag>
          </div>

          {/* Price */}
          <div className='mb-6'>
            <span className='text-5xl font-bold text-green-600 dark:text-green-400'>¥{finalPrice.toFixed(0)}</span>
            {hasDiscount && (
              <span className='text-lg text-slate-400 line-through ml-3'>¥{originalPrice.toFixed(0)}</span>
            )}
          </div>

          {/* Discount Badge */}
          {hasDiscount && (
            <div className='inline-flex items-center gap-1.5 bg-gradient-to-r from-red-50 to-orange-50 dark:from-red-900/20 dark:to-orange-900/20 text-red-600 dark:text-red-400 px-4 py-2 rounded-lg text-sm font-semibold mb-5 self-start'>
              🎉 {t('优惠')} {discountPercent}%
            </div>
          )}

          {/* Features */}
          <div className='space-y-2 mb-6'>
            {[
              { text: t('永不过期') },
              { text: t('按量扣费') },
              { text: t('即时到账') },
            ].map((feature, idx) => (
              <div key={idx} className='flex items-center gap-2 text-slate-600 dark:text-slate-300 text-sm'>
                <span className='text-emerald-500 font-bold'>✓</span>
                {feature.text}
              </div>
            ))}
          </div>

          {/* CTA Button - pushed to bottom */}
          <div className='mt-auto'>
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
                background: isRecommended ? 'linear-gradient(135deg, #10b981, #059669)' : undefined,
              }}
            >
              {t('立即充值')}
            </Button>
          </div>
        </Card>
      </div>
    );
  };

  return (
    <div className='min-h-screen bg-slate-50 dark:bg-gray-900'>
      {/* Hero Section - 简约高级风格 */}
      <section className='relative text-center py-20 px-6 bg-white dark:bg-gray-800 overflow-hidden'>
        {/* 背景装饰 */}
        <div className='absolute inset-0 pointer-events-none'>
          <div className='absolute top-0 left-0 w-full h-full bg-gradient-to-br from-blue-500/5 via-transparent to-blue-400/3'></div>
        </div>

        <div className='relative z-10 max-w-3xl mx-auto'>
          <Title heading={1} className='m-0 mb-4 text-slate-800 dark:text-white' style={{ fontSize: 'clamp(2.5rem, 5vw, 3.5rem)', fontWeight: 700, letterSpacing: '-0.02em' }}>
            {t('选择适合您的API服务方案')}
          </Title>
          <Text className='text-slate-500 dark:text-slate-400 text-xl block'>
            {t('两种计费方式，灵活组合使用。先用订阅套餐，用完自动切换按量付费，无缝衔接。')}
          </Text>
        </div>
      </section>

      {/* Pricing Section */}
      <section className='max-w-7xl mx-auto px-6 py-16'>
        {/* Tab Switch - 简约风格 */}
        <div className='flex justify-center mb-12'>
          <div className='inline-flex bg-white dark:bg-gray-800 rounded-2xl p-1.5 border-2 border-slate-200 dark:border-gray-700 shadow-sm'>
            <button
              onClick={() => setActiveTab('subscription')}
              className={`px-9 py-3.5 rounded-xl font-semibold text-base transition-all duration-250 flex items-center gap-2 ${
                activeTab === 'subscription'
                  ? 'bg-gradient-to-r from-blue-600 to-blue-500 text-white shadow-lg shadow-blue-500/30'
                  : 'text-slate-500 hover:text-slate-700 hover:bg-blue-50 dark:hover:bg-gray-700'
              }`}
            >
              <span className='text-xl'>📦</span>
              <span>{t('订阅套餐')}</span>
            </button>
            <button
              onClick={() => setActiveTab('payg')}
              className={`px-9 py-3.5 rounded-xl font-semibold text-base transition-all duration-250 flex items-center gap-2 ${
                activeTab === 'payg'
                  ? 'bg-gradient-to-r from-blue-600 to-blue-500 text-white shadow-lg shadow-blue-500/30'
                  : 'text-slate-500 hover:text-slate-700 hover:bg-blue-50 dark:hover:bg-gray-700'
              }`}
            >
              <span className='text-xl'>💳</span>
              <span>{t('按量付费')}</span>
            </button>
          </div>
        </div>

        {/* Subscription Tab Content */}
        {activeTab === 'subscription' && (
          <div className='animate-fadeIn'>
            <div className='text-center mb-12'>
              <Title heading={2} className='m-0 mb-4 text-slate-800 dark:text-white text-2xl font-semibold'>
                {t('订阅套餐 - 限时特惠')}
              </Title>
              <Text className='text-slate-500 dark:text-slate-400'>
                {t('承诺使用时间，享受更低价格')}
              </Text>
            </div>

            {loading ? (
              <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6'>
                {[1, 2, 3, 4].map((i) => (
                  <Card key={i} className='border-2 border-slate-200 dark:border-gray-700' style={{ borderRadius: '20px' }} bodyStyle={{ padding: 0 }}>
                    <div className='p-8'>
                      <Skeleton.Title style={{ width: 80, marginBottom: 12 }} />
                      <Skeleton.Title style={{ width: '60%', marginBottom: 24 }} />
                      <Skeleton.Paragraph rows={4} />
                    </div>
                  </Card>
                ))}
              </div>
            ) : error ? (
              <div className='bg-white dark:bg-gray-800 p-12 rounded-3xl text-center'>
                <IconBox size='extra-large' className='text-slate-300 text-6xl mb-4' />
                <Title heading={4} className='mb-2'>{t('加载失败')}</Title>
                <Text type='tertiary' className='block mb-6'>{error}</Text>
                <Button theme='solid' type='primary' icon={<IconRefresh />} onClick={loadPlans} style={{ borderRadius: '12px' }}>
                  {t('重试')}
                </Button>
              </div>
            ) : filteredPlans.length > 0 ? (
              <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6'>
                {filteredPlans.filter(p => p.category !== 'payg').map(plan => renderNewPlanCard(plan))}
              </div>
            ) : (
              <Empty
                image={<IconBox size='extra-large' className='text-slate-300 text-6xl' />}
                title={t('暂无可用套餐')}
                description={t('目前没有可购买的套餐，请稍后再试。')}
                className='bg-white dark:bg-gray-800 p-12 rounded-3xl'
              />
            )}
          </div>
        )}

        {/* Pay-as-you-go Tab Content */}
        {activeTab === 'payg' && (
          <div className='animate-fadeIn'>
            <div className='text-center mb-12'>
              <Title heading={2} className='m-0 mb-4 text-slate-800 dark:text-white text-2xl font-semibold'>
                {t('按量付费 - 钱包充值')}
              </Title>
              <Text className='text-slate-500 dark:text-slate-400'>
                {t('充值到钱包，随用随扣，余额永不过期')}
              </Text>
            </div>

            {topupLoading ? (
              <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6'>
                {[1, 2, 3].map((i) => (
                  <Card key={i} className='border-2 border-slate-200 dark:border-gray-700' style={{ borderRadius: '20px' }} bodyStyle={{ padding: 0 }}>
                    <div className='p-8'>
                      <Skeleton.Title style={{ width: 80, marginBottom: 12 }} />
                      <Skeleton.Title style={{ width: '60%', marginBottom: 24 }} />
                      <Skeleton.Paragraph rows={3} />
                    </div>
                  </Card>
                ))}
              </div>
            ) : topupAmounts.length > 0 ? (
              <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6'>
                {topupAmounts.map((amount, index) => renderNewTopupCard(amount, index))}
              </div>
            ) : (
              <Empty
                image={<IconCreditCard size='extra-large' className='text-slate-300 text-6xl' />}
                title={t('暂无充值选项')}
                description={t('管理员尚未配置充值金额选项，请联系管理员或访问钱包页面。')}
                className='bg-white dark:bg-gray-800 p-12 rounded-3xl'
              />
            )}
          </div>
        )}
      </section>

      {/* How it Works Section */}
      <section className='bg-gradient-to-br from-blue-500/5 to-blue-400/5 py-16 px-6'>
        <div className='max-w-6xl mx-auto'>
          <Title heading={2} className='text-center m-0 mb-4 text-slate-800 dark:text-white text-2xl font-semibold'>
            {t('计费方式说明')}
          </Title>
          <Text className='text-center text-slate-500 dark:text-slate-400 block mb-12'>
            {t('订阅和按量付费可以同时拥有，系统会智能选择最优惠的方式扣费')}
          </Text>

          {/* Flow Diagram */}
          <div className='flex flex-wrap items-center justify-center gap-6 mb-10'>
            <div className='bg-white dark:bg-gray-800 px-8 py-6 rounded-xl border-2 border-blue-500 bg-gradient-to-br from-blue-500/5 to-blue-400/5 min-w-[200px] text-center transition-all duration-300 hover:-translate-y-1 hover:shadow-lg hover:shadow-blue-500/10'>
              <Text className='text-slate-500 dark:text-slate-400 text-sm block mb-2'>{t('第1步')}</Text>
              <Text className='text-blue-600 dark:text-blue-400 text-xl font-bold'>{t('优先使用订阅')}</Text>
            </div>
            <span className='text-3xl text-blue-500 hidden md:block'>→</span>
            <span className='text-3xl text-blue-500 md:hidden rotate-90'>→</span>
            <div className='bg-white dark:bg-gray-800 px-8 py-6 rounded-xl border-2 border-slate-200 dark:border-gray-700 min-w-[200px] text-center transition-all duration-300 hover:-translate-y-1 hover:shadow-lg hover:border-blue-400'>
              <Text className='text-slate-500 dark:text-slate-400 text-sm block mb-2'>{t('订阅用完/过期')}</Text>
              <Text className='text-blue-600 dark:text-blue-400 text-xl font-bold'>{t('自动切换')}</Text>
            </div>
            <span className='text-3xl text-blue-500 hidden md:block'>→</span>
            <span className='text-3xl text-blue-500 md:hidden rotate-90'>→</span>
            <div className='bg-white dark:bg-gray-800 px-8 py-6 rounded-xl border-2 border-slate-200 dark:border-gray-700 min-w-[200px] text-center transition-all duration-300 hover:-translate-y-1 hover:shadow-lg hover:border-blue-400'>
              <Text className='text-slate-500 dark:text-slate-400 text-sm block mb-2'>{t('第2步')}</Text>
              <Text className='text-blue-600 dark:text-blue-400 text-xl font-bold'>{t('使用钱包余额')}</Text>
            </div>
          </div>

          {/* Comparison Cards */}
          <div className='grid grid-cols-1 md:grid-cols-2 gap-6 max-w-4xl mx-auto'>
            <Card className='border-2 border-blue-500' style={{ borderRadius: '16px' }} bodyStyle={{ padding: '32px' }}>
              <Tag color='blue' size='small' shape='circle' className='mb-3'>{t('订阅套餐')}</Tag>
              <Title heading={4} className='m-0 mb-4 text-slate-800 dark:text-white'>{t('限时优惠包')}</Title>
              <ul className='list-none p-0 m-0 space-y-2'>
                {[
                  t('有效期限制（24小时-30天）'),
                  t('价格最优惠（比按量便宜10%-30%）'),
                  t('过期后剩余额度清零'),
                  t('适合稳定高频使用'),
                ].map((item, idx) => (
                  <li key={idx} className='flex items-start gap-2 text-slate-600 dark:text-slate-300'>
                    <span className='text-emerald-500 font-bold flex-shrink-0'>✓</span>
                    <span>{item}</span>
                  </li>
                ))}
              </ul>
            </Card>

            <Card className='border-2 border-slate-200 dark:border-gray-700' style={{ borderRadius: '16px' }} bodyStyle={{ padding: '32px' }}>
              <Tag color='amber' size='small' shape='circle' className='mb-3'>{t('按量付费')}</Tag>
              <Title heading={4} className='m-0 mb-4 text-slate-800 dark:text-white'>{t('充值钱包余额')}</Title>
              <ul className='list-none p-0 m-0 space-y-2'>
                {[
                  t('永久有效，随时可用'),
                  t('标准价格（¥0.50/美金）'),
                  t('余额永不过期'),
                  t('适合偶尔使用或保底'),
                ].map((item, idx) => (
                  <li key={idx} className='flex items-start gap-2 text-slate-600 dark:text-slate-300'>
                    <span className='text-emerald-500 font-bold flex-shrink-0'>✓</span>
                    <span>{item}</span>
                  </li>
                ))}
              </ul>
            </Card>
          </div>
        </div>
      </section>

      {/* FAQ Section */}
      <section className='max-w-4xl mx-auto py-20 px-6'>
        <Title heading={2} className='text-center m-0 mb-12 text-slate-800 dark:text-white text-2xl font-semibold'>
          {t('常见问题')}
        </Title>

        <div className='space-y-4'>
          {[
            {
              q: t('订阅套餐用完后会怎样？'),
              a: t('系统会自动切换到您的钱包余额继续计费（按¥0.50/美金），不会中断服务。建议钱包保持一定余额作为保底。'),
            },
            {
              q: t('订阅套餐过期了，剩余额度还在吗？'),
              a: t('订阅套餐过期后，剩余额度会清零。所以建议根据实际使用量选择合适的套餐，避免浪费。如果用不完，可以选择更小的套餐或直接用按量付费。'),
            },
            {
              q: t('我同时购买了多个订阅套餐，怎么扣费？'),
              a: t('系统会优先使用即将过期的套餐，避免浪费。多个套餐的有效期是独立计算的，不会叠加。'),
            },
            {
              q: t('费用是怎么计算的？'),
              a: t('按照token用量和模型价格计算得出，以实际消费为准。'),
            },
          ].map((faq, idx) => (
            <div
              key={idx}
              className='bg-white dark:bg-gray-800 rounded-xl border-2 border-slate-200 dark:border-gray-700 transition-all duration-250 hover:border-blue-500 hover:shadow-md cursor-pointer'
              onClick={() => setExpandedFaq(expandedFaq === idx ? null : idx)}
            >
              <div className='p-7 flex items-center justify-between'>
                <Text className='font-semibold text-slate-800 dark:text-white flex items-center gap-2'>
                  <span>💡</span> {faq.q}
                </Text>
                {expandedFaq === idx ? <IconChevronUp className='text-slate-400' /> : <IconChevronDown className='text-slate-400' />}
              </div>
              {expandedFaq === idx && (
                <div className='px-7 pb-7 pt-0'>
                  <Text className='text-slate-500 dark:text-slate-400 leading-relaxed'>{faq.a}</Text>
                </div>
              )}
            </div>
          ))}
        </div>
      </section>

      {/* CSS for animations */}
      <style>{`
        @keyframes fadeIn {
          from { opacity: 0; transform: translateY(10px); }
          to { opacity: 1; transform: translateY(0); }
        }
        .animate-fadeIn {
          animation: fadeIn 300ms ease;
        }
      `}</style>
    </div>
  );
};

export default PlanPricing;
