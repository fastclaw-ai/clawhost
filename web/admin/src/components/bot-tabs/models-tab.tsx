"use client";

import { useState, useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  CardAction,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  PlusIcon,
  PencilIcon,
  Trash2Icon,
  ServerIcon,
  BrainIcon,
  AlertCircleIcon,
  LayersIcon,
  SaveIcon,
} from "lucide-react";
import {
  listModelProviders,
  addModelProvider,
  updateModelProvider,
  deleteModelProvider,
  getAgentDefaults,
  setAgentDefaults,
  getErrorMessage,
  type ModelProvider,
  type AgentDefaults,
} from "@/lib/api";

// ── Constants ──────────────────────────────────────────────────

const AUTH_OPTIONS = [
  { value: "api-key", label: "API Key" },
  { value: "bearer", label: "Bearer Token" },
] as const;

const API_OPTIONS = [
  { value: "openai-completions", label: "OpenAI Completions" },
  { value: "anthropic-messages", label: "Anthropic Messages" },
] as const;

// ── Types ──────────────────────────────────────────────────────

interface ModelsTabProps {
  readonly botId: string;
  readonly botStatus: string;
}

interface ProviderFormState {
  name: string;
  baseUrl: string;
  apiKey: string;
  auth: string;
  api: string;
}

const EMPTY_FORM: ProviderFormState = {
  name: "",
  baseUrl: "",
  apiKey: "",
  auth: "api-key",
  api: "openai-completions",
};

// ── Helpers ────────────────────────────────────────────────────

function maskApiKey(key: string | undefined): string {
  if (!key) return "Not set";
  if (key.length <= 8) return "****";
  return `${key.slice(0, 4)}${"*".repeat(Math.min(key.length - 8, 16))}${key.slice(-4)}`;
}

function formFromProvider(provider: ModelProvider): ProviderFormState {
  return {
    name: provider.name,
    baseUrl: provider.baseUrl ?? "",
    apiKey: provider.apiKey ?? "",
    auth: provider.auth ?? "api-key",
    api: provider.api ?? "openai-completions",
  };
}

function formToPayload(form: ProviderFormState): ModelProvider {
  return {
    name: form.name,
    baseUrl: form.baseUrl || undefined,
    apiKey: form.apiKey || undefined,
    auth: form.auth || undefined,
    api: form.api || undefined,
  };
}

// ── Component ──────────────────────────────────────────────────

export function ModelsTab({ botId, botStatus }: ModelsTabProps) {
  const queryClient = useQueryClient();
  const isRunning = botStatus === "running";

  // ── Provider state ─────────────────────────────────────────
  const [showProviderDialog, setShowProviderDialog] = useState(false);
  const [editingProvider, setEditingProvider] = useState<string | null>(null);
  const [providerForm, setProviderForm] = useState<ProviderFormState>(EMPTY_FORM);
  const [providerSaving, setProviderSaving] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  // ── Defaults state ─────────────────────────────────────────
  const [defaultsForm, setDefaultsForm] = useState<AgentDefaults>({
    primary_model: "",
    fallback_model: "",
  });
  const [defaultsSaving, setDefaultsSaving] = useState(false);
  const [defaultsDirty, setDefaultsDirty] = useState(false);

  // ── Queries ────────────────────────────────────────────────

  const {
    data: providers,
    isLoading: providersLoading,
    error: providersError,
  } = useQuery({
    queryKey: ["bot-model-providers", botId],
    queryFn: () => listModelProviders(botId).then((r) => r.data),
    enabled: isRunning,
  });

  const {
    data: agentDefaults,
    isLoading: defaultsLoading,
    error: defaultsError,
  } = useQuery({
    queryKey: ["bot-agent-defaults", botId],
    queryFn: () =>
      getAgentDefaults(botId).then((r) => {
        const defaults = r.data;
        setDefaultsForm({
          primary_model: defaults.primary_model ?? "",
          fallback_model: defaults.fallback_model ?? "",
        });
        setDefaultsDirty(false);
        return defaults;
      }),
    enabled: isRunning,
  });

  // ── Invalidation helper ────────────────────────────────────

  const invalidateProviders = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["bot-model-providers", botId] });
  }, [queryClient, botId]);

  const invalidateDefaults = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["bot-agent-defaults", botId] });
  }, [queryClient, botId]);

  // ── Provider handlers ──────────────────────────────────────

  const openAddProvider = () => {
    setEditingProvider(null);
    setProviderForm(EMPTY_FORM);
    setShowProviderDialog(true);
  };

  const openEditProvider = (provider: ModelProvider) => {
    setEditingProvider(provider.name);
    setProviderForm(formFromProvider(provider));
    setShowProviderDialog(true);
  };

  const handleProviderSubmit = async () => {
    if (!providerForm.name.trim()) {
      toast.error("Provider name is required");
      return;
    }

    try {
      setProviderSaving(true);
      const payload = formToPayload(providerForm);

      if (editingProvider) {
        await updateModelProvider(botId, editingProvider, payload);
        toast.success(`Provider "${providerForm.name}" updated`);
      } else {
        await addModelProvider(botId, payload);
        toast.success(`Provider "${providerForm.name}" added`);
      }

      setShowProviderDialog(false);
      invalidateProviders();
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setProviderSaving(false);
    }
  };

  const handleDeleteProvider = async () => {
    if (!deleteTarget) return;

    try {
      setDeleteLoading(true);
      await deleteModelProvider(botId, deleteTarget);
      toast.success(`Provider "${deleteTarget}" deleted`);
      setDeleteTarget(null);
      invalidateProviders();
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setDeleteLoading(false);
    }
  };

  // ── Defaults handlers ──────────────────────────────────────

  const handleDefaultsChange = (field: keyof AgentDefaults, value: string) => {
    setDefaultsForm((prev) => ({ ...prev, [field]: value }));
    setDefaultsDirty(true);
  };

  const handleDefaultsSave = async () => {
    try {
      setDefaultsSaving(true);
      await setAgentDefaults(botId, {
        primary_model: defaultsForm.primary_model || undefined,
        fallback_model: defaultsForm.fallback_model || undefined,
      });
      toast.success("Agent defaults saved");
      setDefaultsDirty(false);
      invalidateDefaults();
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setDefaultsSaving(false);
    }
  };

  // ── Not running state ──────────────────────────────────────

  if (!isRunning) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-center">
        <AlertCircleIcon className="size-10 text-muted-foreground mb-3" />
        <p className="text-sm font-medium text-muted-foreground">
          Bot must be running to manage models
        </p>
        <p className="text-xs text-muted-foreground mt-1">
          Start the bot from the Overview tab to configure model providers and agent defaults.
        </p>
      </div>
    );
  }

  // ── Loading state ──────────────────────────────────────────

  if (providersLoading || defaultsLoading) {
    return (
      <div className="space-y-6 mt-4">
        <div className="space-y-3">
          <Skeleton className="h-6 w-40" />
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            <Skeleton className="h-36 rounded-xl" />
            <Skeleton className="h-36 rounded-xl" />
          </div>
        </div>
        <Separator />
        <div className="space-y-3">
          <Skeleton className="h-6 w-32" />
          <Skeleton className="h-10 w-full max-w-md" />
          <Skeleton className="h-10 w-full max-w-md" />
        </div>
      </div>
    );
  }

  // ── Error state ────────────────────────────────────────────

  if (providersError || defaultsError) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-center">
        <AlertCircleIcon className="size-10 text-destructive mb-3" />
        <p className="text-sm font-medium text-destructive">
          Failed to load model configuration
        </p>
        <p className="text-xs text-muted-foreground mt-1">
          {getErrorMessage(providersError || defaultsError)}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6 mt-4">
      {/* ── Section 1: Model Providers ──────────────────────── */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-medium flex items-center gap-2">
              <ServerIcon className="size-4" />
              Model Providers
            </h3>
            <p className="text-xs text-muted-foreground mt-0.5">
              Configure LLM provider connections for your bot.
            </p>
          </div>
          <Button size="sm" onClick={openAddProvider}>
            <PlusIcon className="size-4 mr-1" />
            Add Provider
          </Button>
        </div>

        {(!providers || providers.length === 0) ? (
          <Card>
            <CardContent className="flex flex-col items-center justify-center py-8 text-center">
              <LayersIcon className="size-8 text-muted-foreground mb-2" />
              <p className="text-sm text-muted-foreground">
                No model providers configured
              </p>
              <p className="text-xs text-muted-foreground mt-1">
                Add a provider to connect your bot to an LLM service.
              </p>
            </CardContent>
          </Card>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {providers.map((provider) => (
              <Card key={provider.name}>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium truncate">
                    {provider.name}
                  </CardTitle>
                  <CardAction>
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => openEditProvider(provider)}
                      >
                        <PencilIcon className="size-3" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => setDeleteTarget(provider.name)}
                      >
                        <Trash2Icon className="size-3 text-destructive" />
                      </Button>
                    </div>
                  </CardAction>
                  {provider.baseUrl && (
                    <CardDescription className="truncate text-xs">
                      {provider.baseUrl}
                    </CardDescription>
                  )}
                </CardHeader>
                <CardContent className="space-y-2">
                  <div className="flex flex-wrap gap-1.5">
                    {provider.auth && (
                      <Badge variant="secondary">{provider.auth}</Badge>
                    )}
                    {provider.api && (
                      <Badge variant="outline">{provider.api}</Badge>
                    )}
                  </div>
                  <div className="space-y-1">
                    <div className="flex items-center justify-between text-xs">
                      <span className="text-muted-foreground">API Key</span>
                      <code className="text-xs font-mono">
                        {maskApiKey(provider.apiKey)}
                      </code>
                    </div>
                    <div className="flex items-center justify-between text-xs">
                      <span className="text-muted-foreground">Models</span>
                      <span>{provider.models?.length ?? 0} configured</span>
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>

      <Separator />

      {/* ── Section 2: Agent Defaults ───────────────────────── */}
      <div className="space-y-4">
        <div>
          <h3 className="text-sm font-medium flex items-center gap-2">
            <BrainIcon className="size-4" />
            Agent Defaults
          </h3>
          <p className="text-xs text-muted-foreground mt-0.5">
            Set the default models used by the bot agent.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 max-w-2xl">
          <div className="space-y-2">
            <Label htmlFor="primary-model">Primary Model</Label>
            <Input
              id="primary-model"
              placeholder="e.g. anthropic/claude-sonnet-4-20250514"
              value={defaultsForm.primary_model ?? ""}
              onChange={(e) => handleDefaultsChange("primary_model", e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              The main model used for agent responses.
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="fallback-model">Fallback Model</Label>
            <Input
              id="fallback-model"
              placeholder="e.g. openai/gpt-4o"
              value={defaultsForm.fallback_model ?? ""}
              onChange={(e) => handleDefaultsChange("fallback_model", e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Used when the primary model is unavailable.
            </p>
          </div>
        </div>

        <div>
          <Button
            size="sm"
            onClick={handleDefaultsSave}
            disabled={defaultsSaving || !defaultsDirty}
          >
            <SaveIcon className="size-4 mr-1" />
            {defaultsSaving ? "Saving..." : "Save Defaults"}
          </Button>
        </div>
      </div>

      {/* ── Provider Add/Edit Dialog ────────────────────────── */}
      <Dialog
        open={showProviderDialog}
        onOpenChange={setShowProviderDialog}
      >
        <DialogContent className="max-w-[95vw] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {editingProvider ? "Edit Provider" : "Add Provider"}
            </DialogTitle>
            <DialogDescription>
              {editingProvider
                ? `Update the configuration for "${editingProvider}".`
                : "Configure a new model provider connection."}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="provider-name">Name *</Label>
              <Input
                id="provider-name"
                placeholder="e.g. openai, anthropic, local-llm"
                value={providerForm.name}
                onChange={(e) =>
                  setProviderForm((f) => ({ ...f, name: e.target.value }))
                }
                disabled={!!editingProvider}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="provider-base-url">Base URL</Label>
              <Input
                id="provider-base-url"
                placeholder="e.g. https://api.openai.com/v1"
                value={providerForm.baseUrl}
                onChange={(e) =>
                  setProviderForm((f) => ({ ...f, baseUrl: e.target.value }))
                }
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="provider-api-key">API Key</Label>
              <Input
                id="provider-api-key"
                type="password"
                placeholder={editingProvider ? "Leave blank to keep current" : "sk-..."}
                value={providerForm.apiKey}
                onChange={(e) =>
                  setProviderForm((f) => ({ ...f, apiKey: e.target.value }))
                }
              />
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>Auth Method</Label>
                <Select
                  value={providerForm.auth}
                  onValueChange={(v) =>
                    setProviderForm((f) => ({ ...f, auth: v ?? f.auth }))
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {AUTH_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>API Format</Label>
                <Select
                  value={providerForm.api}
                  onValueChange={(v) =>
                    setProviderForm((f) => ({ ...f, api: v ?? f.api }))
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {API_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          </div>

          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button
              variant="outline"
              onClick={() => setShowProviderDialog(false)}
              className="w-full sm:w-auto"
            >
              Cancel
            </Button>
            <Button
              onClick={handleProviderSubmit}
              disabled={providerSaving}
              className="w-full sm:w-auto"
            >
              {providerSaving
                ? "Saving..."
                : editingProvider
                  ? "Update Provider"
                  : "Add Provider"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Delete Confirmation ─────────────────────────────── */}
      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title="Delete Provider"
        description={`Are you sure you want to delete the provider "${deleteTarget}"? This action cannot be undone.`}
        confirmLabel="Delete"
        loading={deleteLoading}
        onConfirm={handleDeleteProvider}
        variant="destructive"
      />
    </div>
  );
}
