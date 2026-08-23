import type { Metadata, Viewport } from 'next';
import { Geist, Geist_Mono } from 'next/font/google';
import './globals.css';
import { SiteHeader } from '@/components/SiteHeader';
import { SiteFooter } from '@/components/SiteFooter';
import { getSession } from '@/api/client';

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
    'Free daily football predictions and soccer tips across Match Result (1X2), Double Chance, Both Teams To Score and Over/Under goals — plus a public, auto-graded accuracy record for every pick, wins and losses alike.',
  openGraph: {
    type: 'website',
    siteName: 'Katafa',
    url: '/',
  },
  twitter: { card: 'summary_large_image' },
  // No blanket `alternates.canonical` here on purpose: Next merges metadata
  // shallowly from layout to page, so setting one here would apply to every
  // route that doesn't override it — telling Google that /accuracy,
  // /methodology and every other page is a duplicate of '/'. Each page sets
  // its own canonical, or leaves it unset and lets it default to its real URL.
};

/**
 * Organization + WebSite JSON-LD, once, site-wide. Not per-page — a search
 * result that can name the publisher and link straight to the accuracy
 * record is worth more here than a rich-result attempt on every route.
 */
const SITE_JSON_LD = {
  '@context': 'https://schema.org',
  '@graph': [
    {
      '@type': 'Organization',
      name: 'Katafa',
      url: SITE_URL,
      logo: `${SITE_URL}/icon.png`,
      description:
        'Football predictions platform publishing free daily statistical tips and paid analyst slips, both graded automatically against a public accuracy record.',
    },
    {
      '@type': 'WebSite',
      name: 'Katafa',
      url: SITE_URL,
    },
  ],
};

export const viewport: Viewport = {
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: '#f5f7fa' },
    { media: '(prefers-color-scheme: dark)', color: '#0a0e15' },
  ],
};

/**
 * Runs before first paint so the theme class is already on <html> when CSS
 * lands — no flash of the wrong theme. Stored choice wins; otherwise light.
 */
const THEME_INIT = `(function(){try{var t=localStorage.getItem('katafa-theme');if(t!=='light'&&t!=='dark'){t='light'}document.documentElement.classList.toggle('dark',t==='dark')}catch(e){}})()`;

export default async function RootLayout({ children }: LayoutProps<'/'>) {
  const session = await getSession();

  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
      suppressHydrationWarning
    >
      <head>
        <script dangerouslySetInnerHTML={{ __html: THEME_INIT }} />
      </head>
      <body className="flex min-h-full flex-col bg-canvas text-fg">
        {/* Static JSON-LD, no user input. */}
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: JSON.stringify(SITE_JSON_LD) }}
        />
        <SiteHeader signedIn={Boolean(session)} />
        <main className="flex-1">{children}</main>
        <SiteFooter />
      </body>
    </html>
  );
}
