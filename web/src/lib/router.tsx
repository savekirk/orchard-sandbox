import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";

const BASE = "/dashboard/";

export type Page =
  | "overview"
  | "transactions"
  | "callbacks"
  | "requests"
  | "messages"
  | "playground"
  | "auto-debit"
  | "test-data"
  | "failures"
  | "settings";

interface Route {
  page: Page;
  params: URLSearchParams;
  navigate: (page: Page, params?: Record<string, string>) => void;
}

const RouterContext = createContext<Route | null>(null);

function readLocation(): { page: Page; params: URLSearchParams } {
  const slug = window.location.pathname.startsWith(BASE) ? window.location.pathname.slice(BASE.length).replace(/\/$/, "") : "";
  return { page: (slug || "overview") as Page, params: new URLSearchParams(window.location.search) };
}

/** A tiny history-based router for the dashboard pages. */
export function RouterProvider({ children }: { children: ReactNode }) {
  const [location, setLocation] = useState(readLocation);

  useEffect(() => {
    const onPop = () => setLocation(readLocation());
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  const navigate = useCallback((page: Page, params?: Record<string, string>) => {
    const query = params ? `?${new URLSearchParams(params)}` : "";
    window.history.pushState(null, "", `${BASE}${page === "overview" ? "" : page}${query}`);
    setLocation(readLocation());
    window.scrollTo(0, 0);
  }, []);

  return <RouterContext.Provider value={{ ...location, navigate }}>{children}</RouterContext.Provider>;
}

export function useRoute() {
  const route = useContext(RouterContext);
  if (!route) throw new Error("useRoute outside RouterProvider");
  return route;
}
