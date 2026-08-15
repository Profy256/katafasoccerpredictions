import type { Metadata, Viewport } from 'next';
import { Geist, Geist_Mono } from 'next/font/google';
import './globals.css';
import { SiteHeader } from '@/components/SiteHeader';
import { SiteFooter } from '@/components/SiteFooter';
import { DemoDataBanner } from '@/components/DemoDataBanner';
import { getSession } from '@/lib/session';

const geistSans = Geist({
  variable: '--font-geist-sans',
  subsets: ['latin'],
});

const geistMono = Geist_Mono({
  variable: '--font-geist-mono',
  subsets: ['latin'],
});

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL ?? 'http://localhost:3000';

export const metadata: Metadata = {
  // Without this the file-convention og/twitter images resolve to relative
  // URLs, which crawlers reject.
  metadataBase: new URL(SITE_URL),
  title: {
    default: 'Katafa — Football Predictions With Receipts',
    template: '%s · Katafa',
  },
  description:
    'Statistically generated football predictions across 1X2, Double Chance, BTTS and Over/Under markets, with a public auto-graded accuracy record for every pick.',
  openGraph: {
    type: 'website',
    siteName: 'Katafa',
    url: '/',
  },
  twitter: { card: 'summary_large_image' },
};

export const viewport: Viewport = {
  themeColor: '#0a0e15',
};

export default async function RootLayout({ children }: LayoutProps<'/'>) {
  const session = await getSession();

  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="flex min-h-full flex-col bg-canvas text-fg">
        <DemoDataBanner />
        <SiteHeader signedIn={Boolean(session)} />
        <main className="flex-1">{children}</main>
        <SiteFooter />
      </body>
    </html>
  );
}
