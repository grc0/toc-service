/* ══════════════════════════════════════════════════════════════════
   Shared Tailwind config for index.html and local.html.
   Load after tailwindcss.js and alongside theme.css, which defines
   every custom property referenced below.
   ══════════════════════════════════════════════════════════════════ */

tailwind.config = {
  // Classes applied from JS never appear in the markup Tailwind scans.
  safelist: [
    'bg-accent-600', 'text-accent-600', 'border-accent-600', 'ring-accent-500',
    'text-white', 'text-muted', 'text-faint', 'hover:text-content',
    'border-transparent',
  ],
  theme: {
    extend: {
      colors: {
        accent: {
          50: 'var(--c-accent-50)', 100: 'var(--c-accent-100)', 500: 'var(--c-accent-500)',
          600: 'var(--c-accent-600)', 700: 'var(--c-accent-700)', 800: 'var(--c-accent-800)',
        },
        base: {
          50: 'var(--c-base-50)', 100: 'var(--c-base-100)', 200: 'var(--c-base-200)',
          300: 'var(--c-base-300)', 400: 'var(--c-base-400)', 500: 'var(--c-base-500)',
          600: 'var(--c-base-600)', 700: 'var(--c-base-700)', 800: 'var(--c-base-800)',
          900: 'var(--c-base-900)',
        },
        // Semantic tokens — these follow the active theme
        surface: 'var(--c-surface)',
        raised: 'var(--c-surface-raised)',
        line: { DEFAULT: 'var(--c-line)', strong: 'var(--c-line-strong)' },
        content: 'var(--c-text)',
        muted: 'var(--c-text-muted)',
        faint: 'var(--c-text-faint)',
        tint: 'var(--c-tint)',
        danger: { DEFAULT: 'var(--c-danger)', tint: 'var(--c-danger-tint)' },
      },
    },
  },
};
