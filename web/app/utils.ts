/**
 * Formats integer paise into Indian Rupee currency format (₹).
 * Handles edge cases: 0 paise, small amounts (< 100 paise), and large values.
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
      ' ' +
      d.toLocaleDateString([], { month: 'short', day: 'numeric' })
    );
  } catch {
    return isoStr;
  }
}
