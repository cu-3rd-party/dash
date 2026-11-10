import type {Config} from 'tailwindcss';

// Палитра темы cu3rd из Forgejo (homelab apps/forgejo/values.yaml и forgejo-dark, на котором она основана),
// как в cu3rd/paste и cu3rd/db-access. Цвета сайта меняются здесь, шаблоны используют только эти имена.
export default {
  content: ['./templates/**/*.tmpl', './web_src/js/**/*.ts'],
  theme: {
    extend: {
      colors: {
        page: '#050505',
        line: '#2e2e2e',
        code: '#0a0a0a',
        box: {DEFAULT: '#0f0f0f', header: '#151515'},
        hover: '#1c1c1c',
        ink: {DEFAULT: '#dddddd', light: '#b1b1b1', muted: '#8a8a8a'},
        primary: {DEFAULT: '#dddddd', hover: '#c4c4c4', contrast: '#050505'},
        secondary: {DEFAULT: '#2e2e2e', hover: '#3a3a3a'},
        warning: {bg: '#644821', border: '#a67a1d', text: '#fff388'},
        danger: {bg: '#5f2525', border: '#783030', text: '#fef2f2', fg: '#f87171'},
        success: {bg: '#14532d', border: '#166534', text: '#dcfce7', fg: '#4ade80'},
      },
      fontFamily: {
        sans: ['-apple-system', '"Segoe UI"', 'system-ui', '"Noto Sans"', 'Roboto', '"Helvetica Neue"', 'Arial', '"Liberation Sans"', 'sans-serif'],
        mono: ['ui-monospace', 'SFMono-Regular', '"SF Mono"', 'Menlo', 'Monaco', 'Consolas', '"Liberation Mono"', '"Courier New"', 'monospace'],
      },
      borderRadius: {box: '4px'},
    },
  },
  plugins: [],
} satisfies Config;
