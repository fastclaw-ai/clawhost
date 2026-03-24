"use client";

import { useEffect, useState, useCallback } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
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
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useAuth } from "@/components/auth-provider";
import {
  listApps,
  createApp,
  updateApp,
  deleteApp,
  resetAppToken,
  type App,
} from "@/lib/api";

function AppActions({
  app,
  onEdit,
  onResetToken,
  onDelete,
}: {
  app: App;
  onEdit: () => void;
  onResetToken: () => void;
  onDelete: () => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="inline-flex items-center justify-center rounded-md text-sm font-medium h-8 px-2 hover:bg-accent hover:text-accent-foreground">
        ...
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={onEdit}>Edit</DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => {
            navigator.clipboard.writeText(app.api_token);
            toast.success("Token copied");
          }}
        >
          Copy Token
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onResetToken}>
          Reset Token
        </DropdownMenuItem>
        <DropdownMenuItem
          className="text-destructive"
          onClick={onDelete}
        >
          Delete
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export default function AppsPage() {
  const { isAuthed } = useAuth();
  const [apps, setApps] = useState<App[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [createForm, setCreateForm] = useState({
    name: "",
    url: "",
    description: "",
    owner_email: "",
    bot_domain_template: "",
  });
  const [editApp, setEditApp] = useState<App | null>(null);
  const [editForm, setEditForm] = useState({
    name: "",
    url: "",
    description: "",
    owner_email: "",
    bot_domain_template: "",
    status: "",
  });
  const [deleteTarget, setDeleteTarget] = useState<App | null>(null);
  const [tokenDisplay, setTokenDisplay] = useState<{
    app: App;
    token: string;
  } | null>(null);

  const fetchApps = useCallback(async () => {
    try {
      setLoading(true);
      const res = await listApps();
      setApps(res.data || []);
    } catch (err) {
      toast.error("Failed to load apps: " + (err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (isAuthed) fetchApps();
  }, [isAuthed, fetchApps]);

  const handleCreate = async () => {
    try {
      await createApp(createForm);
      toast.success("App created");
      setShowCreate(false);
      setCreateForm({ name: "", url: "", description: "", owner_email: "", bot_domain_template: "" });
      fetchApps();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const handleEdit = async () => {
    if (!editApp) return;
    try {
      await updateApp(editApp.id, editForm);
      toast.success("App updated");
      setEditApp(null);
      fetchApps();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deleteApp(deleteTarget.id);
      toast.success("App deleted");
      setDeleteTarget(null);
      fetchApps();
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const handleResetToken = async (app: App) => {
    try {
      const res = await resetAppToken(app.id);
      setTokenDisplay({ app, token: res.data.api_token });
      toast.success("Token reset");
    } catch (err) {
      toast.error((err as Error).message);
    }
  };

  const openEdit = (app: App) => {
    setEditApp(app);
    setEditForm({
      name: app.name,
      url: app.url,
      description: app.description,
      owner_email: app.owner_email,
      bot_domain_template: app.bot_domain_template,
      status: app.status,
    });
  };

  if (!isAuthed) return null;

  return (
    <div className="p-4 md:p-6 space-y-4 md:space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-xl md:text-2xl font-bold">Apps</h1>
          <p className="text-muted-foreground text-sm">
            Manage application instances
          </p>
        </div>
        <Button onClick={() => setShowCreate(true)} className="shrink-0">
          Create App
        </Button>
      </div>

      {loading ? (
        <div className="text-center py-8 text-muted-foreground">Loading...</div>
      ) : apps.length === 0 ? (
        <div className="text-center py-8 text-muted-foreground">No apps yet</div>
      ) : (
        <>
          {/* Desktop table */}
          <div className="hidden md:block border rounded-lg">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Owner</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>API Token</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="w-[80px]"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {apps.map((app) => (
                  <TableRow key={app.id}>
                    <TableCell>
                      <div>
                        <div className="font-medium">{app.name}</div>
                        {app.description && (
                          <div className="text-xs text-muted-foreground truncate max-w-[200px]">
                            {app.description}
                          </div>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-sm">
                      {app.owner_email || "-"}
                    </TableCell>
                    <TableCell>
                      <Badge variant={app.status === "active" ? "default" : "secondary"}>
                        {app.status}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <code className="text-xs bg-muted px-1.5 py-0.5 rounded">
                        {app.api_token.slice(0, 8)}...
                      </code>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {new Date(app.created_at).toLocaleDateString()}
                    </TableCell>
                    <TableCell>
                      <AppActions
                        app={app}
                        onEdit={() => openEdit(app)}
                        onResetToken={() => handleResetToken(app)}
                        onDelete={() => setDeleteTarget(app)}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          {/* Mobile cards */}
          <div className="md:hidden space-y-3">
            {apps.map((app) => (
              <Card key={app.id}>
                <CardContent className="p-4">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="font-medium truncate">{app.name}</span>
                        <Badge
                          variant={app.status === "active" ? "default" : "secondary"}
                          className="shrink-0"
                        >
                          {app.status}
                        </Badge>
                      </div>
                      {app.description && (
                        <p className="text-xs text-muted-foreground mt-1 truncate">
                          {app.description}
                        </p>
                      )}
                    </div>
                    <AppActions
                      app={app}
                      onEdit={() => openEdit(app)}
                      onResetToken={() => handleResetToken(app)}
                      onDelete={() => setDeleteTarget(app)}
                    />
                  </div>
                  <div className="mt-3 grid grid-cols-2 gap-2 text-xs text-muted-foreground">
                    <div>
                      <span className="text-foreground/50">Owner</span>
                      <p className="truncate">{app.owner_email || "-"}</p>
                    </div>
                    <div>
                      <span className="text-foreground/50">Created</span>
                      <p>{new Date(app.created_at).toLocaleDateString()}</p>
                    </div>
                    <div className="col-span-2">
                      <span className="text-foreground/50">Token</span>
                      <code className="block bg-muted px-1.5 py-0.5 rounded mt-0.5 truncate">
                        {app.api_token.slice(0, 16)}...
                      </code>
                    </div>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </>
      )}

      {/* Create Dialog */}
      <Dialog open={showCreate} onOpenChange={setShowCreate}>
        <DialogContent className="max-w-[95vw] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Create App</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Name *</Label>
              <Input
                value={createForm.name}
                onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
                placeholder="My App"
              />
            </div>
            <div className="space-y-2">
              <Label>URL</Label>
              <Input
                value={createForm.url}
                onChange={(e) => setCreateForm({ ...createForm, url: e.target.value })}
                placeholder="https://example.com"
              />
            </div>
            <div className="space-y-2">
              <Label>Description</Label>
              <Input
                value={createForm.description}
                onChange={(e) => setCreateForm({ ...createForm, description: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label>Owner Email</Label>
              <Input
                type="email"
                value={createForm.owner_email}
                onChange={(e) => setCreateForm({ ...createForm, owner_email: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label>Bot Domain Template</Label>
              <Input
                value={createForm.bot_domain_template}
                onChange={(e) => setCreateForm({ ...createForm, bot_domain_template: e.target.value })}
                placeholder="{slug}.example.com"
              />
            </div>
          </div>
          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button variant="outline" onClick={() => setShowCreate(false)} className="w-full sm:w-auto">
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={!createForm.name} className="w-full sm:w-auto">
              Create
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog open={!!editApp} onOpenChange={() => setEditApp(null)}>
        <DialogContent className="max-w-[95vw] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Edit App</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Name</Label>
              <Input
                value={editForm.name}
                onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label>URL</Label>
              <Input
                value={editForm.url}
                onChange={(e) => setEditForm({ ...editForm, url: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label>Description</Label>
              <Input
                value={editForm.description}
                onChange={(e) => setEditForm({ ...editForm, description: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label>Owner Email</Label>
              <Input
                type="email"
                value={editForm.owner_email}
                onChange={(e) => setEditForm({ ...editForm, owner_email: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label>Bot Domain Template</Label>
              <Input
                value={editForm.bot_domain_template}
                onChange={(e) => setEditForm({ ...editForm, bot_domain_template: e.target.value })}
              />
            </div>
            <div className="space-y-2">
              <Label>Status</Label>
              <div className="flex gap-2">
                <Button
                  size="sm"
                  variant={editForm.status === "active" ? "default" : "outline"}
                  onClick={() => setEditForm({ ...editForm, status: "active" })}
                >
                  Active
                </Button>
                <Button
                  size="sm"
                  variant={editForm.status === "disabled" ? "default" : "outline"}
                  onClick={() => setEditForm({ ...editForm, status: "disabled" })}
                >
                  Disabled
                </Button>
              </div>
            </div>
          </div>
          <DialogFooter className="flex-col sm:flex-row gap-2">
            <Button variant="outline" onClick={() => setEditApp(null)} className="w-full sm:w-auto">
              Cancel
            </Button>
            <Button onClick={handleEdit} className="w-full sm:w-auto">Save</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation */}
      <AlertDialog open={!!deleteTarget} onOpenChange={() => setDeleteTarget(null)}>
        <AlertDialogContent className="max-w-[95vw] sm:max-w-lg">
          <AlertDialogHeader>
            <AlertDialogTitle>Delete App</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete &quot;{deleteTarget?.name}&quot;?
              This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete}>Delete</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Token Display Dialog */}
      <Dialog open={!!tokenDisplay} onOpenChange={() => setTokenDisplay(null)}>
        <DialogContent className="max-w-[95vw] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>New API Token</DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">
              Copy this token now. It won&apos;t be shown again.
            </p>
            <code className="block p-3 bg-muted rounded text-xs break-all select-all">
              {tokenDisplay?.token}
            </code>
          </div>
          <DialogFooter>
            <Button
              onClick={() => {
                if (tokenDisplay) {
                  navigator.clipboard.writeText(tokenDisplay.token);
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
