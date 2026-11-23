/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        // FaultLine design system tokens mapped as Tailwind colors
        'fl-bg':        '#070a12',
        'fl-surface':   '#0d121f',
        'fl-card':      '#121829',
        'fl-border':    '#1e293b',
        'fl-rose':      '#f43f5e',
        'fl-blue':      '#3b82f6',
        'fl-emerald':   '#10b981',
        'fl-amber':     '#f59e0b',
      },
      fontFamily: {
        sans: ['Plus Jakarta Sans', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'monospace'],
      },
      boxShadow: {
        'card': '0 4px 20px -2px rgba(0, 0, 0, 0.4)',
        'glow-rose': '0 0 20px rgba(244, 63, 94, 0.3)',
        'glow-blue': '0 0 20px rgba(59, 130, 246, 0.3)',
      },
    },
  },
  plugins: [],
}
