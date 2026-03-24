"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/components/auth-provider";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";

export function AppShell({ children }: { children: React.ReactNode }) {
  const { isAuthed, verifying, login } = useAuth();
  const [tokenInput, setTokenInput] = useState("");
  const [logging, setLogging] = useState(false);
  const [error, setError] = useState("");

  const handleLogin = async () => {
    const t = tokenInput.trim();
    if (!t) return;

    setLogging(true);
    setError("");
    const ok = await login(t);
    setLogging(false);

    if (ok) {
      setTokenInput("");
      setError("");
    } else {
      setError("Invalid admin token");
      toast.error("Invalid admin token");
    }
  };

  if (verifying) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <p className="text-muted-foreground">Verifying...</p>
      </div>
    );
  }

  if (!isAuthed) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="w-full max-w-sm space-y-6 p-8">
          <div className="text-center space-y-2">
            <h1 className="text-2xl font-bold">ClawHost Admin</h1>
            <p className="text-muted-foreground text-sm">
              Enter your admin token to continue
            </p>
          </div>
          <div className="space-y-4">
            <div className="space-y-2">
              <Input
                type="password"
                placeholder="Admin Token"
                value={tokenInput}
                onChange={(e) => {
                  setTokenInput(e.target.value);
                  setError("");
                }}
                onKeyDown={(e) => e.key === "Enter" && handleLogin()}
                disabled={logging}
              />
              {error && (
                <p className="text-sm text-destructive">{error}</p>
              )}
            </div>
            <Button
              className="w-full"
              onClick={handleLogin}
              disabled={logging || !tokenInput.trim()}
            >
              {logging ? "Verifying..." : "Sign In"}
            </Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <SidebarProvider
      style={
        {
          "--sidebar-width": "calc(var(--spacing) * 72)",
          "--header-height": "calc(var(--spacing) * 12)",
        } as React.CSSProperties
      }
    >
      <AppSidebar variant="inset" />
      <SidebarInset>
        <SiteHeader />
        <div className="flex flex-1 flex-col overflow-y-auto">
          {children}
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}
