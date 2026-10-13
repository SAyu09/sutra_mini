'use client';

import React from 'react';
import { CheckCircle2, AlertTriangle, X } from 'lucide-react';

interface ToastProps {
  toast: { message: string; type: 'success' | 'error' } | null;
  onClose: () => void;
}

export function Toast({ toast, onClose }: ToastProps) {
  if (!toast) return null;

  return (
    <div className={`toast-notification ${toast.type}`} role="status">
      <div className="toast-icon-wrap">
        {toast.type === 'success' ? (
          <CheckCircle2 size={16} />
        ) : (
          <AlertTriangle size={16} />
        )}
      </div>
      <span className="toast-text">{toast.message}</span>
      <button
        type="button"
        className="toast-close-btn"
        onClick={onClose}
        aria-label="Dismiss alert"
      >
        <X size={14} />
      </button>
    </div>
  );
}
