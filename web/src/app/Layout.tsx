import { useApi } from "../shared/api/context";
import { href, useRoute, type Route } from "../shared/router";
import { useAsync } from "../shared/useAsync";
import { productName } from "../shared/product";
import { Empty } from "../shared/ui/States";
import { ProjectsPage } from "../features/projects/ProjectsPage";
import { OverviewPage } from "../features/projects/OverviewPage";
import { EndpointsPage } from "../features/endpoints/EndpointsPage";
import { EventsPage } from "../features/events/EventsPage";
import { EventDetailPage } from "../features/events/EventDetailPage";
import { DeliveriesPage } from "../features/deliveries/DeliveriesPage";
import { DeliveryDetailPage } from "../features/deliveries/DeliveryDetailPage";

export function Layout({ onSignOut }: { onSignOut: () => void }) {
  const route = useRoute();
  const projectId = "projectId" in route ? route.projectId : null;

  return (
    <div className="shell">
      <aside className="sidebar">
        <a className="brand" href={href("projects")}>
          {productName}
        </a>
        {projectId && <ProjectNav projectId={projectId} route={route} />}
        <button type="button" className="button signout" onClick={onSignOut}>
          Sign out
        </button>
      </aside>
      <main className="content">
        <Page route={route} />
      </main>
    </div>
  );
}

function ProjectNav({ projectId, route }: { projectId: string; route: Route }) {
  const api = useApi();
  const [project] = useAsync((signal) => api.getProject(projectId, signal), [api, projectId]);
  const items: { label: string; target: string; active: boolean }[] = [
    { label: "Overview", target: href("projects", projectId), active: route.name === "overview" },
    { label: "Endpoints", target: href("projects", projectId, "endpoints"), active: route.name === "endpoints" },
    { label: "Events", target: href("projects", projectId, "events"), active: route.name === "events" || route.name === "event" },
    { label: "Deliveries", target: href("projects", projectId, "deliveries"), active: route.name === "deliveries" || route.name === "delivery" },
  ];
  return (
    <nav aria-label="Project">
      <a className="project-switch" href={href("projects")} title="All projects">
        {project.status === "ready" ? project.data.name : "Project"}
        <span className="muted"> ⇄</span>
      </a>
      <ul className="nav">
        {items.map((item) => (
          <li key={item.label}>
            <a href={item.target} aria-current={item.active ? "page" : undefined}>
              {item.label}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function Page({ route }: { route: Route }) {
  switch (route.name) {
    case "projects":
      return <ProjectsPage />;
    case "overview":
      return <OverviewPage projectId={route.projectId} />;
    case "endpoints":
      return <EndpointsPage projectId={route.projectId} />;
    case "events":
      return <EventsPage projectId={route.projectId} />;
    case "event":
      return <EventDetailPage key={route.eventId} projectId={route.projectId} eventId={route.eventId} />;
    case "deliveries":
      return <DeliveriesPage projectId={route.projectId} query={route.query} />;
    case "delivery":
      return <DeliveryDetailPage key={route.deliveryId} projectId={route.projectId} deliveryId={route.deliveryId} />;
    case "not_found":
      return (
        <Empty title="Page not found">
          <a href={href("projects")}>Back to projects</a>
        </Empty>
      );
  }
}
