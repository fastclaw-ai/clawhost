"use client";

import { useState, useEffect, useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { SaveIcon, RotateCcwIcon, AlertCircleIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  getBotRawConfig,
  updateBotRawConfig,
  getErrorMessage,
} from "@/lib/api";

interface ConfigTabProps {
  readonly botId: string;
  readonly botStatus: string;
}

type ConfigMode = "merge" | "replace";

export function ConfigTab({ botId, botStatus }: ConfigTabProps) {
  const queryClient = useQueryClient();
  const [configText, setConfigText] = useState("");
  const [mode, setMode] = useState<ConfigMode>("merge");
  const [jsonError, setJsonError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const { data: rawConfig, isLoading } = useQuery({
    queryKey: ["bot-raw-config", botId],
    queryFn: () => getBotRawConfig(botId).then((r) => r.data),
    enabled: botStatus === "running",
  });

  const syncFromServer = useCallback(() => {
    if (rawConfig) {
      setConfigText(JSON.stringify(rawConfig, null, 2));
      setJsonError(null);
    }
  }, [rawConfig]);

  useEffect(() => {
    syncFromServer();
  }, [syncFromServer]);

  if (botStatus !== "running") {
    return (
      <div className="text-center py-12 text-muted-foreground">
        Bot must be running to view configuration.
      </div>
    );
  }

  const validateJson = (text: string): Record<string, unknown> | null => {
    try {
      const parsed = JSON.parse(text);
      if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
        setJsonError("Config must be a JSON object");
        return null;
      }
      setJsonError(null);
      return parsed as Record<string, unknown>;
    } catch (err) {
      setJsonError(getErrorMessage(err));
      return null;
    }
  };

  const handleTextChange = (value: string) => {
    setConfigText(value);
    if (value.trim()) {
      validateJson(value);
    } else {
      setJsonError(null);
    }
  };

  const handleSave = async () => {
    const parsed = validateJson(configText);
    if (!parsed) {
      toast.error("Invalid JSON - please fix errors before saving");
      return;
    }
    try {
      setSaving(true);
      await updateBotRawConfig(botId, parsed, mode);
      toast.success(`Config saved (${mode} mode)`);
      queryClient.invalidateQueries({ queryKey: ["bot-raw-config", botId] });
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const handleReset = () => {
    syncFromServer();
    toast.info("Config reset to server version");
  };

  return (
    <div className="space-y-4 mt-4">
      {/* Mode toggle */}
      <div className="space-y-2">
        <Label>Update Mode</Label>
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant={mode === "merge" ? "default" : "outline"}
            onClick={() => setMode("merge")}
          >
            Merge
          </Button>
          <Button
            size="sm"
            variant={mode === "replace" ? "default" : "outline"}
            onClick={() => setMode("replace")}
          >
            Replace
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          {mode === "merge"
            ? "Merge: deep-merges your changes into the existing config. Only specified fields are updated."
            : "Replace: completely overwrites the existing config with the provided JSON. Use with caution."}
        </p>
      </div>

      {/* Editor */}
      <div className="space-y-2">
        <Label htmlFor="config-editor">Configuration (JSON)</Label>
        {isLoading ? (
          <div className="h-64 bg-muted rounded animate-pulse" />
        ) : (
          <Textarea
            id="config-editor"
            className="font-mono min-h-[20rem]"
            rows={20}
            value={configText}
            onChange={(e) => handleTextChange(e.target.value)}
            aria-invalid={!!jsonError}
          />
        )}
        {jsonError && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircleIcon className="size-4 mt-0.5 shrink-0" />
            <span>{jsonError}</span>
          </div>
        )}
      </div>

      {/* Actions */}
      <div className="flex flex-wrap gap-2">
        <Button onClick={handleSave} disabled={saving || !!jsonError}>
          <SaveIcon className="size-4 mr-1" />
          {saving ? "Saving..." : "Save"}
        </Button>
        <Button variant="outline" onClick={handleReset} disabled={saving}>
          <RotateCcwIcon className="size-4 mr-1" />
          Reset
        </Button>
      </div>
    </div>
  );
}
