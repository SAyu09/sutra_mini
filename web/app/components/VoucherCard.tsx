'use client';

import React, { useState } from 'react';
import {
  Copy,
  Check,
  CheckCircle2,
  Clock,
  RotateCcw,
  Zap,
  ShieldAlert,
  Radio,
  FileText,
} from 'lucide-react';
import {
  formatPaiseToINR,
  formatDate,
  formatNextAttempt,
  copyToClipboard,
} from '../utils';

export interface Voucher {
  id: number;
  type: string;
  source_id: number;
  amount_paise: number;
  status: 'pending' | 'pushed' | 'failed';
  attempts: number;
  last_error: string | null;
  client_ref: string;
  external_ref: string | null;
  next_attempt_at: string;
  locked_until?: string | null;
  created_at: string;
}

interface VoucherCardProps {
  voucher: Voucher;
  onRetry?: (id: number) => void;
  retrying?: boolean;
}

export function VoucherCard({ voucher, onRetry, retrying }: VoucherCardProps) {
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  const handleCopy = async (text: string, key: string) => {
    const ok = await copyToClipboard(text);
    if (ok) {
      setCopiedKey(key);
      setTimeout(() => setCopiedKey(null), 1800);
    }
  };

  const isReconciled = voucher.attempts > 1 && voucher.status === 'pushed';
  const isLeased = Boolean(
    voucher.locked_until && new Date(voucher.locked_until).getTime() > Date.now()
  );

  return (
    <div
      className={`vibe-card vibe-card-${voucher.status}`}
      id={`voucher-${voucher.id}`}
    >
      {/* Card Header */}
      <div className="vibe-card-header">
        <div className="vibe-card-id-group">
          <span className="vibe-voucher-id">#VCH-{voucher.id}</span>
          <span className="vibe-source-badge">
            <FileText size={10} />
            Receipt #{voucher.source_id}
          </span>
        </div>

        <div className={`vibe-card-amount amount-${voucher.status}`}>
          {formatPaiseToINR(voucher.amount_paise)}
        </div>
      </div>

      {/* Card Body / Reference Details */}
      <div className="vibe-card-body">
        {voucher.status === 'pushed' && voucher.external_ref && (
          <div className="vibe-ref-row ext-ref-highlight">
            <span className="vibe-ref-label">Accounting Ext Ref</span>
            <div className="vibe-ref-value-group">
              <span className="vibe-ref-mono" title={voucher.external_ref}>
                {voucher.external_ref}
              </span>
              <button
                type="button"
                className="vibe-copy-btn"
                onClick={() => handleCopy(voucher.external_ref!, `ext-${voucher.id}`)}
                title="Copy External Reference"
              >
                {copiedKey === `ext-${voucher.id}` ? (
                  <Check size={12} className="text-emerald" />
                ) : (
                  <Copy size={12} />
                )}
              </button>
            </div>
          </div>
        )}

        <div className="vibe-ref-row">
          <span className="vibe-ref-label">Studio Client Ref</span>
          <div className="vibe-ref-value-group">
            <span className="vibe-ref-mono muted" title={voucher.client_ref}>
              {voucher.client_ref}
            </span>
            <button
              type="button"
              className="vibe-copy-btn"
              onClick={() => handleCopy(voucher.client_ref, `client-${voucher.id}`)}
              title="Copy Client Reference"
            >
              {copiedKey === `client-${voucher.id}` ? (
                <Check size={12} className="text-emerald" />
              ) : (
                <Copy size={12} />
              )}
            </button>
          </div>
        </div>

        {voucher.status === 'failed' && (
          <div className="vibe-error-callout">
            <div className="vibe-error-header">
              <ShieldAlert size={12} />
              <span>Failure Diagnostic</span>
            </div>
            <div className="vibe-error-text">
              {voucher.last_error || 'Accounting API connection timeout'}
            </div>
          </div>
        )}
      </div>

      {/* Card Footer */}
      <div className="vibe-card-footer">
        <div className="vibe-footer-pills">
          {voucher.status === 'pushed' && (
            <>
              <span className="status-pill status-pill-pushed">
                <CheckCircle2 size={11} /> Synced
              </span>
              {isReconciled && (
                <span
                  className="status-pill status-pill-reconciled"
                  title="Voucher recovered and reconciled via HTTP 409 Conflict without duplicate entry"
                >
                  <Zap size={10} /> Reconciled (409)
                </span>
              )}
            </>
          )}

          {voucher.status === 'pending' && (
            <>
              <span className="status-pill status-pill-pending">
                <Clock size={11} /> Attempt {voucher.attempts}
              </span>
              {isLeased && (
                <span className="status-pill status-pill-leased">
                  <Radio size={11} className="pulse-slow" /> Leased
                </span>
              )}
            </>
          )}

          {voucher.status === 'failed' && (
            <span className="status-pill status-pill-failed">
              <ShieldAlert size={11} /> {voucher.attempts}/5 Failed
            </span>
          )}
        </div>

        <div className="vibe-footer-right">
          {voucher.status === 'failed' && onRetry && (
            <button
              type="button"
              id={`retry-btn-${voucher.id}`}
              className="vibe-retry-btn"
              disabled={retrying}
              onClick={() => onRetry(voucher.id)}
            >
              <RotateCcw size={12} className={retrying ? 'spin-icon' : ''} />
              <span>{retrying ? 'Queuing...' : 'Retry'}</span>
            </button>
          )}

          {voucher.status === 'pending' && (
            <span className="vibe-timestamp" title={voucher.next_attempt_at}>
              Due: {formatNextAttempt(voucher.next_attempt_at)}
            </span>
          )}

          {voucher.status === 'pushed' && (
            <span className="vibe-timestamp">
              {formatDate(voucher.created_at)}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
