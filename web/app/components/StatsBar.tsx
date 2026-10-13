'use client';

import React from 'react';
import {
  Clock,
  CheckCircle2,
  AlertOctagon,
  CreditCard,
  Layers,
} from 'lucide-react';
import { formatPaiseToINR } from '../utils';

interface StatsBarProps {
  totalCount: number;
  pendingCount: number;
  pushedCount: number;
  failedCount: number;
  totalSyncedPaise: number;
  lastUpdated: Date | null;
}

export function StatsBar({
  totalCount,
  pendingCount,
  pushedCount,
  failedCount,
  totalSyncedPaise,
  lastUpdated,
}: StatsBarProps) {
  const syncRate = totalCount > 0 ? Math.round((pushedCount / totalCount) * 100) : 100;

  return (
    <div className="stats-bar-grid">
      {/* Metric 1: Total Volume Synced */}
      <div className="stat-card">
        <div className="stat-card-header">
          <span className="stat-card-label">Synced Volume</span>
          <CreditCard size={15} className="stat-icon-muted" />
        </div>
        <div className="stat-card-value font-mono text-emerald">
          {formatPaiseToINR(totalSyncedPaise)}
        </div>
        <div className="stat-card-sub">
          <span>{pushedCount} of {totalCount} vouchers posted</span>
        </div>
      </div>

      {/* Metric 2: Sync Success Rate */}
      <div className="stat-card">
        <div className="stat-card-header">
          <span className="stat-card-label">Sync Health</span>
          <Layers size={15} className="stat-icon-muted" />
        </div>
        <div className="stat-card-value font-mono">
          {syncRate}%
        </div>
        <div className="stat-card-sub">
          <span className="text-secondary">Transactional integrity guaranteed</span>
        </div>
      </div>

      {/* Metric 3: Pending Queue */}
      <div className="stat-card">
        <div className="stat-card-header">
          <span className="stat-card-label">In-Flight Queue</span>
          <Clock size={15} className="stat-icon-amber" />
        </div>
        <div className={`stat-card-value font-mono ${pendingCount > 0 ? 'text-amber' : ''}`}>
          {pendingCount}
        </div>
        <div className="stat-card-sub">
          <span>{pendingCount > 0 ? 'Awaiting worker lease' : 'Queue clear'}</span>
        </div>
      </div>

      {/* Metric 4: Failed / Action Required */}
      <div className="stat-card">
        <div className="stat-card-header">
          <span className="stat-card-label">Failed / Action</span>
          <AlertOctagon size={15} className={failedCount > 0 ? 'stat-icon-rose' : 'stat-icon-muted'} />
        </div>
        <div className={`stat-card-value font-mono ${failedCount > 0 ? 'text-rose' : ''}`}>
          {failedCount}
        </div>
        <div className="stat-card-sub">
          <span>{failedCount > 0 ? 'Requires operator retry' : '0 errors logged'}</span>
        </div>
      </div>
    </div>
  );
}
