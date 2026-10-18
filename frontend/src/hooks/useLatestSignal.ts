import { useCallback, useEffect, useRef } from "react";

/**
 * "Latest request wins" for imperative loaders (refresh buttons, polling,
 * callbacks shared by effects). Each call aborts the previous request and
 * returns a fresh signal; unmount aborts the last one (PF-02).
 */
export function useLatestSignal(): () => AbortSignal {
  const current = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      current.current?.abort();
      current.current = null;
    },
    [],
  );
  return useCallback(() => {
    current.current?.abort();
    const controller = new AbortController();
    current.current = controller;
    return controller.signal;
  }, []);
}
