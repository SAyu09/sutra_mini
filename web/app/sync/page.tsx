'use client';

import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { Header } from '../components/Header';
import { StatsBar } from '../components/StatsBar';
import { KanbanColumn } from '../components/KanbanColumn';
import { SimulateModal } from '../components/SimulateModal';
import { Toast } from '../components/Toast';
import { Voucher } from '../components/VoucherCard';

export default function SyncPage() {
  const [vouchers, setVouchers] = useState<Voucher[]>([]);
  const [loading, setLoading] = useState(true);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [retryingId, setRetryingId] = useState<number | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [showSimModal, setShowSimModal] = useState(false);
  const [simulating, setSimulating] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [theme, setTheme] = useState<'light' | 'dark'>('light');

  // Initialize theme from storage (default to 'light')
  useEffect(() => {
    try {
      const saved = localStorage.getItem('sutra_theme') as 'light' | 'dark' | null;
      const initial = saved === 'dark' ? 'dark' : 'light';
      setTheme(initial);
      document.documentElement.setAttribute('data-theme', initial);
    } catch {
      document.documentElement.setAttribute('data-theme', 'light');
    }
  }, []);

  const handleToggleTheme = () => {
    const nextTheme = theme === 'light' ? 'dark' : 'light';
    setTheme(nextTheme);
    document.documentElement.setAttribute('data-theme', nextTheme);
    try {
      localStorage.setItem('sutra_theme', nextTheme);
    } catch {}
  };

  const showToast = useCallback((message: string, type: 'success' | 'error') => {
    setToast({ message, type });
  }, []);

  const fetchVouchers = useCallback(async (isBackground = false) => {
    if (!isBackground) setLoading(true);
    try {
      const res = await fetch('/v1/vouchers', { cache: 'no-store' });
      if (!res.ok) {
        throw new Error(`Failed to load vouchers: HTTP ${res.status}`);
      }
      const data = await res.json();
      const list: Voucher[] = Array.isArray(data) ? data : data.vouchers || data.data || [];
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
  }, [showToast]);

  useEffect(() => {
    fetchVouchers(false);
  }, [fetchVouchers]);

  // Auto-polling effect (every 2.5s)
  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(() => {
      fetchVouchers(true);
    }, 2500);
    return () => clearInterval(interval);
  }, [autoRefresh, fetchVouchers]);

  // Keyboard shortcut: Escape to close modal
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && showSimModal) {
        setShowSimModal(false);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [showSimModal]);

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

  const handleCreateReceipt = async (issueId: number, qty: number, date: string) => {
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
          issue_id: issueId,
          qty: qty,
          received_on: date,
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

  // Filter vouchers based on search query
  const filteredVouchers = useMemo(() => {
    if (!searchQuery.trim()) return vouchers;
    const q = searchQuery.toLowerCase().trim();
    return vouchers.filter(
      (v) =>
        v.id.toString().includes(q) ||
        v.client_ref.toLowerCase().includes(q) ||
        (v.external_ref && v.external_ref.toLowerCase().includes(q)) ||
        (v.last_error && v.last_error.toLowerCase().includes(q))
    );
  }, [vouchers, searchQuery]);

  const pending = useMemo(
    () => filteredVouchers.filter((v) => v.status === 'pending'),
    [filteredVouchers]
  );
  const pushed = useMemo(
    () => filteredVouchers.filter((v) => v.status === 'pushed'),
    [filteredVouchers]
  );
  const failed = useMemo(
    () => filteredVouchers.filter((v) => v.status === 'failed'),
    [filteredVouchers]
  );

  const totalSyncedPaise = useMemo(
    () =>
      vouchers
        .filter((v) => v.status === 'pushed')
        .reduce((sum, v) => sum + (v.amount_paise || 0), 0),
    [vouchers]
  );

  return (
    <div className="vibe-dashboard-wrapper">
      {/* Toast Alert Notification */}
      <Toast toast={toast} onClose={() => setToast(null)} />

      {/* Main Container */}
      <div className="vibe-app-container">
        {/* Top Header */}
        <Header
          onOpenSimModal={() => setShowSimModal(true)}
          onRefresh={() => fetchVouchers(false)}
          loading={loading}
          autoRefresh={autoRefresh}
          onToggleAutoRefresh={setAutoRefresh}
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          theme={theme}
          onToggleTheme={handleToggleTheme}
        />

        {/* Aggregate Stats Row */}
        <StatsBar
          totalCount={vouchers.length}
          pendingCount={vouchers.filter((v) => v.status === 'pending').length}
          pushedCount={vouchers.filter((v) => v.status === 'pushed').length}
          failedCount={vouchers.filter((v) => v.status === 'failed').length}
          totalSyncedPaise={totalSyncedPaise}
          lastUpdated={lastUpdated}
        />

        {/* Active Filter Indicator */}
        {searchQuery && (
          <div className="search-filter-banner">
            <span>
              Showing search results matching <strong>&ldquo;{searchQuery}&rdquo;</strong> ({filteredVouchers.length} matches)
            </span>
            <button
              onClick={() => setSearchQuery('')}
              className="clear-search-pill"
            >
              Clear filter
            </button>
          </div>
        )}

        {/* 3-Column Kanban Board */}
        <main className="vibe-kanban-board">
          {/* Column 1: Pending */}
          <KanbanColumn
            status="pending"
            title="PENDING"
            description="Awaiting worker lease claim or exponential backoff"
            vouchers={pending}
          />

          {/* Column 2: Pushed */}
          <KanbanColumn
            status="pushed"
            title="PUSHED"
            description="Synchronized & verified with external accounting"
            vouchers={pushed}
          />

          {/* Column 3: Failed */}
          <KanbanColumn
            status="failed"
            title="FAILED"
            description="Max retry attempts exhausted; operator retry ready"
            vouchers={failed}
            onRetry={handleRetry}
            retryingId={retryingId}
          />
        </main>
      </div>

      {/* Simulation Modal Dialog */}
      <SimulateModal
        isOpen={showSimModal}
        onClose={() => setShowSimModal(false)}
        onSubmit={handleCreateReceipt}
        loading={simulating}
      />
    </div>
  );
}
