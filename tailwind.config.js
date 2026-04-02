module.exports = {
  darkMode: ['class', '[data-theme="dark"]'],
  content: [
    './internal/views/**/*.templ',
    './internal/views/**/*.go',
    './cmd/**/*.go'
  ],
  theme: {
    extend: {
      colors: {
        surface: '#0b1326',
        'surface-dim': '#0b1326',
        'surface-container-lowest': '#060e20',
        'surface-container-low': '#131b2e',
        'surface-container': '#171f33',
        'surface-container-high': '#222a3d',
        'surface-container-highest': '#2d3449',
        'surface-variant': '#2d3449',
        outline: '#88929b',
        'outline-variant': '#3e4850',
        primary: '#89ceff',
        'primary-fixed-dim': '#89ceff',
        secondary: '#b7c8e1',
        'secondary-fixed-dim': '#b7c8e1',
        tertiary: '#ffb86e',
        'tertiary-fixed-dim': '#ffb86e',
        'primary-container': '#0ea5e9',
        'on-primary-container': '#003751',
        'on-surface': '#dae2fd',
        'on-surface-variant': '#bec8d2',
        success: '#79f2b3',
        warning: '#ffcf7d',
        critical: '#ff8f87'
      },
      fontFamily: {
        headline: ['Manrope', 'sans-serif'],
        body: ['Inter', 'sans-serif'],
        mono: ['JetBrains Mono', 'monospace']
      },
      boxShadow: {
        glow: '0 0 0 1px rgba(137,206,255,0.1), 0 10px 40px rgba(8,15,30,0.35)'
      },
      backgroundImage: {
        'mesh-dark': 'radial-gradient(circle at top left, rgba(137,206,255,0.18), transparent 30%), radial-gradient(circle at top right, rgba(255,184,110,0.12), transparent 35%), linear-gradient(180deg, rgba(11,19,38,1) 0%, rgba(6,14,32,1) 100%)',
        'mesh-light': 'radial-gradient(circle at top left, rgba(14,165,233,0.10), transparent 30%), radial-gradient(circle at top right, rgba(255,184,110,0.15), transparent 35%), linear-gradient(180deg, #eef6ff 0%, #dfeaf7 100%)'
      }
    }
  },
  plugins: []
}
