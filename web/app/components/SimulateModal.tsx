'use client';

import React, { useState, useEffect } from 'react';
import {
  X,
  Package,
  Layers,
  Calendar,
  Sparkles,
  ArrowRight,
  Info,
} from 'lucide-react';
import { formatPaiseToINR } from '../utils';

interface IssueData {
  id: number;
  party: { id: number; name: string };
  sku: { id: number; code: string; name: string };
  issued: number;
  received: number;
  outstanding: number;
  rate_paise: number;
}

interface SimulateModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (issueId: number, qty: number, date: string) => Promise<void>;
  loading: boolean;
}

export function SimulateModal({
  isOpen,
  onClose,
  onSubmit,
  loading,
}: SimulateModalProps) {
  const [issueId, setIssueId] = useState(1);
  const [qty, setQty] = useState(2);
  const [date, setDate] = useState(() => new Date().toISOString().split('T')[0]);
  const [issueData, setIssueData] = useState<IssueData | null>(null);
  const [issueLoading, setIssueLoading] = useState(false);

  useEffect(() => {
    if (!isOpen) return;

    let active = true;
    setIssueLoading(true);

    fetch(`/v1/issues/${issueId}`)
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (active && data) {
          setIssueData(data);
        }
      })
      .catch(() => {})
      .finally(() => {
        if (active) setIssueLoading(false);
      });

    return () => {
      active = false;
    };
  }, [isOpen, issueId]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onSubmit(issueId, qty, date);
  };

  const estimatedPaise = issueData ? qty * issueData.rate_paise : 0;
  const isOverReceipt = issueData ? qty > issueData.outstanding : false;

  return (
    <div className="vibe-modal-backdrop" onClick={onClose}>
      <div
        className="vibe-modal-card"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        {/* Modal Header */}
        <div className="vibe-modal-header">
          <div className="vibe-modal-title-group">
            <div className="vibe-modal-badge">
              <Sparkles size={11} /> ATELIER PRODUCTION
            </div>
            <h2 className="vibe-modal-title">Record Fabric Job-Work Receipt</h2>
            <p className="vibe-modal-desc">
              Simulates incoming lot from embroidery job-worker with atomic voucher issuance
            </p>
          </div>
          <button
            type="button"
            className="vibe-modal-close"
            onClick={onClose}
            aria-label="Close dialog"
          >
            <X size={16} />
          </button>
        </div>

        {/* Live Issue Info Banner */}
        <div className="vibe-issue-preview">
          <div className="preview-top-row">
            <span className="preview-label">
              <Package size={13} /> Active Job-Work Issue #{issueId}
            </span>
            {issueData && (
              <span className="preview-balance">
                Available Balance: <strong>{issueData.outstanding} pcs</strong>
              </span>
            )}
          </div>

          {issueData ? (
            <div className="preview-details">
              <div className="preview-metric">
                <span className="metric-k">Job-Worker</span>
                <span className="metric-v">{issueData.party.name}</span>
              </div>
              <div className="preview-metric">
                <span className="metric-k">SKU / Item</span>
                <span className="metric-v font-mono">{issueData.sku.name} ({issueData.sku.code})</span>
              </div>
              <div className="preview-metric">
                <span className="metric-k">Contract Rate</span>
                <span className="metric-v text-emerald">{formatPaiseToINR(issueData.rate_paise)} / pc</span>
              </div>
            </div>
          ) : (
            <div className="preview-loading">
              {issueLoading ? 'Fetching contract terms...' : 'Issue details ready'}
            </div>
          )}
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} className="vibe-modal-form">
          {/* Issue ID Selector */}
          <div className="vibe-field-group">
            <label htmlFor="modal-issue-id" className="vibe-label">
              Job-Work Issue ID
            </label>
            <div className="vibe-input-wrap">
              <Layers size={14} className="input-icon" />
              <input
                id="modal-issue-id"
                type="number"
                min="1"
                required
                value={issueId}
                onChange={(e) => setIssueId(Number(e.target.value))}
                className="vibe-input"
              />
            </div>
          </div>

          {/* Quantity Input with Quick Chips */}
          <div className="vibe-field-group">
            <div className="label-row">
              <label htmlFor="modal-qty" className="vibe-label">
                Received Quantity (Pieces)
              </label>
              <div className="quick-qty-chips">
                {[1, 2, 5].map((preset) => (
                  <button
                    key={preset}
                    type="button"
                    className={`qty-chip ${qty === preset ? 'active' : ''}`}
                    onClick={() => setQty(preset)}
                  >
                    +{preset}
                  </button>
                ))}
              </div>
            </div>
            <input
              id="modal-qty"
              type="number"
              min="1"
              max="1000"
              required
              value={qty}
              onChange={(e) => setQty(Number(e.target.value))}
              className={`vibe-input ${isOverReceipt ? 'input-warning' : ''}`}
            />
            {isOverReceipt && (
              <span className="vibe-field-warning">
                <Info size={11} /> Exceeds outstanding balance ({issueData?.outstanding} pcs). This will test the guarded G1 over-receipt rollback!
              </span>
            )}
          </div>

          {/* Received On Date */}
          <div className="vibe-field-group">
            <label htmlFor="modal-date" className="vibe-label">
              Shipment Arrival Date
            </label>
            <div className="vibe-input-wrap">
              <Calendar size={14} className="input-icon" />
              <input
                id="modal-date"
                type="date"
                required
                value={date}
                onChange={(e) => setDate(e.target.value)}
                className="vibe-input"
              />
            </div>
          </div>

          {/* Financial Preview Box */}
          {estimatedPaise > 0 && (
            <div className="vibe-estimate-card">
              <div className="estimate-row">
                <span className="estimate-label">Accounting Voucher Value</span>
                <span className="estimate-value text-emerald font-mono">
                  {formatPaiseToINR(estimatedPaise)}
                </span>
              </div>
              <span className="estimate-formula">
                Formula: {qty} pcs × {formatPaiseToINR(issueData?.rate_paise || 0)}/pc
              </span>
            </div>
          )}

          {/* Modal Actions */}
          <div className="vibe-modal-actions">
            <button
              type="button"
              className="action-btn action-btn-secondary"
              onClick={onClose}
              disabled={loading}
            >
              Cancel
            </button>
            <button
              type="submit"
              id="submit-receipt-btn"
              className="action-btn action-btn-primary"
              disabled={loading}
            >
              <span>{loading ? 'Recording Atomic Tx...' : 'Create Receipt & Voucher'}</span>
              <ArrowRight size={14} />
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
