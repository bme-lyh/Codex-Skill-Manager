import { Check, ChevronDown, FolderOpen } from "lucide-react";
import { useI18n } from "../i18n";
import { rootKindLabel } from "../roots";
import type { RootContract } from "../roots";

export interface RootSelectorProps {
  roots: RootContract[];
  value: string;
  onChange: (rootId: string) => void;
  label?: string;
  compact?: boolean;
  disabled?: boolean;
}

/** A native select keeps root choice keyboard and high-contrast friendly. */
export function RootSelector({ roots, value, onChange, label, compact = false, disabled = false }: RootSelectorProps) {
  const { t, locale } = useI18n();
  if (!roots.length) return null;
  const resolved = roots.some(root => root.rootId === value) ? value : roots[0].rootId;
  const fieldLabel = label ?? t("安装目标", "Install target");
  return <label className={`root-selector${compact ? " compact" : ""}`}>
    <span className="root-selector-label"><FolderOpen size={15} aria-hidden="true" />{fieldLabel}</span>
    <span className="root-selector-control">
      <select aria-label={fieldLabel} value={resolved} disabled={disabled} onChange={event => onChange(event.target.value)}>
        {roots.map(root => <option key={root.rootId} value={root.rootId}>
          {root.rootName} · {rootKindLabel(root.rootKind, locale)}
        </option>)}
      </select>
      <ChevronDown size={15} aria-hidden="true" />
    </span>
    <span className="root-selector-badges" aria-live="polite">
      {roots.map(root => root.rootId === resolved ? <span className="root-badge" key={root.rootId}>
        <Check size={12} aria-hidden="true" />{root.rootName}
      </span> : null)}
    </span>
  </label>;
}

