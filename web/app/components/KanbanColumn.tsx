'use client';

import React from 'react';
import {
  Clock,
  CheckCircle2,
  AlertTriangle,
  Inbox,
} from 'lucide-react';
import { VoucherCard, Voucher } from './VoucherCard';

interface KanbanColumnProps {
  status: 'pending' | 'pushed' | 'failed';
  title: string;
  description: string;
  vouchers: Voucher[];
  onRetry?: (id: number) => void;
  retryingId?: number | null;
}

export function KanbanColumn({
  status,
  title,
  description,
  vouchers,
  onRetry,
  retryingId,
}: KanbanColumnProps) {
  const getIcon = () => {
    switch (status) {
      case 'pending':
        return <Clock size={14} className="text-amber" />;
      case 'pushed':
        return <CheckCircle2 size={14} className="text-emerald" />;
      case 'failed':
        return <AlertTriangle size={14} className="text-rose" />;
    }
  };

  const getBadgeClass = () => {
    switch (status) {
      case 'pending':
        return 'col-badge-pending';
      case 'pushed':
        return 'col-badge-pushed';
      case 'failed':
        return 'col-badge-failed';
    }
  };

  return (
    <section className={`kanban-column-card column-${status}`} id={`col-${status}`}>
      {/* Column Header */}
      <div className="kanban-column-header">
        <div className="kanban-column-title-row">
          <div className="kanban-column-title-wrap">
            <span className="col-icon-wrap">{getIcon()}</span>
            <span className={`col-status-badge ${getBadgeClass()}`}>{title}</span>
          </div>
          <span className="col-item-count">{vouchers.length}</span>
        </div>
        <p className="kanban-column-desc">{description}</p>
      </div>

      {/* Cards List */}
      <div className="kanban-cards-scroll">
        {vouchers.length === 0 ? (
          <div className="kanban-empty-state">
            <Inbox size={28} className="empty-state-icon" />
            <p className="empty-state-title">No vouchers in {title.toLowerCase()}</p>
            <p className="empty-state-sub">
              {status === 'pending'
                ? 'Queue clear · New receipts will appear here'
                : status === 'pushed'
                ? 'Synchronized vouchers will be archived here'
                : 'All retryable errors resolved'}
            </p>
          </div>
        ) : (
          vouchers.map((v) => (
            <VoucherCard
              key={v.id}
              voucher={v}
              onRetry={onRetry}
              retrying={retryingId === v.id}
            />
          ))
        )}
      </div>
    </section>
  );
}
