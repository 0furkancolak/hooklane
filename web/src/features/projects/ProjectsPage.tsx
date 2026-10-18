import { useState, type FormEvent } from "react";
import { useApi } from "../../shared/api/context";
import { describeError } from "../../shared/errors";
import { formatTime } from "../../shared/format";
import { href, navigate } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { Explainer } from "../../shared/ui/Explainer";
import { Icon } from "../../shared/ui/Icon";
import { ErrorState, Loading } from "../../shared/ui/States";

const playgroundProjectName = "Playground";

export function ProjectsPage() {
  const api = useApi();
  const [state, reload] = useAsync((signal) => api.listProjects({ limit: 100 }, signal), [api]);

  return (
    <section>
      <div className="section-head">
        <div>
          <h1>Projects</h1>
          <p className="subtitle">A project is one product or environment. It owns its endpoints, events and deliveries.</p>
        </div>
      </div>

      {state.status === "loading" && <Loading />}
      {state.status === "error" && <ErrorState error={state.error} onRetry={reload} />}
      {state.status === "ready" && (
        <>
          {state.data.projects.length === 0 && <GetStarted />}
          {state.data.projects.length > 0 && (
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
          )}
          <div className="panel">
            <h2>New project</h2>
            <CreateProject onCreated={(id) => navigate(href("projects", id))} />
          </div>
          <Explainer />
        </>
      )}
    </section>
  );
}

/** First-run call to action: try the whole flow before creating anything real. */
function GetStarted() {
  const api = useApi();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function start() {
    setBusy(true);
    setError(null);
    try {
      const project = await api.createProject(playgroundProjectName);
      navigate(href("projects", project.id, "playground"));
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : "Could not create the project.");
      setBusy(false);
    }
  }

  return (
    <div className="panel hero">
      <div>
        <h2>New here? Try it in two minutes</h2>
        <p className="muted">
          The playground creates a project, lets you point an endpoint at a receiver, send a test event and watch it get delivered, retried and logged.
        </p>
      </div>
      <button type="button" className="button button-primary" disabled={busy} onClick={() => void start()}>
        <Icon name="play" size={16} />
        {busy ? "Creating…" : "Open the playground"}
      </button>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

function CreateProject({ onCreated }: { onCreated: (id: string) => void }) {
  const api = useApi();
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const project = await api.createProject(name.trim());
      onCreated(project.id);
    } catch (err) {
      setError(err instanceof Error ? describeError(err) : "Could not create the project.");
      setBusy(false);
    }
  }

  return (
    <form className="inline-form" onSubmit={submit}>
      <label htmlFor="project-name" className="sr-only">
        Project name
      </label>
      <input id="project-name" required maxLength={200} placeholder="Project name, e.g. Acme production" value={name} onChange={(e) => setName(e.target.value)} />
      <button type="submit" className="button button-primary" disabled={busy || name.trim() === ""}>
        <Icon name="plus" size={16} />
        {busy ? "Creating…" : "Create project"}
      </button>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </form>
  );
}
