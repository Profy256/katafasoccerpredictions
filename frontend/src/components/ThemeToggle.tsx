'use client';

import { useCallback } from 'react';

const STORAGE_KEY = 'katafa-theme';

function apply(theme: 'light' | 'dark') {
  document.documentElement.classList.toggle('dark', theme === 'dark');
}

/**
 * Light/dark switch. Renders both icons and lets CSS decide visibility (see
 * `.theme-icon-*` in globals.css), so server and client markup always match —
 * the real theme is only known after the pre-paint script runs.
 */
export function ThemeToggle() {
  const toggle = useCallback(() => {
    const dark = document.documentElement.classList.contains('dark');
    const next = dark ? 'light' : 'dark';
    apply(next);
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // Private mode etc. — the visual state is already applied.
    }
  }, []);

  return (
    <button
      type="button"
      onClick={toggle}
      aria-label="Toggle colour theme"
      title="Toggle theme"
      className="flex h-8 w-8 items-center justify-center rounded-lg border border-line text-fg-muted transition-colors hover:border-brand/40 hover:text-brand"
    >
      {/* Sun — visible in dark mode, offers switching to light. */}
      <svg
        aria-hidden
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        className="theme-icon-light h-4 w-4"
      >
        <circle cx="12" cy="12" r="4" />
        <path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4m11.4-11.4 1.4-1.4" />
      </svg>
      {/* Moon — visible in light mode, offers switching to dark. */}
      <svg
        aria-hidden
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="theme-icon-dark h-4 w-4"
      >
        <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z" />
      </svg>
    </button>
  );
}
