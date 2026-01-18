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

import React from 'react';
import { Link } from 'react-router-dom';
import './HomeBento.css';

const HomeBento = () => {
  return (
    <div className="bento-home-container">
      {/* Hero Section */}
      <section className="bento-hero">
        <div className="bento-container">
          <div className="bento-badge">
            <span className="bento-badge-dot"></span>
            一个账号 · 三大模型
          </div>

          <h1 className="bento-headline">
            <span className="bento-gradient-text">Claude · GPT · Gemini</span><br />
            统一接入，极致性价比
          </h1>

          <p className="bento-subheadline">
            无需多个账号，一站式访问三大AI模型。<br />
            标准1:1倍率，GPT系列仅0.2倍率，为您节省80%成本。
          </p>

          <div className="bento-cta-buttons">
            <Link to="/login" className="bento-cta-button">立即开始</Link>
            <Link to="/plans" className="bento-cta-button-secondary">产品定价</Link>
          </div>
        </div>
      </section>

      {/* Payment Bento Grid */}
      <section className="bento-section">
        <div className="bento-container">
          <h2 className="bento-section-title">灵活的付费方式</h2>
          <p className="bento-section-subtitle">两种模式可叠加使用，优先消耗套餐额度</p>

          <div className="bento-grid">
            <div className="bento-card bento-animate-on-scroll">
              <h3 className="bento-title">按量计费</h3>
              <p className="bento-desc">用多少付多少，无需预付费</p>
              <ul className="bento-features">
                <li><span className="bento-check-icon">✓</span> 无最低消费</li>
                <li><span className="bento-check-icon">✓</span> 实时计费</li>
                <li><span className="bento-check-icon">✓</span> 随时充值</li>
                <li><span className="bento-check-icon">✓</span> 适合轻度使用</li>
              </ul>
            </div>

            <div className="bento-card bento-featured bento-animate-on-scroll bento-animate-delay-1">
              <h3 className="bento-title">包月套餐 · 推荐</h3>
              <p className="bento-desc">限时额度，成本更低</p>
              <ul className="bento-features">
                <li><span className="bento-check-icon">✓</span> 有效期内使用</li>
                <li><span className="bento-check-icon">✓</span> 比按量更便宜</li>
                <li><span className="bento-check-icon">✓</span> 多种额度可选</li>
                <li><span className="bento-check-icon">✓</span> 优先消耗套餐</li>
              </ul>
            </div>
          </div>

          <div className="bento-note-card">
            <p>
              <strong>智能叠加</strong> — 同时购买包月套餐和按量充值，系统将优先使用包月套餐额度，套餐用完后自动切换至按量计费，无缝衔接，不影响使用。
            </p>
          </div>
        </div>
      </section>

      {/* Stats Section */}
      <section className="bento-stats-section">
        <div className="bento-container">
          <h2 className="bento-section-title">服务保障</h2>
          <p className="bento-section-subtitle">稳定可靠，值得信赖</p>

          <div className="bento-stats-grid">
            <div className="bento-stat-card bento-animate-on-scroll">
              <div className="bento-stat-value">高可用</div>
              <div className="bento-stat-label">稳定服务保障</div>
              <div className="bento-stat-note">服务可用性低可联系客服退款</div>
            </div>

            <div className="bento-stat-card bento-animate-on-scroll bento-animate-delay-1">
              <div className="bento-stat-value">真实模型</div>
              <div className="bento-stat-label">拒绝套壳，支持验证</div>
              <div className="bento-stat-note">假模型全额双倍退款</div>
            </div>

            <div className="bento-stat-card bento-animate-on-scroll bento-animate-delay-2">
              <div className="bento-stat-value">安全可靠</div>
              <div className="bento-stat-label">企业级安全防护</div>
              <div className="bento-stat-note">全球加速节点，低延迟</div>
            </div>
          </div>
        </div>
      </section>
    </div>
  );
};

export default HomeBento;
