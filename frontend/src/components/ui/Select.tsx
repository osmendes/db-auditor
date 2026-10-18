import type { SelectHTMLAttributes } from "react";

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  options: Array<{ value: string; label: string }>;
  hint?: string;
}

/**
 * Select alinhado ao Design System do db-auditor (dark):
 * superfície elevated, borda Cinza, tipografia Satoshi, focus branco.
 * Native <select> com chevron custom (sem lib extra).
 */
export function Select({
  label,
  options,
  hint,
  className = "",
  id,
  disabled,
  ...props
}: SelectProps) {
  const selectId = id ?? props.name;
  return (
    <label className="grid gap-1.5 text-sm text-slate-300">
      {label ? (
        <span className="text-xs font-medium tracking-wide text-slate-400">
          {label}
        </span>
      ) : null}
      <span className="relative block">
        <select
          id={selectId}
          disabled={disabled}
          className={
            "h-10 w-full appearance-none rounded-md border border-slate-600 bg-slate-900 py-2 pr-10 pl-3 text-sm text-slate-100 shadow-none transition " +
            "hover:border-slate-500 " +
            "focus:border-white focus:outline-none focus:ring-1 focus:ring-white " +
            "disabled:cursor-not-allowed disabled:opacity-50 " +
            className
          }
          {...props}
        >
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        <span
          className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-slate-400"
          aria-hidden
        >
          <svg
            width="12"
            height="12"
            viewBox="0 0 12 12"
            fill="none"
            aria-hidden="true"
            focusable="false"
          >
            <path
              d="M2.5 4.5L6 8L9.5 4.5"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </span>
      </span>
      {hint ? <span className="text-xs text-slate-500">{hint}</span> : null}
    </label>
  );
}
