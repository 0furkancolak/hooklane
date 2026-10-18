import { createContext, useContext } from "react";
import { currentLang, type Lang } from "./core";

export type LangContextValue = { lang: Lang; setLang: (lang: Lang) => void };
export const LangContext = createContext<LangContextValue>({ lang: currentLang(), setLang: () => undefined });

export function useLang(): LangContextValue {
  return useContext(LangContext);
}
