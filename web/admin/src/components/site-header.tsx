"use client"

import { usePathname, useSearchParams } from "next/navigation"
import { Suspense } from "react"
import { Separator } from "@/components/ui/separator"
import { SidebarTrigger } from "@/components/ui/sidebar"

const pageTitles: Record<string, string> = {
  "/": "Apps",
  "/bots": "Bots",
}

function SiteHeaderInner() {
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const normalized = pathname.replace(/\/+$/, "") || "/"

  // If on bots page with an id param, show "Bot Detail" (will be enhanced later)
  const botId = normalized === "/bots" ? searchParams.get("id") : null
  const tab = searchParams.get("tab")

  let title = pageTitles[normalized] || pageTitles["/"]
  const breadcrumbs: { label: string; href?: string }[] = []

  if (botId) {
    breadcrumbs.push({ label: "Bots", href: "/admin/bots" })
    breadcrumbs.push({ label: "Detail" })
    if (tab) {
      breadcrumbs.push({ label: tab.charAt(0).toUpperCase() + tab.slice(1) })
    }
    title = ""
  }

  return (
    <header className="flex h-(--header-height) shrink-0 items-center gap-2 border-b transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-(--header-height)">
      <div className="flex w-full items-center gap-1 px-4 lg:gap-2 lg:px-6">
        <SidebarTrigger className="-ml-1" />
        <Separator
          orientation="vertical"
          className="mx-2 h-4 data-vertical:self-auto"
        />
        {breadcrumbs.length > 0 ? (
          <nav className="flex items-center gap-1 text-sm">
            {breadcrumbs.map((crumb, i) => (
              <span key={i} className="flex items-center gap-1">
                {i > 0 && <span className="text-muted-foreground">/</span>}
                {crumb.href ? (
                  <a href={crumb.href} className="text-muted-foreground hover:text-foreground transition-colors">
                    {crumb.label}
                  </a>
                ) : (
                  <span className="font-medium">{crumb.label}</span>
                )}
              </span>
            ))}
          </nav>
        ) : (
          <h1 className="text-base font-medium">{title}</h1>
        )}
      </div>
    </header>
  )
}

export function SiteHeader() {
  return (
    <Suspense fallback={
      <header className="flex h-(--header-height) shrink-0 items-center gap-2 border-b">
        <div className="flex w-full items-center gap-1 px-4 lg:gap-2 lg:px-6">
          <div className="h-4 w-20 bg-muted animate-pulse rounded" />
        </div>
      </header>
    }>
      <SiteHeaderInner />
    </Suspense>
  )
}
