"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { PlusIcon, PencilIcon, Trash2Icon, FileTextIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  listSkills,
  updateSkill,
  deleteSkill,
  getErrorMessage,
} from "@/lib/api";

interface SkillsTabProps {
  readonly botId: string;
  readonly botStatus: string;
}

interface SkillForm {
  readonly name: string;
  readonly content: string;
}

const EMPTY_FORM: SkillForm = { name: "", content: "" };

export function SkillsTab({ botId, botStatus }: SkillsTabProps) {
  const queryClient = useQueryClient();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogMode, setDialogMode] = useState<"add" | "edit">("add");
  const [form, setForm] = useState<SkillForm>(EMPTY_FORM);
  const [saving, setSaving] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  const { data: skills, isLoading } = useQuery({
    queryKey: ["skills", botId],
    queryFn: () => listSkills(botId).then((r) => r.data),
    enabled: botStatus === "running",
  });

  if (botStatus !== "running") {
    return (
      <div className="text-center py-12 text-muted-foreground">
        Bot must be running to manage skills.
      </div>
    );
  }

  const openAdd = () => {
    setForm(EMPTY_FORM);
    setDialogMode("add");
    setDialogOpen(true);
  };

  const openEdit = (name: string) => {
    setForm({ name, content: "" });
    setDialogMode("edit");
    setDialogOpen(true);
  };

  const handleSave = async () => {
    if (!form.name.trim()) {
      toast.error("Skill name is required");
      return;
    }
    if (!form.content.trim()) {
      toast.error("Skill content is required");
      return;
    }
    try {
      setSaving(true);
      await updateSkill(botId, form.name.trim(), form.content);
      toast.success(
        dialogMode === "add" ? "Skill created" : "Skill updated",
      );
      setDialogOpen(false);
      queryClient.invalidateQueries({ queryKey: ["skills", botId] });
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      setDeleting(true);
      await deleteSkill(botId, deleteTarget);
      toast.success("Skill deleted");
      setDeleteTarget(null);
      queryClient.invalidateQueries({ queryKey: ["skills", botId] });
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="space-y-4 mt-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-muted-foreground">
          Skills ({skills?.length ?? 0})
        </h3>
        <Button size="sm" onClick={openAdd}>
          <PlusIcon className="size-4 mr-1" /> Add Skill
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          <div className="h-10 bg-muted rounded animate-pulse" />
          <div className="h-10 bg-muted rounded animate-pulse" />
        </div>
      ) : !skills?.length ? (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            <FileTextIcon className="size-8 mx-auto mb-2 opacity-50" />
            <p>No skills configured yet.</p>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead className="w-24 text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {skills.map((skill) => (
                <TableRow key={skill.name}>
                  <TableCell className="font-mono text-sm">
                    {skill.name}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-7"
                        onClick={() => openEdit(skill.name)}
                      >
                        <PencilIcon className="size-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-7 text-destructive hover:text-destructive"
                        onClick={() => setDeleteTarget(skill.name)}
                      >
                        <Trash2Icon className="size-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* Add / Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-[95vw] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {dialogMode === "add" ? "Add Skill" : `Edit Skill: ${form.name}`}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            {dialogMode === "add" && (
              <div className="space-y-2">
                <Label htmlFor="skill-name">Name</Label>
                <Input
                  id="skill-name"
                  placeholder="e.g. greeting"
                  value={form.name}
                  onChange={(e) =>
                    setForm({ ...form, name: e.target.value })
                  }
                />
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="skill-content">Content (Markdown)</Label>
              <Textarea
                id="skill-content"
                className="font-mono min-h-48"
                placeholder="Enter skill content..."
                rows={10}
                value={form.content}
                onChange={(e) =>
                  setForm({ ...form, content: e.target.value })
                }
              />
              {dialogMode === "edit" && (
                <p className="text-xs text-muted-foreground">
                  Enter new content for this skill. This will replace the
                  existing content.
                </p>
              )}
            </div>
          </div>
          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button
              variant="outline"
              onClick={() => setDialogOpen(false)}
              className="w-full sm:w-auto"
            >
              Cancel
            </Button>
            <Button
              onClick={handleSave}
              disabled={saving}
              className="w-full sm:w-auto"
            >
              {saving ? "Saving..." : "Save"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation */}
      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title="Delete Skill"
        description={`Are you sure you want to delete the skill "${deleteTarget}"? This action cannot be undone.`}
        confirmLabel="Delete"
        loading={deleting}
        onConfirm={handleDelete}
        variant="destructive"
      />
    </div>
  );
}
