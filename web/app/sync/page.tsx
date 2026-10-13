'use client';

import React, { useState, useEffect, useCallback } from 'react';
import { formatPaiseToINR, formatDate } from '../utils';

interface Voucher {
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

export default function SyncPage() {
  const [vouchers, setVouchers] = useState<Voucher[]>([]);
  const [loading, setLoading] = useState(true);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [retryingId, setRetryingId] = useState<number | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Simulation form state
  const [showSimModal, setShowSimModal] = useState(false);
  const [simIssueId, setSimIssueId] = useState(1);
  const [simQty, setSimQty] = useState(1);
  const [simDate, setSimDate] = useState(new Date().toISOString().split('T')[0]);
  const [simulating, setSimulating] = useState(false);

  const showToast = (message: string, type: 'success' | 'error') => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 4000);
  };

  const fetchVouchers = useCallback(async (isBackground = false) => {
    if (!isBackground) setLoading(true);
    try {
      const res = await fetch('/v1/vouchers', { cache: 'no-store' });
      if (!res.ok) {
        throw new Error(`Failed to load vouchers: HTTP ${res.status}`);
      }
      const data = await res.json();
      const list = Array.isArray(data) ? data : (data.vouchers || data.data || []);
      setVouchers(list);
      setLastUpdated(new Date());
    } catch (err: unknown) {
      if (!isBackground) {
        const msg = err instanceof Error ? err.message : String(err);
        showToast(`Could not connect to API: ${msg}`, 'error');
      }
    } finally {
      if (!isBackground) setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchVouchers(false);
  }, [fetchVouchers]);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(() => {
      fetchVouchers(true);
    }, 2500);
    return () => clearInterval(interval);
  }, [autoRefresh, fetchVouchers]);

  const handleRetry = async (voucherId: number) => {
    setRetryingId(voucherId);
    try {
      const res = await fetch(`/v1/vouchers/${voucherId}/retry`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      });
      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error?.message || `HTTP ${res.status}`);
      }
      showToast(`Voucher #${voucherId} reset to pending for immediate retry!`, 'success');
      await fetchVouchers(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      showToast(`Retry failed: ${msg}`, 'error');
    } finally {
      setRetryingId(null);
    }
  };

  const handleCreateReceipt = async (e: React.FormEvent) => {
    e.preventDefault();
    setSimulating(true);
    try {
      const idempKey = `sim-${Date.now()}-${Math.random().toString(36).substring(2, 9)}`;
      const res = await fetch('/v1/jobwork/receipts', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': idempKey,
        },
        body: JSON.stringify({
          issue_id: Number(simIssueId),
          qty: Number(simQty),
          received_on: simDate,
        }),
      });
      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error?.message || `HTTP ${res.status}`);
      }
      const receipt = data.receipt || data.data?.receipt;
      const voucher = data.voucher || data.data?.voucher;
      const rId = receipt?.id ? `#${receipt.id} ` : '';
      const vId = voucher?.id ? ` Voucher #${voucher.id} queued.` : '';
      showToast(`Receipt ${rId}recorded!${vId}`, 'success');
      setShowSimModal(false);
      await fetchVouchers(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      showToast(`Receipt creation failed: ${msg}`, 'error');
    } finally {
      setSimulating(false);
    }
  };

  const pending = vouchers.filter((v) => v.status === 'pending');
  const pushed = vouchers.filter((v) => v.status === 'pushed');
  const failed = vouchers.filter((v) => v.status === 'failed');

  return (
    <div className="dashboard-container">
      {/* Toast Alert */}
      {toast && (
        <div className={`toast-notification ${toast.type}`}>
          <span>{toast.type === 'success' ? '✓' : '⚠'}</span>
          <span>{toast.message}</span>
        </div>
      )}

      {/* Top Navigation / Brand */}
      <header className="header">
        <div className="brand-group">
          <div className="brand-badge">SHREE RADHA STUDIO, DELHI</div>
          <h1 className="title">Sutra Accounting Sync Portal</h1>
          <p className="subtitle">Resilient job-work accounting voucher pipeline with fault-tolerant reconciliation</p>
        </div>

        <div className="header-actions">
          <button
            id="sim-receipt-btn"
            className="btn btn-primary"
            onClick={() => setShowSimModal(!showSimModal)}
          >
            <span>+</span> Simulate Job-Work Receipt
          </button>

          <button
            id="refresh-btn"
            className="btn btn-secondary"
            onClick={() => fetchVouchers(false)}
            disabled={loading}
          >
            {loading ? 'Refreshing...' : '↻ Refresh'}
          </button>

          <label className="toggle-label">
            <input
              type="checkbox"
              id="auto-refresh-toggle"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
            />
            <span className="toggle-indicator">
              {autoRefresh && <span className="pulse-indicator"></span>}
              Auto-sync (2.5s)
            </span>
          </label>
        </div>
      </header>

      {/* Status Bar */}
      <div className="status-bar">
        <div className="stat-pill">
          <span className="stat-label">Total Vouchers</span>
          <span className="stat-val">{vouchers.length}</span>
        </div>
        <div className="stat-pill">
          <span className="stat-label">Pending</span>
          <span className="stat-val amber">{pending.length}</span>
        </div>
        <div className="stat-pill">
          <span className="stat-label">Pushed</span>
          <span className="stat-val emerald">{pushed.length}</span>
        </div>
        <div className="stat-pill">
          <span className="stat-label">Failed</span>
          <span className="stat-val rose">{failed.length}</span>
        </div>
        {lastUpdated && (
          <div className="last-sync">
            Last updated: {lastUpdated.toLocaleTimeString()}
          </div>
        )}
      </div>

      {/* Simulate Modal / Drawer */}
      {showSimModal && (
        <div className="modal-backdrop" onClick={() => setShowSimModal(false)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h2>Record Fabric Receipt (Simulate)</h2>
              <button className="close-btn" onClick={() => setShowSimModal(false)}>✕</button>
            </div>
            <form onSubmit={handleCreateReceipt}>
              <div className="form-group">
                <label htmlFor="issue_id">Job-Work Issue ID</label>
                <input
                  id="issue_id"
                  type="number"
                  min="1"
                  required
                  value={simIssueId}
                  onChange={(e) => setSimIssueId(Number(e.target.value))}
                />
                <span className="help-text">Issue #1: Kurti Embroidery (Rate: ₹120.00/pc)</span>
              </div>
              <div className="form-group">
                <label htmlFor="qty">Received Quantity (Pieces)</label>
                <input
                  id="qty"
                  type="number"
                  min="1"
                  max="50"
                  required
                  value={simQty}
                  onChange={(e) => setSimQty(Number(e.target.value))}
                />
              </div>
              <div className="form-group">
                <label htmlFor="received_on">Received On</label>
                <input
                  id="received_on"
                  type="date"
                  required
                  value={simDate}
                  onChange={(e) => setSimDate(e.target.value)}
                />
              </div>
              <div className="modal-actions">
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => setShowSimModal(false)}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  id="submit-receipt-btn"
                  className="btn btn-primary"
                  disabled={simulating}
                >
                  {simulating ? 'Recording Tx...' : 'Create Receipt & Voucher'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 3-Column Kanban Layout */}
      <main className="kanban-grid">
        {/* Column 1: Pending */}
        <section className="column pending-col" id="col-pending">
          <div className="column-header">
            <div className="col-title-group">
              <span className="badge badge-pending">PENDING</span>
              <span className="badge-count">{pending.length}</span>
            </div>
            <span className="col-desc">Awaiting sync or in retry backoff</span>
          </div>

          <div className="cards-list">
            {pending.length === 0 ? (
              <div className="empty-state">No vouchers pending sync</div>
            ) : (
              pending.map((v) => (
                <div key={v.id} className="voucher-card" id={`voucher-${v.id}`}>
                  <div className="card-top">
                    <span className="voucher-id">#VCH-{v.id}</span>
                    <span className="card-amount">{formatPaiseToINR(v.amount_paise)}</span>
                  </div>

                  <div className="card-meta">
                    <span className="client-ref mono" title={v.client_ref}>
                      {v.client_ref}
                    </span>
                  </div>

                  <div className="card-bottom">
                    <span className="pill">Attempts: {v.attempts}</span>
                    {v.locked_until && (
                      <span className="pill pill-leased">
                        <span className="pulse-indicator"></span> Leased
                      </span>
                    )}
                    <span className="meta-time" title={v.next_attempt_at}>
                      Next: {formatDate(v.next_attempt_at)}
                    </span>
                  </div>
                </div>
              ))
            )}
          </div>
        </section>

        {/* Column 2: Pushed */}
        <section className="column pushed-col" id="col-pushed">
          <div className="column-header">
            <div className="col-title-group">
              <span className="badge badge-pushed">PUSHED</span>
              <span className="badge-count">{pushed.length}</span>
            </div>
            <span className="col-desc">Synchronized to accounting system</span>
          </div>

          <div className="cards-list">
            {pushed.length === 0 ? (
              <div className="empty-state">No pushed vouchers yet</div>
            ) : (
              pushed.map((v) => {
                const isReconciled = v.attempts > 1;
                return (
                  <div key={v.id} className="voucher-card card-pushed" id={`voucher-${v.id}`}>
                    <div className="card-top">
                      <span className="voucher-id">#VCH-{v.id}</span>
                      <span className="card-amount emerald-text">{formatPaiseToINR(v.amount_paise)}</span>
                    </div>

                    <div className="card-meta">
                      <div className="ext-ref-row">
                        <span className="ext-label">Accounting Ref:</span>
                        <span className="ext-val mono">{v.external_ref || 'N/A'}</span>
                      </div>
                      <span className="client-ref mono muted" title={v.client_ref}>
                        Ref: {v.client_ref}
                      </span>
                    </div>

                    <div className="card-bottom">
                      <span className="pill pill-success">✓ Synced</span>
                      {isReconciled && (
                        <span className="pill pill-reconciled" title="Reconciled via HTTP 409 conflict after prior attempt">
                          ⚡ Reconciled (409)
                        </span>
                      )}
                      <span className="meta-time">
                        {formatDate(v.created_at)}
                      </span>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        </section>

        {/* Column 3: Failed */}
        <section className="column failed-col" id="col-failed">
          <div className="column-header">
            <div className="col-title-group">
              <span className="badge badge-failed">FAILED</span>
              <span className="badge-count">{failed.length}</span>
            </div>
            <span className="col-desc">Exhausted retries, requires manual retry</span>
          </div>

          <div className="cards-list">
            {failed.length === 0 ? (
              <div className="empty-state">No failed vouchers</div>
            ) : (
              failed.map((v) => (
                <div key={v.id} className="voucher-card card-failed" id={`voucher-${v.id}`}>
                  <div className="card-top">
                    <span className="voucher-id">#VCH-{v.id}</span>
                    <span className="card-amount rose-text">{formatPaiseToINR(v.amount_paise)}</span>
                  </div>

                  <div className="card-meta">
                    <span className="client-ref mono" title={v.client_ref}>
                      {v.client_ref}
                    </span>
                    <div className="error-box">
                      <span className="error-tag">Error</span>
                      <span className="error-msg">{v.last_error || 'Unknown accounting failure'}</span>
                    </div>
                  </div>

                  <div className="card-bottom failed-bottom">
                    <span className="pill pill-danger">
                      {v.attempts} Attempts Exhausted
                    </span>
                    <button
                      id={`retry-btn-${v.id}`}
                      className="btn btn-retry"
                      disabled={retryingId === v.id}
                      onClick={() => handleRetry(v.id)}
                    >
                      {retryingId === v.id ? 'Retrying...' : '↻ Retry'}
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        </section>
      </main>
    </div>
  );
}
