import { defineConfig } from "blume";

export default defineConfig({
  title: "Open Monitoring",
  description:
    "Open-source PC monitoring for Windows — live dashboard, recordable sessions, and an always-on-top HUD overlay.",
  logo: {
    image: "/logo.svg",
    text: "Open Monitoring",
  },
  content: {
    root: "content",
  },
  theme: {
    accent: "teal",
    radius: "md",
    mode: "system",
  },
  navigation: {
    sidebar: [
      "/",
      "/installation",
      {
        label: "Using the app",
        items: ["/dashboard", "/hud", "/recording", "/system-panel"],
      },
      {
        label: "Under the hood",
        items: ["/pawnio", "/metrics", "/building"],
      },
      "/faq",
      {
        label: "GitHub",
        href: "https://github.com/alexdedyura/open-monitoring",
      },
    ],
  },
  seo: {
    og: { enabled: true },
    sitemap: true,
    robots: true,
    structuredData: true,
  },
  ai: {
    llmsTxt: true,
  },
  deployment: {
    output: "static",
    site: "https://alexdedyura.github.io",
    base: "/open-monitoring",
  },
});
