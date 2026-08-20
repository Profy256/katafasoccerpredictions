import type { MetadataRoute } from 'next';

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: 'Katafa — Football Predictions With Receipts',
    short_name: 'Katafa',
    description:
      'Statistically generated football predictions with a public auto-graded accuracy record for every pick.',
    start_url: '/',
    display: 'standalone',
    // Matches --color-canvas so the splash screen and the app shell are the
    // same colour; the mark's own navy (#0f1e2e) is a shade lighter.
    background_color: '#0a0e15',
    theme_color: '#0a0e15',
    icons: [
      // Served by the app/icon.png file convention — no second copy in public/.
      { src: '/icon.png', sizes: '192x192', type: 'image/png' },
      { src: '/brand/katafa-mark-512.png', sizes: '512x512', type: 'image/png' },
      {
        // Opaque variant — stores and launchers that composite onto a light
        // background would otherwise show a green mark on white.
        src: '/brand/katafa-mark-512-navy.png',
        sizes: '512x512',
        type: 'image/png',
      },
    ],
  };
}
