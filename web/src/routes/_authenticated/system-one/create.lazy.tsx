import { createLazyFileRoute } from "@tanstack/react-router";
import { SystemOneCreatePage } from "@/features/system-one/components/system-one-create-page";

export const Route = createLazyFileRoute("/_authenticated/system-one/create")({
  component: SystemOneCreatePage,
});
