import { getRequestConfig } from "next-intl/server";
import { cookies, headers } from "next/headers";
import { locales, defaultLocale, type Locale } from "./config";

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
