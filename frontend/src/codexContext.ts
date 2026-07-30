export function isPackagedFullContextMode(value: string | undefined): boolean {
  return value === "full-target-packaged-no-tools" ||
    value === "full-target-chunk-summaries-no-tools";
}
