import type { InputHTMLAttributes } from "react";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
}

export function Input({ label, className = "", id, ...props }: InputProps) {
  const inputId = id ?? props.name;
  return (
    <label className="grid gap-1.5 text-sm text-slate-300">
      {label ? <span>{label}</span> : null}
      <input
        id={inputId}
        className={`rounded-md border border-slate-500 bg-slate-950 px-3 py-2 text-slate-100 placeholder:text-slate-500 focus:border-[var(--focus-ring)] focus:outline-none focus:ring-1 focus:ring-[var(--focus-ring)] ${className}`}
        {...props}
      />
    </label>
  );
}
