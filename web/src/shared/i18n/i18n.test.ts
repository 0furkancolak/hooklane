import { afterEach, describe, expect, it } from "vitest";
import { currentLang, locale, setCurrentLang, t, tn } from "./core";
import { en } from "./en";
import { tr } from "./tr";

afterEach(() => setCurrentLang("en"));

describe("i18n", () => {
  it("defines every English message in Turkish, and nothing extra", () => {
    expect(Object.keys(tr).sort()).toEqual(Object.keys(en).sort());
    for (const [key, text] of Object.entries(tr)) {
      expect(text.trim(), key).not.toBe("");
      // Placeholders must survive translation.
      const placeholders = (value: string) => (value.match(/\{\w+\}/g) ?? []).sort();
      expect(placeholders(text), key).toEqual(placeholders(en[key as keyof typeof en]));
    }
  });

  it("interpolates variables and keeps unknown placeholders visible", () => {
    expect(t("pager.range", { from: 1, to: 25 })).toBe("Showing 1–25");
    expect(t("events.noneHint")).toContain("{id}");
  });

  it("switches language and persists the choice", () => {
    setCurrentLang("tr");
    expect(currentLang()).toBe("tr");
    expect(locale()).toBe("tr-TR");
    expect(t("nav.projects")).toBe("Projeler");
    expect(localStorage.getItem("hooklane.lang")).toBe("tr");
  });

  it("chooses singular and plural forms", () => {
    expect(tn("playground.queued", 1)).toContain("1 endpoint.");
    expect(tn("playground.queued", 3)).toContain("3 endpoints.");
  });
});
