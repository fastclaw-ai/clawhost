"use client";

import { useEffect, useState, useCallback } from "react";
import { useSearchParams } from "next/navigation";
import { Suspense } from "react";
import { toast } from "sonner";
import { BotDetail } from "@/components/bot-detail";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ExternalLinkIcon } from "lucide-react";
import { useAuth } from "@/components/auth-provider";
import {
  getAdminConfig,
  listApps,
  listBots,
  createBot,
  startBot,
  stopBot,
  deleteBot,
  upgradeBot,
  upgradeAllBots,
  restartAllBots,
  type App,
  type Bot,
} from "@/lib/api";

const statusStyles: Record<string, string> = {
  running: "bg-green-100 text-green-700 border-green-200",
  starting: "bg-yellow-100 text-yellow-700 border-yellow-200",
  created: "bg-blue-100 text-blue-700 border-blue-200",
  stopped: "bg-gray-100 text-gray-600 border-gray-200",
  error: "bg-red-100 text-red-700 border-red-200",
};

function BotActions({
  bot,
  actionLoading,
  onAction,
  onDelete,
}: {
  bot: Bot;
  actionLoading: string | null;
  onAction: (action: () => Promise<unknown>, msg: string, id: string) => void;
  onDelete: () => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="inline-flex items-center justify-center rounded-md text-sm font-medium h-8 px-2 hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
        disabled={actionLoading === bot.id}
      >
        ...
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {bot.status !== "running" && (
          <DropdownMenuItem
            onClick={() => onAction(() => startBot(bot.id), "Bot started", bot.id)}
          >
            Start
          </DropdownMenuItem>
        )}
        {bot.status === "running" && (
          <DropdownMenuItem
            onClick={() => onAction(() => stopBot(bot.id), "Bot stopped", bot.id)}
          >
            Stop
          </DropdownMenuItem>
        )}
        <DropdownMenuItem
          onClick={() => onAction(() => upgradeBot(bot.id), "Bot upgraded", bot.id)}
        >
          Upgrade
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => {
            navigator.clipboard.writeText(bot.id);
            toast.success("Bot ID copied");
          }}
        >
          Copy ID
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => {
            navigator.clipboard.writeText(bot.access_token);
            toast.success("Access token copied");
          }}
        >
          Copy Token
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem className="text-destructive" onClick={onDelete}>
          Delete
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function BotsPageInner() {
  const { isAuthed } = useAuth();
  const searchParams = useSearchParams();
  const detailBotId = searchParams.get("id");
  const detailTab = searchParams.get("tab") || undefined;

  // If ?id= is present, render bot detail view
  if (detailBotId) {
    if (!isAuthed) return null;
    return <BotDetail botId={detailBotId} tab={detailTab} />;
  }
  const [bots, setBots] = useState<Bot[]>([]);
  const [appMap, setAppMap] = useState<Record<string, App>>({});
  const [globalDomainTemplate, setGlobalDomainTemplate] = useState("");
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Bot | null>(null);
  const [bulkAction, setBulkAction] = useState<"upgrade" | "restart" | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>("all");
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [createForm, setCreateForm] = useState({ name: "", app_id: "", user_id: "" });

  const fetchBots = useCallback(async () => {
    try {
      setLoading(true);
      const [botsRes, appsRes, configRes] = await Promise.all([
        listBots(),
        listApps(),
        getAdminConfig(),
      ]);
      setBots(botsRes.data || []);
      const map: Record<string, App> = {};
      for (const app of appsRes.data || []) {
        map[app.id] = app;
      }
      setAppMap(map);
      setGlobalDomainTemplate(configRes.data?.bot_domain_template || "");
    } catch (err) {
      toast.error("Failed to load bots: " + (err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  const getBotUrl = (bot: Bot): string => {
    const app = appMap[bot.app_id];
    const template = app?.bot_domain_template || globalDomainTemplate;
    if (!template) return "";
    return template
      .replace("{bot_id}", bot.slug)
      .replace("{slug}", bot.slug);
  };

  const getBotDomain = (bot: Bot): string => {
    const url = getBotUrl(bot);
    if (!url) return bot.slug;
    return url.replace(/^https?:\/\//, "");
  };

  useEffect(() => {
    if (isAuthed) fetchBots();
  }, [isAuthed, fetchBots]);

  const handleAction = async (
    action: () => Promise<unknown>,
    successMsg: string,
    botId?: string
  ) => {
    try {
      setActionLoading(botId || "bulk");
      await action();
      toast.success(successMsg);
      fetchBots();
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setActionLoading(null);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    await handleAction(() => deleteBot(deleteTarget.id), "Bot deleted", deleteTarget.id);
    setDeleteTarget(null);
  };

  const handleBulkAction = async () => {
    if (!bulkAction) return;
    if (bulkAction === "upgrade") {
      await handleAction(() => upgradeAllBots(), "All bots upgraded");
    } else {
      await handleAction(() => restartAllBots(), "All bots restarted");
    }
    setBulkAction(null);
  };

  const handleCreate = async () => {
    if (!createForm.name || !createForm.app_id) {
      toast.error("Name and App are required");
      return;
    }
    try {
      setActionLoading("create");
      const res = await createBot({
        name: createForm.name,
        app_id: createForm.app_id,
        user_id: createForm.user_id || undefined,
      });
      toast.success("Bot created, starting...");
      try {
        await startBot(res.data.id);
        toast.success("Bot started");
      } catch {
        toast.error("Bot created but failed to start");
      }
      setShowCreateDialog(false);
      setCreateForm({ name: "", app_id: "", user_id: "" });
      fetchBots();
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setActionLoading(null);
    }
  };

  if (!isAuthed) return null;

  const runningCount = bots.filter((b) => b.status === "running").length;
  const stoppedCount = bots.filter((b) => b.status === "stopped").length;
  const filteredBots = statusFilter === "all" ? bots : bots.filter((b) => b.status === statusFilter);

  return (
    <div className="p-4 md:p-6 space-y-4 md:space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl md:text-2xl font-bold">Bots</h1>
          <p className="text-muted-foreground text-sm">
            {bots.length} total &middot; {runningCount} running &middot;{" "}
            {stoppedCount} stopped
          </p>
        </div>
        <div className="flex gap-2 shrink-0 items-center">
          <Select value={statusFilter} onValueChange={(v) => setStatusFilter(v ?? "all")}>
            <SelectTrigger className="w-[130px] h-8 text-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Status</SelectItem>
              <SelectItem value="running">Running</SelectItem>
              <SelectItem value="starting">Starting</SelectItem>
              <SelectItem value="created">Created</SelectItem>
              <SelectItem value="stopped">Stopped</SelectItem>
              <SelectItem value="error">Error</SelectItem>
            </SelectContent>
          </Select>
          <Button size="sm" onClick={() => setShowCreateDialog(true)}>
            Create Bot
          </Button>
          <Button variant="outline" size="sm" onClick={() => setBulkAction("upgrade")}>
            Upgrade All
          </Button>
          <Button variant="outline" size="sm" onClick={() => setBulkAction("restart")}>
            Restart All
          </Button>
        </div>
      </div>

      {loading ? (
        <div className="text-center py-8 text-muted-foreground">Loading...</div>
      ) : filteredBots.length === 0 ? (
        <div className="text-center py-8 text-muted-foreground">No bots yet</div>
      ) : (
        <>
          {/* Desktop table */}
          <div className="hidden lg:block border rounded-lg">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>App</TableHead>
                  <TableHead>User ID</TableHead>
                  <TableHead>Domain</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="w-[80px]"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredBots.map((bot) => (
                  <TableRow key={bot.id}>
                    <TableCell>
                      <a
                        href={`/admin/bots/?id=${bot.id}`}
                        className="font-medium text-foreground hover:underline"
                      >
                        {bot.name}
                      </a>
                    </TableCell>
                    <TableCell className="text-sm">
                      {appMap[bot.app_id]?.name || bot.app_id.slice(0, 8)}
                    </TableCell>
                    <TableCell>
                      <code className="text-xs bg-muted px-1 py-0.5 rounded">
                        {bot.user_id.slice(0, 8)}
                      </code>
                    </TableCell>
                    <TableCell>
                      {getBotUrl(bot) ? (
                        <a
                          href={getBotUrl(bot)}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
                        >
                          <code>{getBotDomain(bot)}</code>
                          <ExternalLinkIcon className="size-3 shrink-0" />
                        </a>
                      ) : (
                        <code className="text-xs text-muted-foreground">{getBotDomain(bot)}</code>
                      )}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline" className={statusStyles[bot.status] || ""}>
                        {bot.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {new Date(bot.created_at).toLocaleDateString()}
                    </TableCell>
                    <TableCell>
                      <BotActions
                        bot={bot}
                        actionLoading={actionLoading}
                        onAction={handleAction}
                        onDelete={() => setDeleteTarget(bot)}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          {/* Mobile / Tablet cards */}
          <div className="lg:hidden space-y-3">
            {filteredBots.map((bot) => (
              <Card key={bot.id}>
                <CardContent className="p-4">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2 flex-wrap">
                        <a
                          href={`/admin/bots/?id=${bot.id}`}
                          className="font-medium hover:underline"
                        >
                          {bot.name}
                        </a>
                        <Badge
                          variant="outline"
                          className={`shrink-0 ${statusStyles[bot.status] || ""}`}
                        >
                          {bot.status}
                        </Badge>
                      </div>
                    </div>
                    <BotActions
                      bot={bot}
                      actionLoading={actionLoading}
                      onAction={handleAction}
                      onDelete={() => setDeleteTarget(bot)}
                    />
                  </div>
                  <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 text-xs text-muted-foreground">
                    <div className="col-span-2">
                      <span className="text-foreground/50">Domain</span>
                      {getBotUrl(bot) ? (
                        <a
                          href={getBotUrl(bot)}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="flex items-center gap-1 bg-muted px-1 py-0.5 rounded mt-0.5 hover:text-foreground transition-colors"
                        >
                          <code className="truncate">{getBotDomain(bot)}</code>
                          <ExternalLinkIcon className="size-3 shrink-0" />
                        </a>
                      ) : (
                        <code className="block bg-muted px-1 py-0.5 rounded mt-0.5 truncate">
                          {getBotDomain(bot)}
                        </code>
                      )}
                    </div>
                    <div>
                      <span className="text-foreground/50">App</span>
                      <p className="truncate">{appMap[bot.app_id]?.name || bot.app_id.slice(0, 8)}</p>
                    </div>
                    <div>
                      <span className="text-foreground/50">Created</span>
                      <p>{new Date(bot.created_at).toLocaleDateString()}</p>
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </>
      )}

      {/* Delete Confirmation */}
      <AlertDialog open={!!deleteTarget} onOpenChange={() => setDeleteTarget(null)}>
        <AlertDialogContent className="max-w-[95vw] sm:max-w-lg">
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Bot</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete &quot;{deleteTarget?.name}&quot;?
              {deleteTarget?.status === "running" &&
                " The running deployment will also be removed."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete}>Delete</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Bulk Action Confirmation */}
      <AlertDialog open={!!bulkAction} onOpenChange={() => setBulkAction(null)}>
        <AlertDialogContent className="max-w-[95vw] sm:max-w-lg">
          <AlertDialogHeader>
            <AlertDialogTitle>
              {bulkAction === "upgrade" ? "Upgrade All Bots" : "Restart All Bots"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {bulkAction === "upgrade"
                ? "This will upgrade all running bots to the latest version. Are you sure?"
                : "This will restart all running bots with a full pod spec rebuild. Are you sure?"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleBulkAction} disabled={actionLoading === "bulk"}>
              {actionLoading === "bulk" ? "Processing..." : "Confirm"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Create Bot Dialog */}
      <Dialog open={showCreateDialog} onOpenChange={setShowCreateDialog}>
        <DialogContent className="max-w-[95vw] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Create Bot</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <Label htmlFor="bot-name">Name *</Label>
              <Input
                id="bot-name"
                placeholder="My Bot"
                value={createForm.name}
                onChange={(e) => setCreateForm((f) => ({ ...f, name: e.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="bot-app">App *</Label>
              <Select
                value={createForm.app_id}
                onValueChange={(v) => setCreateForm((f) => ({ ...f, app_id: v ?? "" }))}
              >
                <SelectTrigger id="bot-app">
                  <SelectValue placeholder="Select an app" />
                </SelectTrigger>
                <SelectContent>
                  {Object.values(appMap).map((app) => (
                    <SelectItem key={app.id} value={app.id}>
                      {app.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="bot-user">User ID</Label>
              <Input
                id="bot-user"
                placeholder="Optional (defaults to admin)"
                value={createForm.user_id}
                onChange={(e) => setCreateForm((f) => ({ ...f, user_id: e.target.value }))}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowCreateDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={actionLoading === "create"}>
              {actionLoading === "create" ? "Creating..." : "Create"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export default function BotsPage() {
  return (
    <Suspense fallback={<div className="p-4 md:p-6"><div className="text-center py-8 text-muted-foreground">Loading...</div></div>}>
      <BotsPageInner />
    </Suspense>
  );
}
