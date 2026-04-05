"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { StatusBadge } from "@/components/status-badge";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PlusIcon, Trash2Icon, AlertCircleIcon, MessageSquareIcon, UsersIcon, QrCodeIcon, CheckIcon, XIcon, RefreshCwIcon } from "lucide-react";
import {
  listChannels,
  addChannel,
  removeChannel,
  listPairingRequests,
  approvePairing,
  revokePairing,
  listPairedUsers,
  wechatLoginStart,
  wechatLoginStatus,
  wechatListAccounts,
  wechatRemoveAccount,
  getErrorMessage,
  type Channel,
} from "@/lib/api";

// ── Constants ──────────────────────────────────────────────────

const CHANNEL_TYPES = [
  "telegram",
  "discord",
  "slack",
  "wechat",
  "feishu",
  "teams",
  "line",
] as const;

type ChannelType = (typeof CHANNEL_TYPES)[number];

const CHANNEL_LABELS: Record<ChannelType, string> = {
  telegram: "Telegram",
  discord: "Discord",
  slack: "Slack",
  wechat: "WeChat",
  feishu: "Feishu",
  teams: "Teams",
  line: "LINE",
};

const DM_POLICIES = ["open", "pairing", "allowlist", "disabled"] as const;
const GROUP_POLICIES = ["open", "allowlist", "disabled"] as const;

interface ChannelField {
  readonly key: string;
  readonly label: string;
  readonly required: boolean;
  readonly type?: "text" | "password";
}

const CHANNEL_FIELDS: Record<ChannelType, readonly ChannelField[]> = {
  telegram: [{ key: "botToken", label: "Bot Token", required: true, type: "password" }],
  discord: [
    { key: "token", label: "Token", required: true, type: "password" },
    { key: "appId", label: "App ID", required: false },
  ],
  slack: [
    { key: "token", label: "Bot Token", required: true, type: "password" },
    { key: "appToken", label: "App Token", required: false, type: "password" },
  ],
  wechat: [],
  feishu: [
    { key: "appId", label: "App ID", required: true },
    { key: "appSecret", label: "App Secret", required: true, type: "password" },
  ],
  teams: [
    { key: "appId", label: "App ID", required: true },
    { key: "appPassword", label: "App Password", required: true, type: "password" },
  ],
  line: [
    { key: "token", label: "Channel Access Token", required: true, type: "password" },
    { key: "channelSecret", label: "Channel Secret", required: true, type: "password" },
  ],
};

// ── Form state ─────────────────────────────────────────────────

interface AddChannelForm {
  readonly channelType: ChannelType;
  readonly account: string;
  readonly dmPolicy: string;
  readonly groupPolicy: string;
  readonly enabled: boolean;
  readonly fields: Record<string, string>;
}

function makeEmptyForm(): AddChannelForm {
  return {
    channelType: "telegram",
    account: "default",
    dmPolicy: "open",
    groupPolicy: "open",
    enabled: true,
    fields: {},
  };
}

// ── Component ──────────────────────────────────────────────────

interface ChannelsTabProps {
  readonly botId: string;
  readonly botStatus: string;
}

export function ChannelsTab({ botId, botStatus }: ChannelsTabProps) {
  const queryClient = useQueryClient();
  const [showAdd, setShowAdd] = useState(false);
  const [form, setForm] = useState<AddChannelForm>(makeEmptyForm);
  const [submitting, setSubmitting] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Channel | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [showWechatLogin, setShowWechatLogin] = useState(false);
  const [qrCodeUrl, setQrCodeUrl] = useState<string | null>(null);
  const [wechatPolling, setWechatPolling] = useState(false);

  const { data: channels, isLoading } = useQuery({
    queryKey: ["channels", botId],
    queryFn: () => listChannels(botId).then((r) => r.data ?? []),
    enabled: botStatus === "running",
  });

  // ── Not running guard ────────────────────────────────────────

  if (botStatus !== "running") {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-muted-foreground gap-3">
        <AlertCircleIcon className="size-8" />
        <p className="text-sm">Bot must be running to manage channels</p>
      </div>
    );
  }

  // ── Handlers ─────────────────────────────────────────────────

  const updateField = (key: string, value: string) => {
    setForm((f) => ({ ...f, fields: { ...f.fields, [key]: value } }));
  };

  const handleAddOpen = () => {
    setForm(makeEmptyForm());
    setShowAdd(true);
  };

  const handleSubmit = async () => {
    const fields = CHANNEL_FIELDS[form.channelType];
    for (const f of fields) {
      if (f.required && !form.fields[f.key]?.trim()) {
        toast.error(`${f.label} is required`);
        return;
      }
    }

    try {
      setSubmitting(true);
      await addChannel(botId, {
        channel: form.channelType,
        account: form.account || "default",
        enabled: form.enabled,
        dmPolicy: form.dmPolicy,
        groupPolicy: form.groupPolicy,
        ...form.fields,
      });
      toast.success("Channel added");
      setShowAdd(false);
      queryClient.invalidateQueries({ queryKey: ["channels", botId] });
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      setDeleting(true);
      await removeChannel(botId, deleteTarget.channel, deleteTarget.account);
      toast.success("Channel removed");
      setDeleteTarget(null);
      queryClient.invalidateQueries({ queryKey: ["channels", botId] });
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setDeleting(false);
    }
  };

  const handleWechatLogin = async () => {
    try {
      const res = await wechatLoginStart(botId);
      setQrCodeUrl(res.data.qrcode_url);
      setShowWechatLogin(true);
      // Start polling
      setWechatPolling(true);
      const poll = setInterval(async () => {
        try {
          const status = await wechatLoginStatus(botId);
          if (status.data.status === "confirmed") {
            clearInterval(poll);
            setWechatPolling(false);
            setShowWechatLogin(false);
            setQrCodeUrl(null);
            toast.success("WeChat connected!");
            queryClient.invalidateQueries({ queryKey: ["channels", botId] });
          } else if (status.data.status === "expired") {
            clearInterval(poll);
            setWechatPolling(false);
            toast.error("QR code expired, please try again");
          }
        } catch {
          clearInterval(poll);
          setWechatPolling(false);
        }
      }, 3000);
    } catch (err) {
      toast.error(getErrorMessage(err));
    }
  };

  // ── Render ───────────────────────────────────────────────────

  const currentFields = CHANNEL_FIELDS[form.channelType];
  const isWeChat = form.channelType === "wechat";

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-muted-foreground">
          Connected Channels
        </h3>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="outline" onClick={handleWechatLogin}>
            <QrCodeIcon className="size-4 mr-1" /> WeChat Login
          </Button>
          <Button size="sm" onClick={handleAddOpen}>
            <PlusIcon className="size-4 mr-1" /> Add Channel
          </Button>
        </div>
      </div>

      {/* Channel list */}
      {isLoading ? (
        <div className="space-y-3">
          {[1, 2].map((i) => (
            <div key={i} className="h-20 bg-muted rounded animate-pulse" />
          ))}
        </div>
      ) : !channels?.length ? (
        <div className="flex flex-col items-center justify-center py-12 text-muted-foreground gap-2">
          <MessageSquareIcon className="size-8" />
          <p className="text-sm">No channels connected</p>
          <p className="text-xs">Add a channel to start receiving messages</p>
        </div>
      ) : (
        <div className="grid gap-3">
          {channels.map((ch) => (
            <ChannelCard
              key={`${ch.channel}-${ch.account ?? "default"}`}
              botId={botId}
              channel={ch}
              onDelete={() => setDeleteTarget(ch)}
            />
          ))}
        </div>
      )}

      {/* Add Channel Dialog */}
      <Dialog open={showAdd} onOpenChange={setShowAdd}>
        <DialogContent className="max-w-[95vw] sm:max-w-lg max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Add Channel</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            {/* Channel type */}
            <div className="space-y-2">
              <Label>Channel Type</Label>
              <Select
                value={form.channelType}
                onValueChange={(v) =>
                  setForm((f) => ({
                    ...f,
                    channelType: (v as ChannelType) ?? f.channelType,
                    fields: {},
                  }))
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {CHANNEL_TYPES.map((type) => (
                    <SelectItem key={type} value={type}>
                      {CHANNEL_LABELS[type]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* WeChat note */}
            {isWeChat && (
              <div className="rounded-md border border-yellow-200 bg-yellow-50 px-4 py-3 text-sm text-yellow-800 dark:border-yellow-800 dark:bg-yellow-950 dark:text-yellow-200">
                Use WeChat QR login instead. Go to the WeChat-specific login
                flow for this bot.
              </div>
            )}

            {/* Channel-specific fields */}
            {currentFields.map((field) => (
              <div key={field.key} className="space-y-2">
                <Label htmlFor={`add-${field.key}`}>
                  {field.label}
                  {field.required && (
                    <span className="text-destructive ml-1">*</span>
                  )}
                </Label>
                <Input
                  id={`add-${field.key}`}
                  type={field.type ?? "text"}
                  value={form.fields[field.key] ?? ""}
                  onChange={(e) => updateField(field.key, e.target.value)}
                  placeholder={field.label}
                />
              </div>
            ))}

            {/* Common fields (except WeChat) */}
            {!isWeChat && (
              <>
                <div className="space-y-2">
                  <Label htmlFor="add-account">Account</Label>
                  <Input
                    id="add-account"
                    value={form.account}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, account: e.target.value }))
                    }
                    placeholder="default"
                  />
                </div>

                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label>DM Policy</Label>
                    <Select
                      value={form.dmPolicy}
                      onValueChange={(v) =>
                        setForm((f) => ({ ...f, dmPolicy: v ?? f.dmPolicy }))
                      }
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {DM_POLICIES.map((p) => (
                          <SelectItem key={p} value={p}>
                            {p}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="space-y-2">
                    <Label>Group Policy</Label>
                    <Select
                      value={form.groupPolicy}
                      onValueChange={(v) =>
                        setForm((f) => ({
                          ...f,
                          groupPolicy: v ?? f.groupPolicy,
                        }))
                      }
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {GROUP_POLICIES.map((p) => (
                          <SelectItem key={p} value={p}>
                            {p}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  <Checkbox
                    id="add-enabled"
                    checked={form.enabled}
                    onCheckedChange={(checked) =>
                      setForm((f) => ({ ...f, enabled: !!checked }))
                    }
                  />
                  <Label htmlFor="add-enabled" className="text-sm font-normal">
                    Enabled
                  </Label>
                </div>
              </>
            )}
          </div>
          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button
              variant="outline"
              onClick={() => setShowAdd(false)}
              className="w-full sm:w-auto"
            >
              Cancel
            </Button>
            <Button
              onClick={handleSubmit}
              disabled={submitting || isWeChat}
              className="w-full sm:w-auto"
            >
              {submitting ? "Adding..." : "Add Channel"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* WeChat QR Login Dialog */}
      <Dialog open={showWechatLogin} onOpenChange={(open) => { if (!open) { setShowWechatLogin(false); setWechatPolling(false); } }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>WeChat QR Login</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col items-center gap-4 py-4">
            {qrCodeUrl ? (
              <>
                <img src={qrCodeUrl} alt="WeChat QR Code" className="w-48 h-48 border rounded" />
                <p className="text-sm text-muted-foreground text-center">
                  Scan with WeChat to connect
                </p>
                {wechatPolling && (
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <RefreshCwIcon className="size-3 animate-spin" /> Waiting for scan...
                  </div>
                )}
              </>
            ) : (
              <p className="text-sm text-muted-foreground">Loading QR code...</p>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation */}
      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title="Remove Channel"
        description={
          deleteTarget
            ? `Remove ${CHANNEL_LABELS[deleteTarget.channel as ChannelType] ?? deleteTarget.channel} channel (account: ${deleteTarget.account ?? "default"})? This will disconnect it from the bot.`
            : ""
        }
        confirmLabel="Remove"
        loading={deleting}
        onConfirm={handleDelete}
        variant="destructive"
      />
    </div>
  );
}

// ── Channel Card ───────────────────────────────────────────────

interface ChannelCardProps {
  readonly botId: string;
  readonly channel: Channel;
  readonly onDelete: () => void;
}

function ChannelCard({ botId, channel, onDelete }: ChannelCardProps) {
  const label =
    CHANNEL_LABELS[channel.channel as ChannelType] ?? channel.channel;

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <CardTitle className="text-sm font-medium">{label}</CardTitle>
            <Badge variant="outline" className="text-xs">
              {channel.channel}
            </Badge>
            {channel.enabled !== undefined && (
              <StatusBadge
                status={channel.enabled ? "active" : "disabled"}
              />
            )}
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="size-8 text-muted-foreground hover:text-destructive"
            onClick={onDelete}
          >
            <Trash2Icon className="size-4" />
          </Button>
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        <p className="text-xs text-muted-foreground">
          Account: {channel.account ?? "default"}
        </p>
        <PairingSection botId={botId} channelName={channel.channel} />
      </CardContent>
    </Card>
  );
}

// ── Pairing Section ───────────────────────────────────────────

function PairingSection({ botId, channelName }: { botId: string; channelName: string }) {
  const queryClient = useQueryClient();
  const [expanded, setExpanded] = useState(false);
  const [approveCode, setApproveCode] = useState("");
  const [actionLoading, setActionLoading] = useState<string | null>(null);

  const { data: pendingRequests } = useQuery({
    queryKey: ["pairing-requests", botId, channelName],
    queryFn: () => listPairingRequests(botId, channelName).then((r) => r.data ?? []),
    enabled: expanded,
  });

  const { data: pairedUsers } = useQuery({
    queryKey: ["paired-users", botId, channelName],
    queryFn: () => listPairedUsers(botId, channelName).then((r) => r.data ?? []),
    enabled: expanded,
  });

  const handleApprove = async (code: string) => {
    try {
      setActionLoading("approve");
      await approvePairing(botId, channelName, code);
      toast.success("Pairing approved");
      setApproveCode("");
      queryClient.invalidateQueries({ queryKey: ["pairing-requests", botId, channelName] });
      queryClient.invalidateQueries({ queryKey: ["paired-users", botId, channelName] });
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setActionLoading(null);
    }
  };

  const handleRevoke = async (userId: string) => {
    try {
      setActionLoading(userId);
      await revokePairing(botId, channelName, userId);
      toast.success("Pairing revoked");
      queryClient.invalidateQueries({ queryKey: ["paired-users", botId, channelName] });
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setActionLoading(null);
    }
  };

  if (!expanded) {
    return (
      <Button variant="ghost" size="sm" className="mt-1" onClick={() => setExpanded(true)}>
        <UsersIcon className="size-3 mr-1" /> Manage Pairing
      </Button>
    );
  }

  return (
    <div className="mt-3 space-y-3 border-t pt-3">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">Pairing Management</span>
        <Button variant="ghost" size="sm" onClick={() => setExpanded(false)}>
          <XIcon className="size-3" />
        </Button>
      </div>

      {/* Approve with code */}
      <div className="space-y-1">
        <span className="text-xs text-muted-foreground">Approve by code</span>
        <div className="flex gap-2">
          <Input
            value={approveCode}
            onChange={(e) => setApproveCode(e.target.value)}
            placeholder="Enter pairing code"
            className="h-8 text-sm"
          />
          <Button
            size="sm"
            disabled={!approveCode.trim() || actionLoading === "approve"}
            onClick={() => handleApprove(approveCode.trim())}
          >
            <CheckIcon className="size-3 mr-1" />
            {actionLoading === "approve" ? "..." : "Approve"}
          </Button>
        </div>
      </div>

      {/* Pending requests */}
      {pendingRequests && pendingRequests.length > 0 && (
        <div className="space-y-1">
          <span className="text-xs text-muted-foreground">Pending Requests ({pendingRequests.length})</span>
          <div className="space-y-1">
            {(pendingRequests as Array<Record<string, unknown>>).map((req, i) => (
              <div key={i} className="flex items-center justify-between text-xs bg-muted rounded px-2 py-1">
                <span className="font-mono truncate">{String(req.user_id || req.code || `request-${i}`)}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 text-xs"
                  onClick={() => handleApprove(String(req.code || ""))}
                  disabled={!!actionLoading}
                >
                  Approve
                </Button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Paired users */}
      {pairedUsers && (pairedUsers as unknown[]).length > 0 && (
        <div className="space-y-1">
          <span className="text-xs text-muted-foreground">Paired Users ({(pairedUsers as unknown[]).length})</span>
          <div className="space-y-1">
            {(pairedUsers as Array<Record<string, unknown>>).map((user, i) => (
              <div key={i} className="flex items-center justify-between text-xs bg-muted rounded px-2 py-1">
                <span className="font-mono truncate">{String(user.user_id || user.id || `user-${i}`)}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 text-xs text-destructive"
                  onClick={() => handleRevoke(String(user.user_id || user.id))}
                  disabled={actionLoading === String(user.user_id || user.id)}
                >
                  Revoke
                </Button>
              </div>
            ))}
          </div>
        </div>
      )}

      {(!pendingRequests || pendingRequests.length === 0) && (!pairedUsers || (pairedUsers as unknown[]).length === 0) && (
        <p className="text-xs text-muted-foreground">No pairing requests or paired users</p>
      )}
    </div>
  );
}
