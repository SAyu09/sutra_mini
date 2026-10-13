import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'Sutra Mini | Shree Radha Studio Accounting Sync',
  description: 'Resilient accounting voucher integration portal for Shree Radha Studio, Delhi',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
