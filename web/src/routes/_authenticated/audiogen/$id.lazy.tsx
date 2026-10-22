import { createLazyFileRoute } from "@tanstack/react-router";
import { AudioGenConfigPage } from "@/features/audiogen/components/audiogen-config-page";
export const Route = createLazyFileRoute("/_authenticated/audiogen/$id")({
  component: Page,
});
function Page() {
  const { id } = Route.useParams();
  return <AudioGenConfigPage id={Number(id)} />;
}
