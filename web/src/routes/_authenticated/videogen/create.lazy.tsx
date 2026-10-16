import { createLazyFileRoute } from "@tanstack/react-router";
import { VideoGenConfigPage } from "@/features/videogen/components/videogen-config-page";
export const Route = createLazyFileRoute("/_authenticated/videogen/create")({
  component: () => <VideoGenConfigPage />,
});
