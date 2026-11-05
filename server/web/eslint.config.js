// ESLint runs the React hooks rules, which check what the React Compiler
// relies on, and the syntax rules Biome can't express. Biome does
// everything else.
import reactHooks from "eslint-plugin-react-hooks";
import tseslint from "typescript-eslint";

export default [
  { ignores: ["dist/", "mock-public/", "node_modules/", "src/api/schema.d.ts"] },
  {
    files: ["src/**/*.{ts,tsx}"],
    languageOptions: { parser: tseslint.parser },
    plugins: { "react-hooks": reactHooks },
    linterOptions: { reportUnusedDisableDirectives: "error" },
    rules: {
      ...reactHooks.configs["recommended-latest"].rules,
      "react-hooks/exhaustive-deps": "error",
      "react-hooks/incompatible-library": "error",
      "react-hooks/unsupported-syntax": "error",
    },
  },
  {
    files: ["src/**/*.tsx"],
    ignores: ["src/components/FormError.tsx", "**/*.test.tsx"],
    rules: {
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXOpeningElement[name.name='p'] > JSXAttribute[name.name='role'][value.value='alert']",
          message: "Say why something failed with FormError.",
        },
      ],
    },
  },
];
