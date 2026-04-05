"use client";

import { useState, useEffect, useCallback, lazy, Suspense } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  SaveIcon,
  RotateCcwIcon,
  AlertCircleIcon,
  CopyIcon,
  WrapTextIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  getBotRawConfig,
  updateBotRawConfig,
  getErrorMessage,
} from "@/lib/api";

const CodeEditor = lazy(
  () => import("@uiw/react-textarea-code-editor").then((m) => ({ default: m.default }))
);

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
  const [wordWrap, setWordWrap] = useState(true);

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

  const handleFormat = () => {
    const parsed = validateJson(configText);
    if (parsed) {
      setConfigText(JSON.stringify(parsed, null, 2));
      toast.success("Formatted");
    }
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(configText);
    toast.success("Copied to clipboard");
  };

  const handleSave = async () => {
    const parsed = validateJson(configText);
    if (!parsed) {
      toast.error("Invalid JSON — fix errors before saving");
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
    toast.info("Reset to server version");
  };

  const lineCount = configText.split("\n").length;

  return (
    <div className="space-y-4 mt-4">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        {/* Mode toggle */}
        <div className="flex items-center gap-2">
          <Label className="text-xs text-muted-foreground shrink-0">Mode:</Label>
          <Button
            size="sm"
            variant={mode === "merge" ? "default" : "outline"}
            onClick={() => setMode("merge")}
            className="h-7 text-xs"
          >
            Merge
          </Button>
          <Button
            size="sm"
            variant={mode === "replace" ? "default" : "outline"}
            onClick={() => setMode("replace")}
            className="h-7 text-xs"
          >
            Replace
          </Button>
        </div>

        {/* Tools */}
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" className="h-7 text-xs" onClick={handleFormat}>
            {"{ }"}  Format
          </Button>
          <Button variant="ghost" size="sm" className="h-7 text-xs" onClick={handleCopy}>
            <CopyIcon className="size-3 mr-1" /> Copy
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 text-xs"
            onClick={() => setWordWrap((w) => !w)}
          >
            <WrapTextIcon className="size-3 mr-1" /> {wordWrap ? "No Wrap" : "Wrap"}
          </Button>
        </div>
      </div>

      <p className="text-xs text-muted-foreground">
        {mode === "merge"
          ? "Merge: deep-merges your changes into existing config. Only specified fields update."
          : "Replace: completely overwrites the config. Use with caution."}
      </p>

      {/* Editor */}
      {isLoading ? (
        <div className="h-96 bg-muted rounded animate-pulse" />
      ) : (
        <div className="relative border rounded-lg overflow-hidden">
          <Suspense
            fallback={
              <textarea
                className="w-full min-h-[24rem] p-4 font-mono text-sm bg-[#1e1e1e] text-[#d4d4d4] resize-y"
                value={configText}
                onChange={(e) => handleTextChange(e.target.value)}
              />
            }
          >
            <CodeEditor
              value={configText}
              language="json"
              onChange={(e) => handleTextChange(e.target.value)}
              padding={16}
              style={{
                fontSize: 13,
                fontFamily: "var(--font-geist-mono), ui-monospace, monospace",
                minHeight: "24rem",
                backgroundColor: "#1e1e1e",
                overflowWrap: wordWrap ? "break-word" : "normal",
                whiteSpace: wordWrap ? "pre-wrap" : "pre",
              }}
              data-color-mode="dark"
            />
          </Suspense>

          {/* Status bar */}
          <div className="flex items-center justify-between px-3 py-1 bg-[#252526] text-[#858585] text-xs border-t border-[#3c3c3c]">
            <span>{lineCount} lines</span>
            <span className={jsonError ? "text-red-400" : "text-green-400"}>
              {jsonError ? `Error: ${jsonError}` : "Valid JSON"}
            </span>
          </div>
        </div>
      )}

      {jsonError && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircleIcon className="size-4 mt-0.5 shrink-0" />
          <span>{jsonError}</span>
        </div>
      )}

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
