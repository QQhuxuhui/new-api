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
  Empty,
  Skeleton,
} from '@douyinfe/semi-ui';
import {
  IconBox,
  IconCalendarClock,
  IconCreditCard,
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
        label: t('包月套餐'),
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

    // 计算节省百分比
    const discountPercent = getDiscountPercent(plan.price, plan.original_price);

    return (
      <div
        key={plan.id}
        className='group relative cursor-pointer'
        style={{
          transition: 'all 300ms cubic-bezier(0.4, 0, 0.2, 1)',
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.transform = 'translateY(-8px)';
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.transform = 'translateY(0)';
        }}
      >
        <Card
          className='relative h-full bg-white dark:bg-gray-800 flex flex-col'
          style={{
            borderRadius: '20px',
            overflow: 'hidden',
            border: '2px solid #E2E8F0',
            transition: 'all 300ms cubic-bezier(0.4, 0, 0.2, 1)',
          }}
          bodyStyle={{ padding: '32px', display: 'flex', flexDirection: 'column', height: '100%' }}
          onMouseEnter={(e) => {
            e.currentTarget.style.borderColor = '#2563EB';
            e.currentTarget.style.boxShadow = '0 20px 60px rgba(37, 99, 235, 0.15)';
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.borderColor = '#E2E8F0';
            e.currentTarget.style.boxShadow = 'none';
          }}
        >
          {/* Header */}
          <div className='mb-6'>
            <Title
              heading={4}
              className='m-0 mb-2'
              style={{ color: '#1E293B', fontFamily: "'Poppins', 'Inter', system-ui, sans-serif" }}
            >
              {plan.display_name || plan.name}
            </Title>
            {validity && (
              <span
                className='inline-block px-3 py-1 rounded-xl text-sm font-semibold'
                style={{
                  background: 'rgba(37, 99, 235, 0.1)',
                  color: '#2563EB',
                }}
              >
                ⏱️ {t(validity)}
              </span>
            )}
          </div>

          {/* Price */}
          <div className='mb-6'>
            <span
              className='price-animated'
              style={{
                fontSize: '3rem',
                fontWeight: 700,
                color: '#2563EB',
                fontFamily: "'Poppins', 'Inter', system-ui, sans-serif",
                lineHeight: 1,
              }}
            >
              ¥{plan.price}
            </span>
          </div>

          {/* Quota Info Box */}
          {plan.quota_usd > 0 && (
            <div
              className='rounded-xl p-5 mb-5'
              style={{
                background: 'linear-gradient(135deg, #F0F9FF, #E0F2FE)',
              }}
            >
              <Text className='text-sm block mb-3 font-medium' style={{ color: '#475569' }}>
                {t('您将获得')}
              </Text>
              <Text
                className='price-animated block'
                style={{
                  fontSize: '1.75rem',
                  fontWeight: 700,
                  color: '#2563EB',
                  fontFamily: "'Poppins', 'Inter', system-ui, sans-serif",
                }}
              >
                ${plan.quota_usd}
              </Text>
              <Text className='text-sm' style={{ color: '#475569' }}>
                {t('美金额度')}
              </Text>
            </div>
          )}

          {/* Savings Badge */}
          {discountPercent > 0 && (
            <div
              className='inline-flex items-center gap-1.5 px-4 py-2 rounded-lg text-sm font-semibold mb-5 self-start'
              style={{
                background: 'linear-gradient(135deg, #ECFDF5, #D1FAE5)',
                color: '#065F46',
              }}
            >
              💰 {t('相比按量付费节省')} <span style={{ fontSize: '1.125rem', color: '#10B981' }}>{discountPercent}%</span>
            </div>
          )}

          {/* Custom Features - 从后台配置读取 */}
          {features.length > 0 && (
            <div
              className='rounded-lg p-4 mb-6'
              style={{ background: '#F8FAFC', color: '#475569' }}
            >
              <div className='space-y-2'>
                {features.map((feature, idx) => (
                  <div key={idx} className='flex items-center gap-2 text-sm'>
                    <span style={{ color: '#10B981', fontWeight: 700 }}>✓</span>
                    {feature.text}
                  </div>
                ))}
              </div>
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
              className='cursor-pointer'
              style={{
                borderRadius: '12px',
                height: '48px',
                fontWeight: 600,
                fontFamily: "'Poppins', 'Inter', system-ui, sans-serif",
                transition: 'all 250ms cubic-bezier(0.4, 0, 0.2, 1)',
                ...(isRecommended
                  ? {
                      background: 'linear-gradient(135deg, #2563EB, #3B82F6)',
                      border: 'none',
                    }
                  : {
                      background: 'white',
                      color: '#2563EB',
                      border: '2px solid #2563EB',
                    }),
              }}
              onMouseEnter={(e) => {
                if (isRecommended) {
                  e.currentTarget.style.boxShadow = '0 8px 24px rgba(37, 99, 235, 0.35)';
                  e.currentTarget.style.transform = 'translateY(-2px)';
                  e.currentTarget.style.background = 'linear-gradient(135deg, #1D4ED8, #2563EB)';
                } else {
                  e.currentTarget.style.background = 'rgba(37, 99, 235, 0.08)';
                  e.currentTarget.style.borderColor = '#1D4ED8';
                  e.currentTarget.style.transform = 'translateY(-2px)';
                }
              }}
              onMouseLeave={(e) => {
                if (isRecommended) {
                  e.currentTarget.style.boxShadow = 'none';
                  e.currentTarget.style.transform = 'translateY(0)';
                  e.currentTarget.style.background = 'linear-gradient(135deg, #2563EB, #3B82F6)';
                } else {
                  e.currentTarget.style.background = 'white';
                  e.currentTarget.style.borderColor = '#2563EB';
                  e.currentTarget.style.transform = 'translateY(0)';
                }
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
        className='group relative cursor-pointer'
        style={{
          transition: 'all 300ms cubic-bezier(0.4, 0, 0.2, 1)',
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.transform = 'translateY(-8px)';
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.transform = 'translateY(0)';
        }}
      >
        <Card
          className='relative h-full bg-white dark:bg-gray-800 flex flex-col'
          style={{
            borderRadius: '20px',
            overflow: 'hidden',
            border: '2px solid #E2E8F0',
            transition: 'all 300ms cubic-bezier(0.4, 0, 0.2, 1)',
          }}
          bodyStyle={{ padding: '32px', display: 'flex', flexDirection: 'column', height: '100%' }}
          onMouseEnter={(e) => {
            e.currentTarget.style.borderColor = '#2563EB';
            e.currentTarget.style.boxShadow = '0 20px 60px rgba(37, 99, 235, 0.15)';
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.borderColor = '#E2E8F0';
            e.currentTarget.style.boxShadow = 'none';
          }}
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
            <span
              className='inline-block px-3 py-1 rounded-xl text-sm font-semibold'
              style={{
                background: 'rgba(16, 185, 129, 0.1)',
                color: '#10B981',
              }}
            >
              💳 {t('钱包充值')}
            </span>
          </div>

          {/* Price */}
          <div className='mb-6'>
            <span
              className='price-animated'
              style={{
                fontSize: '3rem',
                fontWeight: 700,
                color: '#F59E0B',
                fontFamily: "'Poppins', 'Inter', system-ui, sans-serif",
                lineHeight: 1,
              }}
            >
              ¥{finalPrice.toFixed(0)}
            </span>
            {hasDiscount && (
              <span className='text-lg ml-3 line-through' style={{ color: '#94A3B8' }}>
                ¥{originalPrice.toFixed(0)}
              </span>
            )}
          </div>

          {/* Discount Badge */}
          {hasDiscount && (
            <div
              className='inline-flex items-center gap-1.5 px-4 py-2 rounded-lg text-sm font-semibold mb-5 self-start'
              style={{
                background: 'linear-gradient(135deg, #FEF3C7, #FDE68A)',
                color: '#92400E',
              }}
            >
              🎉 {t('优惠')} {discountPercent}%
            </div>
          )}

          {/* Quota Info - 美金额度显示在下方 */}
          <div
            className='rounded-xl p-5 mb-6'
            style={{
              background: 'linear-gradient(135deg, #F0F9FF, #E0F2FE)',
            }}
          >
            <Text className='text-sm block mb-2 font-medium' style={{ color: '#475569' }}>
              {t('获得额度')}
            </Text>
            <Text
              className='price-animated block'
              style={{
                fontSize: '1.75rem',
                fontWeight: 700,
                color: '#2563EB',
                fontFamily: "'Poppins', 'Inter', system-ui, sans-serif",
              }}
            >
              ${value}
            </Text>
            <Text className='text-sm' style={{ color: '#475569' }}>
              {t('美金')}
            </Text>
          </div>

          {/* CTA Button - pushed to bottom */}
          <div className='mt-auto'>
            <Button
              theme='solid'
              type='primary'
              size='large'
              block
              loading={creatingOrder}
              onClick={() => handleTopupPurchase(value)}
              className='cursor-pointer'
              style={{
                borderRadius: '12px',
                height: '48px',
                fontWeight: 600,
                fontFamily: "'Poppins', 'Inter', system-ui, sans-serif",
                background: 'linear-gradient(135deg, #2563EB, #3B82F6)',
                border: 'none',
                transition: 'all 250ms cubic-bezier(0.4, 0, 0.2, 1)',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.boxShadow = '0 8px 24px rgba(37, 99, 235, 0.35)';
                e.currentTarget.style.transform = 'translateY(-2px)';
                e.currentTarget.style.background = 'linear-gradient(135deg, #1D4ED8, #2563EB)';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.boxShadow = 'none';
                e.currentTarget.style.transform = 'translateY(0)';
                e.currentTarget.style.background = 'linear-gradient(135deg, #2563EB, #3B82F6)';
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
    <div className='plan-pricing-page min-h-screen bg-[#F8FAFC] dark:bg-gray-900'>
      {/* Hero Section - 简约高级风格 */}
      <section className='relative text-center py-20 px-6 bg-white dark:bg-gray-800 overflow-hidden'>
        {/* 背景装饰 - 渐变光晕效果 */}
        <div className='absolute inset-0 pointer-events-none overflow-hidden'>
          <div
            className='absolute w-[200%] h-[200%]'
            style={{
              top: '-50%',
              left: '-50%',
              background: 'radial-gradient(circle at 30% 20%, rgba(37, 99, 235, 0.08) 0%, transparent 50%), radial-gradient(circle at 70% 80%, rgba(59, 130, 246, 0.04) 0%, transparent 50%)',
            }}
          ></div>
        </div>

        <div className='relative z-10 max-w-3xl mx-auto'>
          <Title
            heading={1}
            className='m-0 mb-4 text-[#1E293B] dark:text-white'
            style={{
              fontSize: 'clamp(2.5rem, 5vw, 3.5rem)',
              fontWeight: 700,
              letterSpacing: '-0.02em',
              fontFamily: "'Poppins', 'Inter', system-ui, sans-serif",
            }}
          >
            {t('选择适合您的API服务方案')}
          </Title>
          <Text
            className='text-[#475569] dark:text-slate-400 block max-w-xl mx-auto'
            style={{ fontSize: '1.25rem', lineHeight: 1.6, marginTop: '20px' }}
          >
            {t('两种计费方式，灵活组合使用。先用包月套餐，用完自动切换按量付费，无缝衔接。')}
          </Text>
        </div>
      </section>

      {/* Pricing Section */}
      <section className='max-w-7xl mx-auto px-6 py-16'>
        {/* Tab Switch - 简约高级风格 */}
        <div className='flex justify-center mb-12'>
          <div
            className='inline-flex bg-white dark:bg-gray-800 rounded-2xl p-1.5 shadow-sm'
            style={{
              border: '2px solid #E2E8F0',
              boxShadow: '0 4px 12px rgba(0, 0, 0, 0.05)',
            }}
          >
            <button
              onClick={() => setActiveTab('subscription')}
              className='cursor-pointer px-9 py-3.5 rounded-xl font-semibold text-base flex items-center gap-2 focus:outline-none focus-visible:ring-2 focus-visible:ring-[#2563EB] focus-visible:ring-offset-2'
              style={{
                transition: 'all 250ms cubic-bezier(0.4, 0, 0.2, 1)',
                fontFamily: "'Poppins', 'Inter', system-ui, sans-serif",
                ...(activeTab === 'subscription'
                  ? {
                      background: 'linear-gradient(135deg, #2563EB, #3B82F6)',
                      color: 'white',
                      boxShadow: '0 4px 16px rgba(37, 99, 235, 0.3)',
                    }
                  : {
                      color: '#475569',
                      background: 'transparent',
                    }),
              }}
              onMouseEnter={(e) => {
                if (activeTab !== 'subscription') {
                  e.currentTarget.style.color = '#1E293B';
                  e.currentTarget.style.background = 'rgba(37, 99, 235, 0.05)';
                }
              }}
              onMouseLeave={(e) => {
                if (activeTab !== 'subscription') {
                  e.currentTarget.style.color = '#475569';
                  e.currentTarget.style.background = 'transparent';
                }
              }}
            >
              <IconBox size='large' />
              <span>{t('包月套餐')}</span>
            </button>
            <button
              onClick={() => setActiveTab('payg')}
              className='cursor-pointer px-9 py-3.5 rounded-xl font-semibold text-base flex items-center gap-2 focus:outline-none focus-visible:ring-2 focus-visible:ring-[#2563EB] focus-visible:ring-offset-2'
              style={{
                transition: 'all 250ms cubic-bezier(0.4, 0, 0.2, 1)',
                fontFamily: "'Poppins', 'Inter', system-ui, sans-serif",
                ...(activeTab === 'payg'
                  ? {
                      background: 'linear-gradient(135deg, #2563EB, #3B82F6)',
                      color: 'white',
                      boxShadow: '0 4px 16px rgba(37, 99, 235, 0.3)',
                    }
                  : {
                      color: '#475569',
                      background: 'transparent',
                    }),
              }}
              onMouseEnter={(e) => {
                if (activeTab !== 'payg') {
                  e.currentTarget.style.color = '#1E293B';
                  e.currentTarget.style.background = 'rgba(37, 99, 235, 0.05)';
                }
              }}
              onMouseLeave={(e) => {
                if (activeTab !== 'payg') {
                  e.currentTarget.style.color = '#475569';
                  e.currentTarget.style.background = 'transparent';
                }
              }}
            >
              <IconCreditCard size='large' />
              <span>{t('按量付费')}</span>
            </button>
          </div>
        </div>

        {/* Subscription Tab Content */}
        {activeTab === 'subscription' && (
          <div className='animate-fadeIn'>
            <div className='text-center mb-12'>
              <Title heading={2} className='m-0 mb-4 text-slate-800 dark:text-white text-2xl font-semibold'>
                {t('包月套餐 - 限时特惠')}
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
            <div className='text-center mb-8'>
              <Title heading={2} className='m-0 mb-4 text-slate-800 dark:text-white text-2xl font-semibold'>
                {t('按量付费 - 钱包充值')}
              </Title>
              <Text className='text-slate-500 dark:text-slate-400'>
                {t('充值到钱包，随用随扣，余额永不过期')}
              </Text>
            </div>

            {/* 通用特色说明 - 提取到卡片上方 */}
            <div
              className='flex flex-wrap justify-center gap-6 mb-10 px-4'
            >
              {[
                { text: t('无最低消费'), icon: '💰' },
                { text: t('余额永不过期'), icon: '♾️' },
                { text: t('随时充值'), icon: '⚡' },
                { text: t('透明计费'), icon: '📊' },
              ].map((feature, idx) => (
                <div
                  key={idx}
                  className='flex items-center gap-2 px-4 py-2 rounded-full text-sm font-medium'
                  style={{
                    background: 'white',
                    border: '2px solid #E2E8F0',
                    color: '#475569',
                  }}
                >
                  <span>{feature.icon}</span>
                  {feature.text}
                </div>
              ))}
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
      <section
        className='py-16 px-6'
        style={{
          background: 'linear-gradient(135deg, rgba(37, 99, 235, 0.05), rgba(59, 130, 246, 0.05))',
        }}
      >
        <div className='max-w-6xl mx-auto'>
          <Title
            heading={2}
            className='text-center m-0 mb-4'
            style={{
              color: '#1E293B',
              fontSize: '1.75rem',
              fontWeight: 600,
              fontFamily: "'Poppins', 'Inter', system-ui, sans-serif",
            }}
          >
            {t('计费方式说明')}
          </Title>
          <Text
            className='text-center block mb-12'
            style={{ color: '#475569', fontSize: '1rem' }}
          >
            {t('订阅和按量付费可以同时拥有，系统会智能选择最优惠的方式扣费')}
          </Text>

          {/* Flow Diagram */}
          <div className='flex flex-wrap items-center justify-center gap-6 mb-10'>
            {/* Step 1 - Active */}
            <div
              className='bg-white dark:bg-gray-800 px-8 py-6 rounded-xl min-w-[200px] text-center cursor-pointer'
              style={{
                border: '2px solid #2563EB',
                background: 'linear-gradient(135deg, rgba(37, 99, 235, 0.05), rgba(59, 130, 246, 0.05))',
                transition: 'all 300ms cubic-bezier(0.4, 0, 0.2, 1)',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.transform = 'translateY(-4px)';
                e.currentTarget.style.boxShadow = '0 8px 24px rgba(37, 99, 235, 0.1)';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.transform = 'translateY(0)';
                e.currentTarget.style.boxShadow = 'none';
              }}
            >
              <Text className='text-sm block mb-2' style={{ color: '#475569' }}>{t('第1步')}</Text>
              <Text
                className='text-xl font-bold'
                style={{ color: '#2563EB', fontFamily: "'Poppins', 'Inter', system-ui, sans-serif" }}
              >
                {t('优先使用订阅')}
              </Text>
            </div>

            <span className='text-3xl hidden md:block' style={{ color: '#2563EB' }}>→</span>
            <span className='text-3xl md:hidden' style={{ color: '#2563EB', transform: 'rotate(90deg)' }}>→</span>

            {/* Step 2 */}
            <div
              className='bg-white dark:bg-gray-800 px-8 py-6 rounded-xl min-w-[200px] text-center cursor-pointer'
              style={{
                border: '2px solid #E2E8F0',
                transition: 'all 300ms cubic-bezier(0.4, 0, 0.2, 1)',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.transform = 'translateY(-4px)';
                e.currentTarget.style.boxShadow = '0 8px 24px rgba(37, 99, 235, 0.1)';
                e.currentTarget.style.borderColor = '#3B82F6';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.transform = 'translateY(0)';
                e.currentTarget.style.boxShadow = 'none';
                e.currentTarget.style.borderColor = '#E2E8F0';
              }}
            >
              <Text className='text-sm block mb-2' style={{ color: '#475569' }}>{t('订阅用完/过期')}</Text>
              <Text
                className='text-xl font-bold'
                style={{ color: '#2563EB', fontFamily: "'Poppins', 'Inter', system-ui, sans-serif" }}
              >
                {t('自动切换')}
              </Text>
            </div>

            <span className='text-3xl hidden md:block' style={{ color: '#2563EB' }}>→</span>
            <span className='text-3xl md:hidden' style={{ color: '#2563EB', transform: 'rotate(90deg)' }}>→</span>

            {/* Step 3 */}
            <div
              className='bg-white dark:bg-gray-800 px-8 py-6 rounded-xl min-w-[200px] text-center cursor-pointer'
              style={{
                border: '2px solid #E2E8F0',
                transition: 'all 300ms cubic-bezier(0.4, 0, 0.2, 1)',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.transform = 'translateY(-4px)';
                e.currentTarget.style.boxShadow = '0 8px 24px rgba(37, 99, 235, 0.1)';
                e.currentTarget.style.borderColor = '#3B82F6';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.transform = 'translateY(0)';
                e.currentTarget.style.boxShadow = 'none';
                e.currentTarget.style.borderColor = '#E2E8F0';
              }}
            >
              <Text className='text-sm block mb-2' style={{ color: '#475569' }}>{t('第2步')}</Text>
              <Text
                className='text-xl font-bold'
                style={{ color: '#2563EB', fontFamily: "'Poppins', 'Inter', system-ui, sans-serif" }}
              >
                {t('使用钱包余额')}
              </Text>
            </div>
          </div>

          {/* Comparison Cards */}
          <div className='grid grid-cols-1 md:grid-cols-2 gap-6 max-w-4xl mx-auto'>
            {/* Subscription Card */}
            <Card
              className='bg-white dark:bg-gray-800'
              style={{
                borderRadius: '16px',
                border: '2px solid #2563EB',
              }}
              bodyStyle={{ padding: '32px' }}
            >
              <span
                className='inline-block px-3 py-1 rounded-xl text-xs font-semibold mb-3'
                style={{ background: '#DBEAFE', color: '#1E40AF' }}
              >
                {t('包月套餐')}
              </span>
              <Title
                heading={4}
                className='m-0 mb-4'
                style={{ color: '#1E293B', fontFamily: "'Poppins', 'Inter', system-ui, sans-serif" }}
              >
                {t('限时优惠包')}
              </Title>
              <ul className='list-none p-0 m-0 space-y-2'>
                {[
                  t('有效期限制'),
                  t('价格最优惠'),
                  t('过期后剩余额度清零'),
                  t('适合稳定高频使用'),
                ].map((item, idx) => (
                  <li key={idx} className='flex items-start gap-2 text-sm' style={{ color: '#475569', padding: '8px 0' }}>
                    <span style={{ color: '#10B981', fontWeight: 700, flexShrink: 0 }}>✓</span>
                    <span>{item}</span>
                  </li>
                ))}
              </ul>
            </Card>

            {/* PAYG Card */}
            <Card
              className='bg-white dark:bg-gray-800'
              style={{
                borderRadius: '16px',
                border: '2px solid #E2E8F0',
              }}
              bodyStyle={{ padding: '32px' }}
            >
              <span
                className='inline-block px-3 py-1 rounded-xl text-xs font-semibold mb-3'
                style={{ background: '#FEF3C7', color: '#92400E' }}
              >
                {t('按量付费')}
              </span>
              <Title
                heading={4}
                className='m-0 mb-4'
                style={{ color: '#1E293B', fontFamily: "'Poppins', 'Inter', system-ui, sans-serif" }}
              >
                {t('充值钱包余额')}
              </Title>
              <ul className='list-none p-0 m-0 space-y-2'>
                {[
                  t('永久有效，随时可用'),
                  t('标准价格'),
                  t('余额永不过期'),
                  t('适合偶尔使用或保底'),
                ].map((item, idx) => (
                  <li key={idx} className='flex items-start gap-2 text-sm' style={{ color: '#475569', padding: '8px 0' }}>
                    <span style={{ color: '#10B981', fontWeight: 700, flexShrink: 0 }}>✓</span>
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
        <Title
          heading={2}
          className='text-center m-0 mb-12'
          style={{
            color: '#1E293B',
            fontSize: '1.75rem',
            fontWeight: 600,
            fontFamily: "'Poppins', 'Inter', system-ui, sans-serif",
          }}
        >
          {t('常见问题')}
        </Title>

        <div className='space-y-4'>
          {[
            {
              q: t('包月套餐用完后会怎样？'),
              a: t('系统会自动切换到您的钱包余额继续计费（按¥0.50/美金），不会中断服务。建议钱包保持一定余额作为保底。'),
            },
            {
              q: t('包月套餐过期了，剩余额度还在吗？'),
              a: t('包月套餐过期后，剩余额度会清零。所以建议根据实际使用量选择合适的套餐，避免浪费。如果用不完，可以选择更小的套餐或直接用按量付费。'),
            },
            {
              q: t('我同时购买了多个包月套餐，怎么扣费？'),
              a: t('系统会优先使用即将过期的套餐，避免浪费。多个套餐的有效期是独立计算的，不会叠加。'),
            },
            {
              q: t('费用是怎么计算的？'),
              a: t('按照token用量和模型价格计算得出，以实际消费为准。'),
            },
          ].map((faq, idx) => (
            <div
              key={idx}
              className='bg-white dark:bg-gray-800 rounded-xl cursor-pointer'
              style={{
                border: '2px solid #E2E8F0',
                transition: 'all 250ms cubic-bezier(0.4, 0, 0.2, 1)',
              }}
              onClick={() => setExpandedFaq(expandedFaq === idx ? null : idx)}
              onMouseEnter={(e) => {
                e.currentTarget.style.borderColor = '#2563EB';
                e.currentTarget.style.boxShadow = '0 4px 20px rgba(37, 99, 235, 0.08)';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.borderColor = '#E2E8F0';
                e.currentTarget.style.boxShadow = 'none';
              }}
            >
              <div className='p-7 flex items-center justify-between'>
                <Text
                  className='font-semibold flex items-center gap-2'
                  style={{ color: '#1E293B', fontSize: '1.0625rem' }}
                >
                  <span>💡</span> {faq.q}
                </Text>
                {expandedFaq === idx
                  ? <IconChevronUp style={{ color: '#94A3B8' }} />
                  : <IconChevronDown style={{ color: '#94A3B8' }} />
                }
              </div>
              {expandedFaq === idx && (
                <div className='px-7 pb-7 pt-0'>
                  <Text style={{ color: '#475569', lineHeight: 1.7 }}>{faq.a}</Text>
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

        @keyframes priceReveal {
          0% {
            opacity: 0;
            transform: translateY(10px);
          }
          100% {
            opacity: 1;
            transform: translateY(0);
          }
        }
        .price-animated {
          display: inline-block;
          animation: priceReveal 600ms cubic-bezier(0.4, 0, 0.2, 1) forwards;
        }

        /* Selection color */
        .plan-pricing-page ::selection {
          background: rgba(37, 99, 235, 0.2);
          color: #1E293B;
        }

        /* Better focus indicators */
        .plan-pricing-page a:focus-visible,
        .plan-pricing-page button:focus-visible {
          outline: 2px solid #2563EB;
          outline-offset: 2px;
        }

        /* Accessibility - reduced motion */
        @media (prefers-reduced-motion: reduce) {
          .plan-pricing-page * {
            animation: none !important;
            transition: none !important;
          }
        }
      `}</style>
    </div>
  );
};

export default PlanPricing;
