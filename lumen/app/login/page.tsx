"use client";

import { FormEvent, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { login, registerTenant } from "@/lib/auth";

function isSafeRedirectTarget(target: string | null): target is string {
  return Boolean(target && target.startsWith("/") && !target.startsWith("//"));
}

export default function LoginPage() {
  const t = useTranslations("loginPage");
  const router = useRouter();
  const searchParams = useSearchParams();
  const nextTarget = searchParams?.get("next") || null;
  const [mode, setMode] = useState<"login" | "register">("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError("");
    if (!username.trim()) {
      setError(t("tenantRequired"));
      return;
    }
    if (!password) {
      setError(t("passwordRequired"));
      return;
    }
    setSubmitting(true);

    try {
      if (mode === "register") {
        await registerTenant(username, password, displayName);
      } else {
        await login(username, password);
      }
      router.replace(isSafeRedirectTarget(nextTarget) ? nextTarget : "/dashboard");
    } catch (err) {
      const fallback = mode === "register" ? t("registerFailed") : t("loginFailed");
      setError(err instanceof Error ? err.message : fallback);
      setSubmitting(false);
    }
  };

  return (
    <main className="min-h-screen bg-gradient-to-br from-background via-muted/40 to-background px-4 py-12">
      <div className="mx-auto w-full max-w-md">
        <section className="rounded-2xl border border-border bg-card p-6 shadow-sm lg:p-8">
          <div className="inline-flex items-center gap-2 rounded-md bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">
            <ShieldCheck className="h-3.5 w-3.5" />
            {t("brand")}
          </div>
          <h1 className="mt-4 text-2xl font-semibold text-foreground">
            {mode === "register" ? t("registerTitle") : t("title")}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {mode === "register" ? t("registerDescription") : t("description")}
          </p>

          <form className="mt-6 space-y-4" onSubmit={handleSubmit}>
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-foreground" htmlFor="username">
                {t("tenant")}
              </label>
              <Input
                id="username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="username"
                disabled={submitting}
                placeholder={t("tenantPlaceholder")}
                required
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-foreground" htmlFor="password">
                {t("password")}
              </label>
              <Input
                id="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="current-password"
                type="password"
                disabled={submitting}
                required
              />
            </div>

            {mode === "register" && (
              <div className="space-y-1.5">
                <label className="text-sm font-medium text-foreground" htmlFor="displayName">
                  {t("displayName")}
                </label>
                <Input
                  id="displayName"
                  value={displayName}
                  onChange={(event) => setDisplayName(event.target.value)}
                  autoComplete="organization"
                  disabled={submitting}
                  placeholder={t("displayNamePlaceholder")}
                />
              </div>
            )}

            {error && (
              <div className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">
                {error}
              </div>
            )}

            <Button type="submit" className="w-full" disabled={submitting}>
              {submitting
                ? mode === "register"
                  ? t("signingUp")
                  : t("signingIn")
                : mode === "register"
                  ? t("signUp")
                  : t("signIn")}
            </Button>

            <Button
              type="button"
              variant="ghost"
              className="w-full text-xs text-muted-foreground"
              onClick={() => {
                setMode((prev) => (prev === "login" ? "register" : "login"));
                setError("");
              }}
              disabled={submitting}
            >
              {mode === "login" ? t("switchToRegister") : t("switchToLogin")}
            </Button>
          </form>
        </section>
      </div>
    </main>
  );
}
