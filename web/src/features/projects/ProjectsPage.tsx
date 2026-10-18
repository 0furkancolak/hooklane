import { useApi } from "../../shared/api/context";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { formatTime } from "../../shared/format";
import { Icon } from "../../shared/ui/Icon";
import { Empty, ErrorState, Loading } from "../../shared/ui/States";

export function ProjectsPage() {
  const api = useApi();
  const [state, reload] = useAsync((signal) => api.listProjects({ limit: 100 }, signal), [api]);

  return (
    <section>
      <div className="section-head">
        <div>
          <h1>Projects</h1>
          <p className="subtitle">Each project has its own endpoints, events and deliveries.</p>
        </div>
      </div>
      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" &&
        (state.data.projects.length === 0 ? (
          <Empty title="No projects yet">Create one with POST /v1/projects.</Empty>
        ) : (
          <ul className="cards">
            {state.data.projects.map((project) => (
              <li key={project.id}>
                <a className="card" href={href("projects", project.id)}>
                  <span className="project-avatar">{project.name.slice(0, 1).toUpperCase()}</span>
                  <span className="card-text">
                    <strong>{project.name}</strong>
                    <span className="muted">Created {formatTime(project.created_at)}</span>
                  </span>
                  <Icon name="arrow" size={16} />
                </a>
              </li>
            ))}
          </ul>
        ))}
    </section>
  );
}
