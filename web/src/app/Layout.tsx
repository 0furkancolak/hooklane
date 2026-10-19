import { useState, type FormEvent } from "react";
import { useApi } from "../shared/api/context";
import { href, useRoute, type Route } from "../shared/router";
import { useAsync } from "../shared/useAsync";
import { productName } from "../shared/product";
import { Empty } from "../shared/ui/States";
import { Icon, type IconName } from "../shared/ui/Icon";
import { useTheme } from "../shared/theme";
import { languages, t, useLang } from "../shared/i18n";
import { describeError } from "../shared/errors";
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
  const { lang, setLang } = useLang();
  const api = useApi();
  const [project] = useAsync((signal) => (projectId ? api.getProject(projectId, signal) : Promise.resolve(null)), [api, projectId]);
  const projectName = project.status === "ready" && project.data ? project.data.name : null;

  return (
    <div className="shell">
      <aside className="sidebar">
        <a className="brand" href={href("projects")}>
          {productName}
        </a>
        {projectId ? <ProjectNav projectId={projectId} projectName={projectName} route={route} /> : <HomeNav />}
        {api.isSession && <AccountMenu />}
        <div className="sidebar-foot">
          <div className="segmented" role="group" aria-label={t("nav.language")}>
            {languages.map((l) => (
              <button key={l.code} type="button" aria-pressed={lang === l.code} onClick={() => setLang(l.code)}>
                {l.label}
              </button>
            ))}
          </div>
          <button type="button" className="button button-ghost" onClick={toggleTheme} aria-label={t("nav.switchTheme")}>
            <Icon name={theme === "dark" ? "sun" : "moon"} />
            <span className="btn-label">{theme === "dark" ? t("nav.lightTheme") : t("nav.darkTheme")}</span>
          </button>
          <button type="button" className="button button-ghost signout" onClick={onSignOut} aria-label={t("nav.signOut")}>
            <Icon name="logout" />
            <span className="btn-label">{t("nav.signOut")}</span>
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

function pageTitle(name: Route["name"]): string {
  switch (name) {
    case "projects":
      return t("nav.projects");
    case "overview":
      return t("nav.overview");
    case "playground":
      return t("nav.playground");
    case "endpoints":
      return t("nav.endpoints");
    case "api_keys":
      return t("nav.apiKeys");
    case "events":
      return t("nav.events");
    case "event":
      return t("crumb.event");
    case "deliveries":
      return t("nav.deliveries");
    case "delivery":
      return t("crumb.delivery");
    case "not_found":
    case "portal":
      return t("crumb.notFound");
  }
}

function Topbar({ route, projectId, projectName }: { route: Route; projectId: string | null; projectName: string | null }) {
  return (
    <header className="topbar">
      <nav aria-label={t("nav.breadcrumb")} className="breadcrumb">
        <a href={href("projects")}>{t("nav.projects")}</a>
        {projectId && (
          <>
            <span aria-hidden="true">/</span>
            <a href={href("projects", projectId)}>{projectName ?? t("nav.project")}</a>
          </>
        )}
        {route.name !== "projects" && route.name !== "overview" && (
          <>
            <span aria-hidden="true">/</span>
            <span aria-current="page">{pageTitle(route.name)}</span>
          </>
        )}
      </nav>
    </header>
  );
}

function HomeNav() {
  return (
    <nav aria-label={t("nav.main")} className="project-nav">
      <ul className="nav">
        <li>
          <a href={href("projects")} aria-current="page">
            <Icon name="overview" />
            {t("nav.projects")}
          </a>
        </li>
        <li>
          <a href="/openapi.yaml" target="_blank" rel="noreferrer">
            <Icon name="events" />
            {t("nav.apiReference")}
          </a>
        </li>
      </ul>
      <p className="nav-hint">{t("nav.openProjectHint")}</p>
    </nav>
  );
}

function ProjectNav({ projectId, projectName, route }: { projectId: string; projectName: string | null; route: Route }) {
  const items: { label: string; icon: IconName; target: string; active: boolean }[] = [
    { label: t("nav.overview"), icon: "overview", target: href("projects", projectId), active: route.name === "overview" },
    { label: t("nav.playground"), icon: "play", target: href("projects", projectId, "playground"), active: route.name === "playground" },
    { label: t("nav.endpoints"), icon: "endpoints", target: href("projects", projectId, "endpoints"), active: route.name === "endpoints" },
    { label: t("nav.events"), icon: "events", target: href("projects", projectId, "events"), active: route.name === "events" || route.name === "event" },
    { label: t("nav.deliveries"), icon: "deliveries", target: href("projects", projectId, "deliveries"), active: route.name === "deliveries" || route.name === "delivery" },
    { label: t("nav.apiKeys"), icon: "key", target: href("projects", projectId, "api-keys"), active: route.name === "api_keys" },
  ];
  return (
    <nav aria-label={t("nav.project")} className="project-nav">
      <a className="project-switch" href={href("projects")} title={t("nav.allProjects")}>
        <span className="project-avatar">{projectName ? projectName.slice(0, 1).toUpperCase() : "·"}</span>
        <span className="project-switch-text">
          <small>{t("nav.project")}</small>
          {projectName ?? t("common.loading")}
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
      return <EndpointsPage projectId={route.projectId} query={route.query} />;
    case "api_keys":
      return <APIKeysPage projectId={route.projectId} />;
    case "events":
      return <EventsPage projectId={route.projectId} query={route.query} />;
    case "event":
      return <EventDetailPage key={route.eventId} projectId={route.projectId} eventId={route.eventId} />;
    case "deliveries":
      return <DeliveriesPage projectId={route.projectId} query={route.query} />;
    case "delivery":
      return <DeliveryDetailPage key={route.deliveryId} projectId={route.projectId} deliveryId={route.deliveryId} />;
    case "not_found":
    case "portal":
      return (
        <Empty title={t("notFound.title")}>
          <a href={href("projects")}>{t("notFound.back")}</a>
        </Empty>
      );
  }
}

function AccountMenu() {
  const api = useApi();
  const [state] = useAsync((signal) => api.me(signal), [api]);
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);

  // The active organization decides which projects the API returns, so a
  // switch reloads everything from the projects list.
  function enter() {
    window.location.hash = "#/projects";
    window.location.reload();
  }

  async function switchTo(id: string) {
    setError(null);
    try {
      await api.switchOrganization(id);
      enter();
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : t("account.failed"));
    }
  }

  async function create(event: FormEvent) {
    event.preventDefault();
    setError(null);
    try {
      await api.createOrganization(name.trim());
      enter();
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : t("account.failed"));
    }
  }

  if (state.status !== "ready") return null;
  const { user, organization, organizations } = state.data;
  return (
    <div className="account-menu">
      <small className="muted" title={user.email}>
        {user.email}
      </small>
      <label htmlFor="org-switch">{t("account.organization")}</label>
      <select id="org-switch" value={organization.id} onChange={(e) => void switchTo(e.target.value)}>
        {organizations.map((o) => (
          <option key={o.id} value={o.id}>
            {o.name}
          </option>
        ))}
      </select>
      {adding ? (
        <form onSubmit={(e) => void create(e)}>
          <input aria-label={t("account.newOrganizationName")} value={name} onChange={(e) => setName(e.target.value)} placeholder={t("account.newOrganizationName")} />
          <div className="actions">
            <button type="submit" className="button button-primary" disabled={name.trim() === ""}>
              {t("account.create")}
            </button>
            <button type="button" className="button" onClick={() => setAdding(false)}>
              {t("account.cancel")}
            </button>
          </div>
        </form>
      ) : (
        <button type="button" className="button button-ghost" onClick={() => setAdding(true)}>
          {t("account.newOrganization")}
        </button>
      )}
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
