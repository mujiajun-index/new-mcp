import { createLazyFileRoute } from "@tanstack/react-router";
import { AudioGenConfigPage } from "@/features/audiogen/components/audiogen-config-page";
export const Route = createLazyFileRoute("/_authenticated/audiogen/create")({
  component: () => <AudioGenConfigPage />,
});
