"use client";

import { FormEvent, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { getLoginAccountHints, login } from "@/lib/auth";

function isSafeRedirectTarget(target: string | null): target is string {
  return Boolean(target && target.startsWith("/") && !target.startsWith("//"));
}

export default function LoginPage() {
  const t = useTranslations("loginPage");
  const router = useRouter();
  const searchParams = useSearchParams();
  const nextTarget = searchParams?.get("next") || null;
  const accounts = useMemo(() => getLoginAccountHints(), []);
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("admin");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError("");
    setSubmitting(true);

    try {
      login(username, password);
      router.replace(isSafeRedirectTarget(nextTarget) ? nextTarget : "/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : t("loginFailed"));
      setSubmitting(false);
    }
  };

  return (
    <main className="min-h-screen bg-gradient-to-br from-background via-muted/40 to-background px-4 py-12">
      <div className="mx-auto grid w-full max-w-5xl gap-6 lg:grid-cols-[1fr_1.15fr]">
        <section className="rounded-2xl border border-border bg-card p-6 shadow-sm lg:p-8">
          <div className="inline-flex items-center gap-2 rounded-md bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary">
            <ShieldCheck className="h-3.5 w-3.5" />
            {t("brand")}
          </div>
          <h1 className="mt-4 text-2xl font-semibold text-foreground">{t("title")}</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {t("description")}
          </p>

          <form className="mt-6 space-y-4" onSubmit={handleSubmit}>
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-foreground" htmlFor="username">
                {t("username")}
              </label>
              <Input
                id="username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="username"
                disabled={submitting}
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

            {error && (
              <div className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">
                {error}
              </div>
            )}

            <Button type="submit" className="w-full" disabled={submitting}>
              {submitting ? t("signingIn") : t("signIn")}
            </Button>
          </form>
        </section>

        <section className="rounded-2xl border border-border bg-card p-6 shadow-sm lg:p-8">
          <h2 className="text-lg font-semibold text-card-foreground">{t("testUsersTitle")}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("testUsersDescription")}
          </p>

          <div className="mt-5 space-y-3">
            {accounts.map((account) => (
              <button
                key={account.username}
                type="button"
                onClick={() => {
                  setUsername(account.username);
                  setPassword(account.password);
                  setError("");
                }}
                className="w-full rounded-lg border border-border bg-background/70 px-4 py-3 text-left transition-colors hover:bg-muted/60"
              >
                <div className="text-sm font-medium text-foreground">
                  {account.username} / {account.password}
                </div>
                <div className="mt-1 text-xs text-muted-foreground">{account.note}</div>
              </button>
            ))}
          </div>
        </section>
      </div>
    </main>
  );
}
