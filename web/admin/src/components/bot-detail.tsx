"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { StatusBadge } from "@/components/status-badge";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  ArrowLeftIcon,
  CopyIcon,
  ExternalLinkIcon,
  PlayIcon,
  SquareIcon,
  RefreshCwIcon,
  ArrowUpCircleIcon,
  KeyIcon,
  PencilIcon,
} from "lucide-react";
import {
  getBot,
  getBotConnect,
  updateBot as updateBotApi,
  resetBotToken,
  startBot,
  stopBot,
  restartBot,
  upgradeBot,
  listApps,
  getErrorMessage,
} from "@/lib/api";

function copyToClipboard(text: string, label: string) {
  navigator.clipboard.writeText(text);
  toast.success(`${label} copied`);
}

interface BotDetailProps {
  readonly botId: string;
  readonly tab?: string;
}

export function BotDetail({ botId, tab: initialTab }: BotDetailProps) {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState(initialTab || "overview");
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [showEdit, setShowEdit] = useState(false);
  const [editForm, setEditForm] = useState({
    name: "",
    slug: "",
    expires_at: "",
  });
  const [showResetToken, setShowResetToken] = useState(false);
  const [newToken, setNewToken] = useState<string | null>(null);

  // Fetch bot data
  const { data: bot, isLoading: botLoading } = useQuery({
    queryKey: ["bot", botId],
    queryFn: () => getBot(botId).then((r) => r.data),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "starting" ? 5000 : false;
    },
  });

  // Fetch apps for name lookup
  const { data: apps } = useQuery({
    queryKey: ["apps"],
    queryFn: () => listApps().then((r) => r.data || []),
    staleTime: 60000,
  });

  // Fetch connect info (only if running)
  const { data: connectInfo } = useQuery({
    queryKey: ["bot-connect", botId],
    queryFn: () => getBotConnect(botId).then((r) => r.data),
    enabled: bot?.status === "running",
  });

  const appName =
    apps?.find((a) => a.id === bot?.app_id)?.name ||
    bot?.app_id?.slice(0, 8) ||
    "";

  const handleAction = async (
    action: () => Promise<unknown>,
    successMsg: string,
    actionId: string,
  ) => {
    try {
      setActionLoading(actionId);
      await action();
      toast.success(successMsg);
      queryClient.invalidateQueries({ queryKey: ["bot", botId] });
      queryClient.invalidateQueries({ queryKey: ["bot-connect", botId] });
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setActionLoading(null);
    }
  };

  const openEdit = () => {
    if (!bot) return;
    setEditForm({
      name: bot.name,
      slug: bot.slug,
      expires_at: bot.expires_at ? bot.expires_at.slice(0, 16) : "",
    });
    setShowEdit(true);
  };

  const handleEdit = async () => {
    try {
      setActionLoading("edit");
      await updateBotApi(botId, {
        name: editForm.name || undefined,
        slug: editForm.slug || undefined,
        expires_at: editForm.expires_at
          ? new Date(editForm.expires_at).toISOString()
          : null,
      });
      toast.success("Bot updated");
      setShowEdit(false);
      queryClient.invalidateQueries({ queryKey: ["bot", botId] });
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setActionLoading(null);
    }
  };

  const handleResetToken = async () => {
    try {
      setActionLoading("reset-token");
      const res = await resetBotToken(botId);
      setNewToken(res.data.access_token);
      setShowResetToken(false);
      toast.success("Token reset");
      queryClient.invalidateQueries({ queryKey: ["bot", botId] });
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setActionLoading(null);
    }
  };

  const handleTabChange = (value: unknown) => {
    const tab = String(value);
    setActiveTab(tab);
    window.history.replaceState(null, "", `/admin/bots/?id=${botId}&tab=${tab}`);
  };

  if (botLoading) {
    return (
      <div className="p-4 md:p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-8 bg-muted rounded w-48" />
          <div className="h-4 bg-muted rounded w-32" />
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mt-6">
            <div className="h-40 bg-muted rounded" />
            <div className="h-40 bg-muted rounded" />
            <div className="h-40 bg-muted rounded" />
          </div>
        </div>
      </div>
    );
  }

  if (!bot) {
    return (
      <div className="p-4 md:p-6">
        <Button variant="ghost" size="sm" render={<a href="/admin/bots/" />}>
          <ArrowLeftIcon className="size-4 mr-1" /> Back to Bots
        </Button>
        <div className="text-center py-12 text-muted-foreground">
          Bot not found
        </div>
      </div>
    );
  }

  return (
    <div className="p-4 md:p-6 space-y-4 md:space-y-6">
      {/* Header */}
      <div>
        <a href="/admin/bots/" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors mb-2 -ml-2 px-2 py-1">
          <ArrowLeftIcon className="size-4" /> Bots
        </a>
        <div className="flex items-center gap-3">
          <h1 className="text-xl md:text-2xl font-bold">{bot.name}</h1>
          <StatusBadge status={bot.status} />
        </div>
        <p className="text-sm text-muted-foreground mt-1">
          {appName && `App: ${appName}`}
          {bot.slug && ` · Slug: ${bot.slug}`}
        </p>
      </div>

      {/* Tabs */}
      <Tabs value={activeTab} onValueChange={handleTabChange}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="channels" disabled>
            Channels
          </TabsTrigger>
          <TabsTrigger value="skills" disabled>
            Skills
          </TabsTrigger>
          <TabsTrigger value="models" disabled>
            Models
          </TabsTrigger>
          <TabsTrigger value="devices" disabled>
            Devices
          </TabsTrigger>
          <TabsTrigger value="config" disabled>
            Config
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4 md:space-y-6 mt-4">
          {/* Quick Actions */}
          <div className="flex flex-wrap gap-2">
            {bot.status !== "running" ? (
              <Button
                size="sm"
                onClick={() =>
                  handleAction(
                    () => startBot(botId),
                    "Bot started",
                    "start",
                  )
                }
                disabled={!!actionLoading}
              >
                <PlayIcon className="size-4 mr-1" />
                {actionLoading === "start" ? "Starting..." : "Start"}
              </Button>
            ) : (
              <Button
                size="sm"
                variant="outline"
                onClick={() =>
                  handleAction(() => stopBot(botId), "Bot stopped", "stop")
                }
                disabled={!!actionLoading}
              >
                <SquareIcon className="size-4 mr-1" />
                {actionLoading === "stop" ? "Stopping..." : "Stop"}
              </Button>
            )}
            {bot.status === "running" && (
              <Button
                size="sm"
                variant="outline"
                onClick={() =>
                  handleAction(
                    () => restartBot(botId),
                    "Bot restarted",
                    "restart",
                  )
                }
                disabled={!!actionLoading}
              >
                <RefreshCwIcon className="size-4 mr-1" />
                {actionLoading === "restart" ? "Restarting..." : "Restart"}
              </Button>
            )}
            <Button
              size="sm"
              variant="outline"
              onClick={() =>
                handleAction(
                  () => upgradeBot(botId),
                  "Bot upgraded",
                  "upgrade",
                )
              }
              disabled={!!actionLoading}
            >
              <ArrowUpCircleIcon className="size-4 mr-1" />
              {actionLoading === "upgrade" ? "Upgrading..." : "Upgrade"}
            </Button>
            <Separator orientation="vertical" className="h-8" />
            <Button
              size="sm"
              variant="outline"
              onClick={() => setShowResetToken(true)}
              disabled={!!actionLoading}
            >
              <KeyIcon className="size-4 mr-1" /> Reset Token
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={openEdit}
              disabled={!!actionLoading}
            >
              <PencilIcon className="size-4 mr-1" /> Edit
            </Button>
          </div>

          {/* Info Cards */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {/* Status Card */}
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  Status
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="flex items-center gap-2">
                  <StatusBadge status={bot.status} />
                </div>
                {bot.endpoint && (
                  <div>
                    <span className="text-xs text-muted-foreground">
                      Endpoint
                    </span>
                    <p className="text-sm font-mono truncate">{bot.endpoint}</p>
                  </div>
                )}
                <div className="grid grid-cols-2 gap-2 text-xs">
                  <div>
                    <span className="text-muted-foreground">Created</span>
                    <p>{new Date(bot.created_at).toLocaleDateString()}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Updated</span>
                    <p>{new Date(bot.updated_at).toLocaleDateString()}</p>
                  </div>
                </div>
              </CardContent>
            </Card>

            {/* Connection Card */}
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  Connection
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                {bot.status !== "running" ? (
                  <p className="text-sm text-muted-foreground">
                    Bot must be running to show connection info
                  </p>
                ) : connectInfo ? (
                  <>
                    {connectInfo.ws_url && (
                      <div>
                        <span className="text-xs text-muted-foreground">
                          WebSocket URL
                        </span>
                        <div className="flex items-center gap-1">
                          <code className="text-xs truncate flex-1">
                            {connectInfo.ws_url}
                          </code>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-6 shrink-0"
                            onClick={() =>
                              copyToClipboard(connectInfo.ws_url, "WS URL")
                            }
                          >
                            <CopyIcon className="size-3" />
                          </Button>
                        </div>
                      </div>
                    )}
                    {connectInfo.webchat_url && (
                      <div>
                        <span className="text-xs text-muted-foreground">
                          Web Chat
                        </span>
                        <div className="flex items-center gap-1">
                          <a
                            href={connectInfo.webchat_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-xs text-blue-600 hover:underline truncate"
                          >
                            {connectInfo.webchat_url}
                          </a>
                          <ExternalLinkIcon className="size-3 shrink-0 text-muted-foreground" />
                        </div>
                      </div>
                    )}
                    <div>
                      <span className="text-xs text-muted-foreground">
                        Access Token
                      </span>
                      <div className="flex items-center gap-1">
                        <code className="text-xs">
                          {"•".repeat(16)}
                        </code>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-6 shrink-0"
                          onClick={() =>
                            copyToClipboard(connectInfo.token, "Access Token")
                          }
                        >
                          <CopyIcon className="size-3" />
                        </Button>
                      </div>
                    </div>
                  </>
                ) : (
                  <p className="text-sm text-muted-foreground">Loading...</p>
                )}
              </CardContent>
            </Card>

            {/* Configuration Card */}
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  Configuration
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <div>
                  <span className="text-xs text-muted-foreground">Bot ID</span>
                  <div className="flex items-center gap-1">
                    <code className="text-xs truncate flex-1">{bot.id}</code>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-6 shrink-0"
                      onClick={() => copyToClipboard(bot.id, "Bot ID")}
                    >
                      <CopyIcon className="size-3" />
                    </Button>
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-2 text-xs">
                  <div>
                    <span className="text-muted-foreground">Slug</span>
                    <p className="font-mono">{bot.slug}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">User ID</span>
                    <p className="font-mono truncate">{bot.user_id}</p>
                  </div>
                </div>
                <div>
                  <span className="text-xs text-muted-foreground">Expires</span>
                  <p className="text-sm">
                    {bot.expires_at
                      ? new Date(bot.expires_at).toLocaleString()
                      : "Never"}
                  </p>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        {/* Placeholder tabs for future sprints */}
        <TabsContent value="channels">
          <div className="text-center py-12 text-muted-foreground">
            Coming soon
          </div>
        </TabsContent>
        <TabsContent value="skills">
          <div className="text-center py-12 text-muted-foreground">
            Coming soon
          </div>
        </TabsContent>
        <TabsContent value="models">
          <div className="text-center py-12 text-muted-foreground">
            Coming soon
          </div>
        </TabsContent>
        <TabsContent value="devices">
          <div className="text-center py-12 text-muted-foreground">
            Coming soon
          </div>
        </TabsContent>
        <TabsContent value="config">
          <div className="text-center py-12 text-muted-foreground">
            Coming soon
          </div>
        </TabsContent>
      </Tabs>

      {/* Edit Dialog */}
      <Dialog open={showEdit} onOpenChange={setShowEdit}>
        <DialogContent className="max-w-[95vw] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Edit Bot</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="edit-name">Name</Label>
              <Input
                id="edit-name"
                value={editForm.name}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, name: e.target.value }))
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-slug">Slug</Label>
              <Input
                id="edit-slug"
                value={editForm.slug}
                onChange={(e) =>
                  setEditForm((f) => ({ ...f, slug: e.target.value }))
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-expires">Expires At</Label>
              <div className="flex gap-2">
                <Input
                  id="edit-expires"
                  type="datetime-local"
                  value={editForm.expires_at}
                  onChange={(e) =>
                    setEditForm((f) => ({ ...f, expires_at: e.target.value }))
                  }
                  className="flex-1"
                />
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    setEditForm((f) => ({ ...f, expires_at: "" }))
                  }
                >
                  Clear
                </Button>
              </div>
            </div>
          </div>
          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button
              variant="outline"
              onClick={() => setShowEdit(false)}
              className="w-full sm:w-auto"
            >
              Cancel
            </Button>
            <Button
              onClick={handleEdit}
              disabled={actionLoading === "edit"}
              className="w-full sm:w-auto"
            >
              {actionLoading === "edit" ? "Saving..." : "Save"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Reset Token Confirmation */}
      <ConfirmDialog
        open={showResetToken}
        onOpenChange={setShowResetToken}
        title="Reset Access Token"
        description="This will generate a new access token and invalidate the current one. Any connected clients will be disconnected."
        confirmLabel="Reset Token"
        loading={actionLoading === "reset-token"}
        onConfirm={handleResetToken}
      />

      {/* New Token Display */}
      <Dialog open={!!newToken} onOpenChange={() => setNewToken(null)}>
        <DialogContent className="max-w-[95vw] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>New Access Token</DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">
              Copy this token now. It won&apos;t be shown again.
            </p>
            <code className="block p-3 bg-muted rounded text-xs break-all select-all">
              {newToken}
            </code>
          </div>
          <DialogFooter>
            <Button
              onClick={() => {
                if (newToken) {
                  navigator.clipboard.writeText(newToken);
                  toast.success("Copied");
                }
              }}
            >
              Copy
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
