/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      animation: {
        // Glitch effects
        glitch: 'glitch 0.3s ease-in-out',
        'glitch-loop': 'glitch 2s ease-in-out infinite',

        // Scanline effect
        scanline: 'scanline 8s linear infinite',

        // Neon pulse
        'neon-pulse': 'neon-pulse 2s ease-in-out infinite',

        // Float effect
        float: 'float 6s ease-in-out infinite',

        // Cyber border flow
        'border-flow': 'border-flow 3s linear infinite',

        // Holographic shimmer
        holographic: 'holographic 5s ease-in-out infinite',

        // Data stream
        'data-stream': 'data-stream 2s linear infinite',

        // Glow pulse
        'glow-pulse': 'glow-pulse 2s ease-in-out infinite',

        // Typing cursor
        'cursor-blink': 'cursor-blink 1s step-end infinite',
      },
      keyframes: {
        glitch: {
          '0%, 100%': {
            transform: 'translate(0)',
            filter: 'hue-rotate(0deg)',
          },
          '20%': {
            transform: 'translate(-2px, 2px)',
            filter: 'hue-rotate(90deg)',
          },
          '40%': {
            transform: 'translate(-2px, -2px)',
            filter: 'hue-rotate(180deg)',
          },
          '60%': {
            transform: 'translate(2px, 2px)',
            filter: 'hue-rotate(270deg)',
          },
          '80%': {
            transform: 'translate(2px, -2px)',
            filter: 'hue-rotate(360deg)',
          },
        },
        scanline: {
          '0%': { transform: 'translateY(-100%)' },
          '100%': { transform: 'translateY(100vh)' },
        },
        'neon-pulse': {
          '0%, 100%': {
            boxShadow:
              '0 0 5px currentColor, 0 0 10px currentColor, 0 0 20px currentColor',
            opacity: '1',
          },
          '50%': {
            boxShadow:
              '0 0 10px currentColor, 0 0 20px currentColor, 0 0 40px currentColor, 0 0 60px currentColor',
            opacity: '0.8',
          },
        },
        float: {
          '0%, 100%': { transform: 'translateY(0px)' },
          '50%': { transform: 'translateY(-10px)' },
        },
        'border-flow': {
          '0%': { backgroundPosition: '0% 50%' },
          '50%': { backgroundPosition: '100% 50%' },
          '100%': { backgroundPosition: '0% 50%' },
        },
        holographic: {
          '0%': {
            backgroundPosition: '0% 50%',
            filter: 'hue-rotate(0deg)',
          },
          '50%': {
            backgroundPosition: '100% 50%',
            filter: 'hue-rotate(180deg)',
          },
          '100%': {
            backgroundPosition: '0% 50%',
            filter: 'hue-rotate(360deg)',
          },
        },
        'data-stream': {
          '0%': { transform: 'translateY(-100%)', opacity: '0' },
          '50%': { opacity: '1' },
          '100%': { transform: 'translateY(100%)', opacity: '0' },
        },
        'glow-pulse': {
          '0%, 100%': {
            textShadow:
              '0 0 10px currentColor, 0 0 20px currentColor, 0 0 40px currentColor',
          },
          '50%': {
            textShadow:
              '0 0 20px currentColor, 0 0 40px currentColor, 0 0 80px currentColor',
          },
        },
        'cursor-blink': {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0' },
        },
      },
      backgroundImage: {
        'cyber-grid': `
          linear-gradient(rgba(168, 85, 247, 0.1) 1px, transparent 1px),
          linear-gradient(90deg, rgba(168, 85, 247, 0.1) 1px, transparent 1px)
        `,
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        holographic:
          'linear-gradient(135deg, #a855f7, #3b82f6, #06b6d4, #ec4899, #a855f7)',
      },
      backgroundSize: {
        'cyber-grid': '50px 50px',
      },
      boxShadow: {
        neon: '0 0 5px currentColor, 0 0 10px currentColor, 0 0 20px currentColor',
        'neon-strong':
          '0 0 10px currentColor, 0 0 20px currentColor, 0 0 40px currentColor, 0 0 60px currentColor',
      },
    },
  },
  plugins: [],
}
