import { useApi } from "../../shared/api/context";
import { href } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { formatTime } from "../../shared/format";
import { Empty, ErrorState, Loading } from "../../shared/ui/States";

export function ProjectsPage() {
  const api = useApi();
  const [state, reload] = useAsync((signal) => api.listProjects({ limit: 100 }, signal), [api]);

  return (
    <section>
      <h1>Projects</h1>
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
                  <strong>{project.name}</strong>
                  <span className="muted">Created {formatTime(project.created_at)}</span>
                </a>
              </li>
            ))}
          </ul>
        ))}
    </section>
  );
}
