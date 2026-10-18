import { useApi } from "../shared/api/context";
import { href, useRoute, type Route } from "../shared/router";
import { useAsync } from "../shared/useAsync";
import { productName } from "../shared/product";
import { Empty } from "../shared/ui/States";
import { Icon, type IconName } from "../shared/ui/Icon";
import { useTheme } from "../shared/theme";
import { ProjectsPage } from "../features/projects/ProjectsPage";
import { OverviewPage } from "../features/projects/OverviewPage";
import { PlaygroundPage } from "../features/playground/PlaygroundPage";
import { EndpointsPage } from "../features/endpoints/EndpointsPage";
import { APIKeysPage } from "../features/apikeys/APIKeysPage";
import { EventsPage } from "../features/events/EventsPage";
import { EventDetailPage } from "../features/events/EventDetailPage";
import { DeliveriesPage } from "../features/deliveries/DeliveriesPage";
import { DeliveryDetailPage } from "../features/deliveries/DeliveryDetailPage";

export function Layout({ onSignOut }: { onSignOut: () => void }) {
  const route = useRoute();
  const projectId = "projectId" in route ? route.projectId : null;
  const [theme, toggleTheme] = useTheme();
  const api = useApi();
  const [project] = useAsync((signal) => (projectId ? api.getProject(projectId, signal) : Promise.resolve(null)), [api, projectId]);
  const projectName = project.status === "ready" && project.data ? project.data.name : null;

  return (
    <div className="shell">
      <aside className="sidebar">
        <a className="brand" href={href("projects")}>
          <span className="brand-mark">
            <Icon name="bolt" size={16} />
          </span>
          {productName}
        </a>
        {projectId ? <ProjectNav projectId={projectId} projectName={projectName} route={route} /> : <HomeNav />}
        <div className="sidebar-foot">
          <button type="button" className="button button-ghost" onClick={toggleTheme} aria-label="Switch theme">
            <Icon name={theme === "dark" ? "sun" : "moon"} />
            <span className="btn-label">{theme === "dark" ? "Light theme" : "Dark theme"}</span>
          </button>
          <button type="button" className="button button-ghost signout" onClick={onSignOut} aria-label="Sign out">
            <Icon name="logout" />
            <span className="btn-label">Sign out</span>
          </button>
        </div>
      </aside>
      <div className="main">
        <Topbar route={route} projectId={projectId} projectName={projectName} />
        <main className="content">
          <Page route={route} />
        </main>
      </div>
    </div>
  );
}

const pageTitles: Record<Route["name"], string> = {
  projects: "Projects",
  overview: "Overview",
  endpoints: "Endpoints",
  playground: "Playground",
  api_keys: "API keys",
  events: "Events",
  event: "Event",
  deliveries: "Deliveries",
  delivery: "Delivery",
  not_found: "Not found",
};

function Topbar({ route, projectId, projectName }: { route: Route; projectId: string | null; projectName: string | null }) {
  return (
    <header className="topbar">
      <nav aria-label="Breadcrumb" className="breadcrumb">
        <a href={href("projects")}>Projects</a>
        {projectId && (
          <>
            <span aria-hidden="true">/</span>
            <a href={href("projects", projectId)}>{projectName ?? "Project"}</a>
          </>
        )}
        {route.name !== "projects" && route.name !== "overview" && (
          <>
            <span aria-hidden="true">/</span>
            <span aria-current="page">{pageTitles[route.name]}</span>
          </>
        )}
      </nav>
    </header>
  );
}

function HomeNav() {
  return (
    <nav aria-label="Main" className="project-nav">
      <ul className="nav">
        <li>
          <a href={href("projects")} aria-current="page">
            <Icon name="overview" />
            Projects
          </a>
        </li>
        <li>
          <a href="/openapi.yaml" target="_blank" rel="noreferrer">
            <Icon name="events" />
            API reference
          </a>
        </li>
      </ul>
      <p className="nav-hint">Open a project to see its playground, endpoints, events and deliveries.</p>
    </nav>
  );
}

function ProjectNav({ projectId, projectName, route }: { projectId: string; projectName: string | null; route: Route }) {
  const items: { label: string; icon: IconName; target: string; active: boolean }[] = [
    { label: "Overview", icon: "overview", target: href("projects", projectId), active: route.name === "overview" },
    { label: "Playground", icon: "play", target: href("projects", projectId, "playground"), active: route.name === "playground" },
    { label: "Endpoints", icon: "endpoints", target: href("projects", projectId, "endpoints"), active: route.name === "endpoints" },
    { label: "Events", icon: "events", target: href("projects", projectId, "events"), active: route.name === "events" || route.name === "event" },
    { label: "Deliveries", icon: "deliveries", target: href("projects", projectId, "deliveries"), active: route.name === "deliveries" || route.name === "delivery" },
    { label: "API keys", icon: "key", target: href("projects", projectId, "api-keys"), active: route.name === "api_keys" },
  ];
  return (
    <nav aria-label="Project" className="project-nav">
      <a className="project-switch" href={href("projects")} title="All projects">
        <span className="project-avatar">{projectName ? projectName.slice(0, 1).toUpperCase() : "·"}</span>
        <span className="project-switch-text">
          <small>Project</small>
          {projectName ?? "Loading…"}
        </span>
        <Icon name="swap" size={14} />
      </a>
      <ul className="nav">
        {items.map((item) => (
          <li key={item.label}>
            <a href={item.target} aria-current={item.active ? "page" : undefined}>
              <Icon name={item.icon} />
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
    case "playground":
      return <PlaygroundPage projectId={route.projectId} />;
    case "endpoints":
      return <EndpointsPage projectId={route.projectId} />;
    case "api_keys":
      return <APIKeysPage projectId={route.projectId} />;
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
