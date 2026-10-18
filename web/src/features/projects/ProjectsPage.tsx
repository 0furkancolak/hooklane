import { useState, type FormEvent } from "react";
import { useApi } from "../../shared/api/context";
import { describeError } from "../../shared/errors";
import { formatDate } from "../../shared/format";
import { t } from "../../shared/i18n";
import { href, navigate } from "../../shared/router";
import { useAsync } from "../../shared/useAsync";
import { Explainer } from "../../shared/ui/Explainer";
import { Icon } from "../../shared/ui/Icon";
import { PageHeader } from "../../shared/ui/PageHeader";
import { ErrorState, Loading } from "../../shared/ui/States";

const playgroundProjectName = "Playground";

export function ProjectsPage() {
  const api = useApi();
  const [state, reload] = useAsync((signal) => api.listProjects({ limit: 100 }, signal), [api]);

  return (
    <section>
      <PageHeader title={t("projects.title")} subtitle={t("projects.subtitle")} />

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
                    <span className="card-text">
                      <strong>{project.name}</strong>
                      <span className="muted">{t("projects.created", { date: formatDate(project.created_at) })}</span>
                    </span>
                    <Icon name="arrow" size={16} />
                  </a>
                </li>
              ))}
            </ul>
          )}
          <div className="panel">
            <h2>{t("projects.new")}</h2>
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
      setError(err instanceof Error ? describeError(err) : t("projects.createFailed"));
      setBusy(false);
    }
  }

  return (
    <div className="panel hero">
      <div>
        <h2>{t("projects.getStarted.title")}</h2>
        <p className="muted">{t("projects.getStarted.text")}</p>
      </div>
      <button type="button" className="button button-primary" disabled={busy} onClick={() => void start()}>
        <Icon name="play" size={16} />
        {busy ? t("common.creating") : t("projects.openPlayground")}
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
      setError(err instanceof Error ? describeError(err) : t("projects.createFailed"));
      setBusy(false);
    }
  }

  return (
    <form className="inline-form" onSubmit={submit}>
      <label htmlFor="project-name" className="sr-only">
        {t("projects.nameLabel")}
      </label>
      <input id="project-name" required maxLength={200} placeholder={t("projects.namePlaceholder")} value={name} onChange={(e) => setName(e.target.value)} />
      <button type="submit" className="button button-primary" disabled={busy || name.trim() === ""}>
        <Icon name="plus" size={16} />
        {busy ? t("common.creating") : t("projects.create")}
      </button>
      {error && (
        <p className="field-error" role="alert">
          {error}
        </p>
      )}
    </form>
  );
}
