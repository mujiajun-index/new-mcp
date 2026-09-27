import { createLazyFileRoute } from "@tanstack/react-router";
import { SystemOneListPage } from "@/features/system-one/components/system-one-list-page";

export const Route = createLazyFileRoute("/_authenticated/system-one/")({
  component: SystemOneListPage,
});
