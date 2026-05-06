import type { Config } from 'tailwindcss';

const config: Config = {
  content: ['./index.html', './src/**/*.{ts,tsx}', './tests/**/*.{ts,tsx}'],
  theme: {
    extend: {},
  },
  plugins: [],
};

export default config;
