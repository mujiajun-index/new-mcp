import { createLazyFileRoute } from "@tanstack/react-router";
import { VideoGenConfigPage } from "@/features/videogen/components/videogen-config-page";
export const Route = createLazyFileRoute("/_authenticated/videogen/$id")({
  component: Page,
});
function Page() {
  const { id } = Route.useParams();
  return <VideoGenConfigPage id={Number(id)} />;
}
