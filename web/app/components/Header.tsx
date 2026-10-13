'use client';

import React from 'react';
import {
  Plus,
  RefreshCw,
  Search,
  Sparkles,
  Layers,
} from 'lucide-react';

interface HeaderProps {
  onOpenSimModal: () => void;
  onRefresh: () => void;
  loading: boolean;
  autoRefresh: boolean;
  onToggleAutoRefresh: (val: boolean) => void;
  searchQuery: string;
  onSearchChange: (q: string) => void;
}

export function Header({
  onOpenSimModal,
  onRefresh,
  loading,
  autoRefresh,
  onToggleAutoRefresh,
  searchQuery,
  onSearchChange,
}: HeaderProps) {
  return (
    <header className="portal-header">
      <div className="brand-lockup">
        <div className="brand-top-row">
          <div className="brand-chip">
            <span className="brand-dot"></span>
            SHREE RADHA STUDIO · ATELIER DELHI
          </div>
          <span className="badge-vibe">
            <Sparkles size={11} /> Vibe Sync v1.0
          </span>
        </div>

        <h1 className="portal-title">Sutra Accounting Sync</h1>
        <p className="portal-subtitle">
          High-reliability job-work voucher pipeline with transactional idempotency & 409 fault reconciliation
        </p>
      </div>

      <div className="portal-toolbar">
        {/* Search input */}
        <div className="search-box">
          <Search size={14} className="search-icon" />
          <input
            type="text"
            placeholder="Search ref or ID..."
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            className="search-input"
          />
          {searchQuery && (
            <button
              onClick={() => onSearchChange('')}
              className="search-clear-btn"
              title="Clear search"
            >
              ✕
            </button>
          )}
        </div>

        {/* Action button: Simulate Receipt */}
        <button
          id="sim-receipt-btn"
          className="action-btn action-btn-primary"
          onClick={onOpenSimModal}
        >
          <Plus size={15} />
          <span>Record Receipt</span>
        </button>

        {/* Action button: Refresh */}
        <button
          id="refresh-btn"
          className={`action-btn action-btn-secondary ${loading ? 'loading' : ''}`}
          onClick={onRefresh}
          disabled={loading}
          title="Refresh voucher state"
        >
          <RefreshCw size={14} className={loading ? 'spin-icon' : ''} />
          <span>{loading ? 'Syncing...' : 'Refresh'}</span>
        </button>

        {/* Auto sync switch */}
        <label className="auto-sync-toggle" title="Auto-refresh voucher list every 2.5s">
          <input
            type="checkbox"
            id="auto-refresh-toggle"
            checked={autoRefresh}
            onChange={(e) => onToggleAutoRefresh(e.target.checked)}
          />
          <span className="toggle-switch-track">
            <span className="toggle-switch-thumb"></span>
          </span>
          <span className="toggle-label-text">
            {autoRefresh ? (
              <>
                <span className="live-dot pulse-animation"></span>
                <span>Live Engine</span>
              </>
            ) : (
              <span>Engine Paused</span>
            )}
          </span>
        </label>
      </div>
    </header>
  );
}
