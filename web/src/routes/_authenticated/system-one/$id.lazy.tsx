import { createLazyFileRoute } from "@tanstack/react-router";
import { SystemOneDetailPage } from "@/features/system-one/components/system-one-detail-page";

export const Route = createLazyFileRoute("/_authenticated/system-one/$id")({
  component: SystemOneDetailPage,
});
