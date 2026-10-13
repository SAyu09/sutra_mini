/**
 * Formats integer paise into Indian Rupee currency format (₹).
 * Invariant: Never show raw paise to the user.
 */
export function formatPaiseToINR(amountPaise: number): string {
  const rupees = (amountPaise || 0) / 100;
  return new Intl.NumberFormat('en-IN', {
    style: 'currency',
    currency: 'INR',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(rupees);
}

/**
 * Formats an ISO timestamp string into a friendly localized display.
 */
export function formatDate(isoStr: string): string {
  try {
    const d = new Date(isoStr);
    return (
      d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) +
      ' · ' +
      d.toLocaleDateString([], { month: 'short', day: 'numeric' })
    );
  } catch {
    return isoStr;
  }
}

/**
 * Formats relative time until a future timestamp (e.g. for next_attempt_at).
 */
export function formatNextAttempt(isoStr: string): string {
  try {
    const target = new Date(isoStr).getTime();
    const now = Date.now();
    const diffSec = Math.round((target - now) / 1000);
    if (diffSec <= 0) return 'Immediate';
    if (diffSec < 60) return `in ${diffSec}s`;
    const diffMin = Math.round(diffSec / 60);
    return `in ${diffMin}m`;
  } catch {
    return 'Pending';
  }
}

/**
 * Copies string to clipboard with fallback.
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Fallback if clipboard API is unavailable
  }
  return false;
}
