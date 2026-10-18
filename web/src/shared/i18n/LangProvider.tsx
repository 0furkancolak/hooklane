import { Fragment, useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { currentLang, setCurrentLang, type Lang } from "./core";
import { LangContext } from "./context";

/** Holds the language and remounts the tree when it changes, so every string is re-read. */
export function LangProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(currentLang);
  const setLang = useCallback((next: Lang) => {
    setCurrentLang(next);
    setLangState(next);
  }, []);
  const value = useMemo(() => ({ lang, setLang }), [lang, setLang]);
  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);
  return (
    <LangContext.Provider value={value}>
      <Fragment key={lang}>{children}</Fragment>
    </LangContext.Provider>
  );
}
