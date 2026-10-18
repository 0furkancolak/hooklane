import { t } from "../i18n";
import { Icon, type IconName } from "./Icon";

const steps: { icon: IconName; n: number }[] = [
  { icon: "events", n: 1 },
  { icon: "bolt", n: 2 },
  { icon: "repeat", n: 3 },
  { icon: "search", n: 4 },
];

const reasons = [1, 2, 3, 4];

/** Says what Hooklane is for and why to use it, in plain words. */
export function Explainer() {
  return (
    <section className="panel explainer" aria-labelledby="explainer-title">
      <h2 id="explainer-title">{t("explainer.title")}</h2>
      <p className="lead">{t("explainer.lead")}</p>
      <ol className="steps">
        {steps.map((step) => (
          <li key={step.n}>
            <span className="step-icon">
              <Icon name={step.icon} />
            </span>
            <strong>{t(`explainer.step${step.n}.title` as never)}</strong>
            <span className="muted">{t(`explainer.step${step.n}.text` as never)}</span>
          </li>
        ))}
      </ol>
      <h3>{t("explainer.why")}</h3>
      <ul className="reasons">
        {reasons.map((n) => (
          <li key={n}>
            <Icon name="check" size={16} />
            {t(`explainer.reason${n}` as never)}
          </li>
        ))}
      </ul>
    </section>
  );
}
