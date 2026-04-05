import { Badge } from "@/components/ui/badge";

const statusStyles: Record<string, string> = {
  running: "bg-green-100 text-green-700 border-green-200",
  starting: "bg-yellow-100 text-yellow-700 border-yellow-200",
  created: "bg-blue-100 text-blue-700 border-blue-200",
  stopped: "bg-gray-100 text-gray-600 border-gray-200",
  error: "bg-red-100 text-red-700 border-red-200",
  active: "",
  disabled: "bg-gray-100 text-gray-600 border-gray-200",
};

interface StatusBadgeProps {
  status: string;
  variant?: "default" | "outline";
}

export function StatusBadge({ status, variant = "outline" }: StatusBadgeProps) {
  return (
    <Badge variant={variant} className={statusStyles[status] || ""}>
      {status}
    </Badge>
  );
}
