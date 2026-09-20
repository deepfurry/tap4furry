import type { ButtonHTMLAttributes, ReactNode } from 'react';
import styles from './Primitives.module.scss';
export function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className={`${styles.panel} flex flex-col gap-5 p-5 sm:p-7`}>
      <h2>{title}</h2>
      {children}
    </section>
  );
}
export function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <label className="flex flex-col gap-2">
      <span className={styles.label}>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  );
}
export function Button(props: ButtonHTMLAttributes<HTMLButtonElement>) {
  return <button {...props} className={`${styles.button} px-4 py-2 ${props.className ?? ''}`} />;
}
export function Badge({ children }: { children: ReactNode }) {
  return <span className={`${styles.badge} px-2 py-1`}>{children}</span>;
}
export function EmptyState({ children }: { children: ReactNode }) {
  return <p className={`${styles.empty} p-6`}>{children}</p>;
}
