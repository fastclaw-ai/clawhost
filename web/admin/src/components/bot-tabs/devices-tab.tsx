"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  SmartphoneIcon,
  MonitorIcon,
  CheckIcon,
  XIcon,
  CircleIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  listDevices,
  approveDevice,
  revokeDevice,
  getErrorMessage,
  type Device,
} from "@/lib/api";

interface DevicesTabProps {
  readonly botId: string;
  readonly botStatus: string;
}

function PlatformIcon({ platform }: { readonly platform: string }) {
  const lower = platform.toLowerCase();
  if (lower.includes("mobile") || lower.includes("phone") || lower.includes("ios") || lower.includes("android")) {
    return <SmartphoneIcon className="size-4 text-muted-foreground" />;
  }
  return <MonitorIcon className="size-4 text-muted-foreground" />;
}

function DeviceCard({
  device,
  actions,
}: {
  readonly device: Device;
  readonly actions: React.ReactNode;
}) {
  return (
    <Card>
      <CardContent className="flex items-center justify-between gap-4 py-4">
        <div className="flex items-center gap-3 min-w-0">
          <PlatformIcon platform={device.platform} />
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium truncate">
                {device.platform}
              </span>
              <Badge variant="secondary" className="text-xs shrink-0">
                {device.client_mode}
              </Badge>
              {device.connected ? (
                <CircleIcon className="size-2.5 fill-green-500 text-green-500 shrink-0" />
              ) : (
                <CircleIcon className="size-2.5 fill-gray-300 text-gray-300 shrink-0" />
              )}
            </div>
            <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
              <span>{device.status}</span>
              <span>·</span>
              <span>{device.age}</span>
              {device.role && (
                <>
                  <span>·</span>
                  <span>{device.role}</span>
                </>
              )}
            </div>
          </div>
        </div>
        <div className="shrink-0">{actions}</div>
      </CardContent>
    </Card>
  );
}

function DeviceList({
  botId,
  status,
}: {
  readonly botId: string;
  readonly status: "pending" | "paired";
}) {
  const queryClient = useQueryClient();
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<Device | null>(null);
  const [revoking, setRevoking] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ["devices", botId, status],
    queryFn: () => listDevices(botId, status).then((r) => r.data),
  });

  const devices = data?.devices ?? [];

  const invalidateAll = () => {
    queryClient.invalidateQueries({ queryKey: ["devices", botId] });
  };

  const handleApprove = async (device: Device) => {
    try {
      setActionLoading(device.device_id);
      await approveDevice(botId, device.device_id);
      toast.success("Device approved");
      invalidateAll();
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setActionLoading(null);
    }
  };

  const handleRevoke = async () => {
    if (!revokeTarget) return;
    try {
      setRevoking(true);
      await revokeDevice(botId, revokeTarget.device_id);
      toast.success("Device revoked");
      setRevokeTarget(null);
      invalidateAll();
    } catch (err) {
      toast.error(getErrorMessage(err));
    } finally {
      setRevoking(false);
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-3">
        <div className="h-20 bg-muted rounded animate-pulse" />
        <div className="h-20 bg-muted rounded animate-pulse" />
      </div>
    );
  }

  if (!devices.length) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        No {status} devices.
      </div>
    );
  }

  return (
    <>
      <div className="space-y-3">
        {devices.map((device) => (
          <DeviceCard
            key={device.device_id}
            device={device}
            actions={
              status === "pending" ? (
                <Button
                  size="sm"
                  onClick={() => handleApprove(device)}
                  disabled={actionLoading === device.device_id}
                >
                  <CheckIcon className="size-4 mr-1" />
                  {actionLoading === device.device_id
                    ? "Approving..."
                    : "Approve"}
                </Button>
              ) : (
                <Button
                  size="sm"
                  variant="outline"
                  className="text-destructive hover:text-destructive"
                  onClick={() => setRevokeTarget(device)}
                >
                  <XIcon className="size-4 mr-1" />
                  Revoke
                </Button>
              )
            }
          />
        ))}
      </div>

      <ConfirmDialog
        open={!!revokeTarget}
        onOpenChange={(open) => {
          if (!open) setRevokeTarget(null);
        }}
        title="Revoke Device"
        description={`Are you sure you want to revoke device "${revokeTarget?.platform ?? ""}" (${revokeTarget?.device_id.slice(0, 8) ?? ""}...)? The device will need to be re-paired.`}
        confirmLabel="Revoke"
        loading={revoking}
        onConfirm={handleRevoke}
        variant="destructive"
      />
    </>
  );
}

export function DevicesTab({ botId, botStatus }: DevicesTabProps) {
  if (botStatus !== "running") {
    return (
      <div className="text-center py-12 text-muted-foreground">
        Bot must be running to manage devices.
      </div>
    );
  }

  return (
    <div className="space-y-4 mt-4">
      <Tabs defaultValue="pending">
        <TabsList>
          <TabsTrigger value="pending">Pending</TabsTrigger>
          <TabsTrigger value="paired">Paired</TabsTrigger>
        </TabsList>
        <TabsContent value="pending" className="mt-4">
          <DeviceList botId={botId} status="pending" />
        </TabsContent>
        <TabsContent value="paired" className="mt-4">
          <DeviceList botId={botId} status="paired" />
        </TabsContent>
      </Tabs>
    </div>
  );
}
