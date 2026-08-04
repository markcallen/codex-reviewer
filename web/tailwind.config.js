/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,jsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Inter"', "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ['"JetBrains Mono"', "ui-monospace", "SFMono-Regular", "monospace"],
      },
      colors: {
        ink: "#10151c",
        paper: "#f7f7f2",
        line: "#d7d8cf",
        steel: "#54606f",
        signal: "#1f8a70",
        ember: "#d75f32",
      },
      boxShadow: {
        glow: "0 24px 80px rgba(31, 138, 112, 0.18)",
      },
    },
  },
  plugins: [],
};
