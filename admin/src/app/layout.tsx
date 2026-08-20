import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: {
    default: 'Katafa admin',
    template: '%s · Katafa admin',
  },
  // Ops tooling, not a public product page — never indexed, never OG-carded.
  robots: { index: false, follow: false },
};

export default function RootLayout({ children }: LayoutProps<'/'>) {
  return (
    <html lang="en" className="h-full antialiased">
      <body className="min-h-full bg-canvas text-fg">{children}</body>
    </html>
  );
}
