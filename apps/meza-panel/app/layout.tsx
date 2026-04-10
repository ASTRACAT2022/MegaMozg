import type { Metadata } from "next";

import "./globals.css";

const themeInitScript = `
(() => {
  try {
    const savedTheme = window.localStorage.getItem("meza-theme");
    const hasValidSavedTheme = savedTheme === "light" || savedTheme === "dark";
    const systemThemeIsDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    const initialTheme = hasValidSavedTheme ? savedTheme : (systemThemeIsDark ? "dark" : "light");
    document.documentElement.setAttribute("data-theme", initialTheme);
    document.documentElement.style.colorScheme = initialTheme;
  } catch {
    document.documentElement.setAttribute("data-theme", "light");
    document.documentElement.style.colorScheme = "light";
  }
})();
`;

export const metadata: Metadata = {
  title: "MezaMozg Panel",
  description: "Панель оркестрации серверной инфраструктуры",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="ru" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
      </head>
      <body suppressHydrationWarning>{children}</body>
    </html>
  );
}
