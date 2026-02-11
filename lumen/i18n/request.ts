import { getRequestConfig } from "next-intl/server";
import { cookies, headers } from "next/headers";

export const locales = ["en", "zh-CN", "zh-TW", "ja", "fr"] as const;
export type Locale = (typeof locales)[number];
export const defaultLocale: Locale = "en";

export const localeNames: Record<Locale, string> = {
  en: "English",
  "zh-CN": "简体中文",
  "zh-TW": "繁體中文",
  ja: "日本語",
  fr: "Français",
};

export default getRequestConfig(async () => {
  const cookieStore = await cookies();
  const headerStore = await headers();

  let locale: Locale = defaultLocale;

  const cookieLocale = cookieStore.get("NEXT_LOCALE")?.value;
  if (cookieLocale && locales.includes(cookieLocale as Locale)) {
    locale = cookieLocale as Locale;
  } else {
    const acceptLanguage = headerStore.get("accept-language") ?? "";
    const preferred = acceptLanguage.split(",").map((l) => l.split(";")[0].trim());
    for (const lang of preferred) {
      if (locales.includes(lang as Locale)) {
        locale = lang as Locale;
        break;
      }
      const prefix = lang.split("-")[0];
      const match = locales.find((l) => l.startsWith(prefix));
      if (match) {
        locale = match;
        break;
      }
    }
  }

  return {
    locale,
    messages: (await import(`../messages/${locale}.json`)).default,
  };
});
