/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        'fl-bg':        '#0b0f17',
        'fl-surface':   '#0e1422',
        'fl-card':      '#131a2c',
        'fl-border':    '#1e293b',
        'fl-rose':      '#e11d48',
        'fl-sky':       '#0284c7',
        'fl-emerald':   '#059669',
        'fl-amber':     '#d97706',
      },
      fontFamily: {
        sans: ['Inter', '-apple-system', 'BlinkMacSystemFont', '"Segoe UI"', 'Roboto', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      borderRadius: {
        DEFAULT: '4px',
        md: '6px',
        lg: '8px',
      },
    },
  },
  plugins: [],
}
