/// <reference types="vite/client" />

// TypeScript 7 no longer infers side-effect CSS imports from the bundler
// alone. Keep the declaration local to the Vite entry point so strict
// type-checking remains compatible with the runtime CSS imports.
declare module "*.css" {
  const css: string;
  export default css;
}
